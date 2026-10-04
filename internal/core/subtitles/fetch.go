// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Subtitler is where subtitles are fetched on request: opensubtitles.Client (D224).
type Subtitler interface {
	Search(ctx context.Context, query opensubtitles.Query) ([]opensubtitles.Candidate, error)
	Download(ctx context.Context, providerFileID int64) (opensubtitles.Downloaded, error)
	Allowance(ctx context.Context) (opensubtitles.Allowance, error)
}

// providerOpenSubtitles is the fetched_subtitle table's name for OpenSubtitles.
const providerOpenSubtitles = "opensubtitles"

// ErrNoSubtitler is a server that fetches no subtitles: it was started without a provider.
var ErrNoSubtitler = errors.New("this server fetches no subtitles")

// ErrUnreadable is a fetched subtitle its node could not convert: it is not kept, and not
// the person's choice (D224 §5).
var ErrUnreadable = errors.New("the fetched subtitle could not be read")

// ErrNothingToSearchBy is a file that is neither identified nor hashed for OpenSubtitles, so
// nothing names it to the provider.
var ErrNothingToSearchBy = errors.New("the file is not identified and has no OpenSubtitles hash yet")

// Offer is what a person is shown before choosing a subtitle to fetch: the candidates,
// each unverified, those made for the file first, and what the account has left (D224 §4).
type Offer struct {
	Candidates []opensubtitles.Candidate
	Allowance  opensubtitles.Allowance
	// DidAskByIDs is a search that asked by the work's ids, which finds subtitles made for any copy of it;
	// false for a file not identified yet.
	DidAskByIDs bool
	// DidAskByOpensubtitlesHash is a search that asked by the file's moviehash, which finds those made for its bytes.
	DidAskByOpensubtitlesHash bool
}

// Candidates lists the subtitles OpenSubtitles has in language for a file a person may
// play, found by what the file is: its work's ids or its series' and place, and its moviehash
// (D224 §3).
func (service *Service) Candidates(ctx context.Context, fileID int64, language string) (Offer, error) {
	if service.subtitler == nil {
		return Offer{}, ErrNoSubtitler
	}
	if _, err := library.PlayableFile(ctx, service.database.Read(), fileID); err != nil {
		return Offer{}, err
	}
	query, hasIDs, err := service.subtitleQuery(ctx, fileID, language)
	if err != nil {
		return Offer{}, err
	}
	allowance, err := service.subtitler.Allowance(ctx)
	if err != nil {
		return Offer{}, err
	}
	candidates, err := service.subtitler.Search(ctx, query)
	if err != nil {
		return Offer{}, err
	}
	// One uploaded for these exact bytes is listed first (D224 §3), whatever order the provider
	// answered in; the rest keep its order.
	slices.SortStableFunc(candidates, func(one, other opensubtitles.Candidate) int {
		switch {
		case one.IsOpensubtitlesHashMatch == other.IsOpensubtitlesHashMatch:
			return 0
		case one.IsOpensubtitlesHashMatch:
			return -1
		}
		return 1
	})
	return Offer{
		Candidates: candidates, Allowance: allowance, DidAskByIDs: hasIDs,
		DidAskByOpensubtitlesHash: query.MovieHash != "",
	}, nil
}

// subtitleQuery is what names a file to OpenSubtitles, and whether that includes its work's ids.
func (service *Service) subtitleQuery(
	ctx context.Context, fileID int64, language string,
) (query opensubtitles.Query, hasIDs bool, err error) {
	file, err := store.OrNotFound(service.database.Read().GetFileForSubtitleSearch(ctx, fileID))
	if err != nil {
		return opensubtitles.Query{}, false, fmt.Errorf("read file %d: %w", fileID, err)
	}
	query = opensubtitles.Query{
		MovieHash: ptr.ValueOrZero(file.OpensubtitlesHash), Language: strings.ToLower(language),
	}
	switch {
	case file.TitleID != nil && ptr.ValueOrZero(file.TitleKind) == "episode":
		place, placeErr := store.OrNotFound(service.database.Read().GetEpisodePlace(ctx, *file.TitleID))
		if placeErr != nil && !errors.Is(placeErr, store.ErrNotFound) {
			return opensubtitles.Query{}, false, fmt.Errorf("read the place of title %d: %w", *file.TitleID, placeErr)
		}
		if placeErr == nil {
			query.Season, query.Episode = int(ptr.ValueOrZero(place.GroupNumber)),
				int(ptr.ValueOrZero(place.NumberInGroup))
			query.SeriesIMDbID, query.SeriesTMDBID, err = service.providerIDs(ctx, place.CollectionID)
			hasIDs = query.SeriesIMDbID != "" || query.SeriesTMDBID != ""
		}
	case file.TitleID != nil:
		query.IMDbID, query.TMDBID, err = service.providerIDs(ctx, *file.TitleID)
		hasIDs = query.IMDbID != "" || query.TMDBID != ""
	}
	if err != nil {
		return opensubtitles.Query{}, false, err
	}
	if !hasIDs && query.MovieHash == "" {
		return opensubtitles.Query{}, false, ErrNothingToSearchBy
	}
	return query, hasIDs, nil
}

// providerIDs are an item's IMDb and TMDB ids, as identification recorded them.
func (service *Service) providerIDs(ctx context.Context, itemID int64) (imdbID, tmdbID string, err error) {
	rows, err := service.database.Read().ListItemProviderIDs(ctx, itemID)
	if err != nil {
		return "", "", fmt.Errorf("read the ids of item %d: %w", itemID, err)
	}
	for _, row := range rows {
		switch row.Provider {
		case "imdb":
			imdbID = row.ProviderID
		case "tmdb":
			tmdbID = row.ProviderID
		}
	}
	return imdbID, tmdbID, nil
}

// FetchRequest is the candidate a person chose to fetch.
type FetchRequest struct {
	// ProviderFileID is the candidate's file at OpenSubtitles.
	ProviderFileID int64
	// Language is the candidate's language, BCP 47.
	Language string
}

// Fetched is a subtitle fetched on request, and what the account has left.
type Fetched struct {
	Track              preferences.TrackRef
	RemainingDownloads int
	DownloadsResetAt   time.Time
}

// Fetch downloads the subtitle a person chose, which spends one of the account's
// downloads, and has the node that holds the file convert it while the person waits, since
// hostile text is parsed on a node (D224 §5): the conversion job, queued at play priority, which
// the fetch waits for, at most the core's silence limit (D238). Only a subtitle the node converted
// is kept, in persistent.db, gzipped, since it cost that download (D224 §6), with its converted
// text, which the caller makes the file's subtitle choice for that person: nothing unparsed becomes a
// choice. A
// node that is away is found before a download is spent; a subtitle it cannot convert is
// ErrUnreadable, and one it did not convert in time is nodes.ErrNodeSilent, each kept
// nowhere. A download whose conversion never came back is not spent again by the next try while
// its link works (opensubtitles.Client.Download). A subtitle already kept for the file is not
// fetched again: the ask is given it, and a second ask while the first is underway waits for it
// (D244 §4).
func (service *Service) Fetch(
	ctx context.Context, userID, fileID int64, request FetchRequest,
) (Fetched, error) {
	if service.subtitler == nil {
		return Fetched{}, ErrNoSubtitler
	}
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return Fetched{}, err
	}
	providerFileID := strconv.FormatInt(request.ProviderFileID, 10)
	end, err := service.fetchesUnderway.begin(ctx, fetchKey(fileID, providerFileID))
	if err != nil {
		return Fetched{}, err
	}
	defer end()
	if kept, isKept, keptErr := service.alreadyFetched(ctx, fileID, providerFileID); keptErr != nil || isKept {
		return kept, keptErr
	}
	if !service.nodes.IsConnected(ctx, file.NodeID) {
		return Fetched{}, fmt.Errorf("convert a fetched subtitle on node %d: %w", file.NodeID, nodes.ErrNodeSilent)
	}
	downloaded, err := service.subtitler.Download(ctx, request.ProviderFileID)
	if err != nil {
		return Fetched{}, err
	}
	body, err := bounded.Gzip(downloaded.Body)
	if err != nil {
		return Fetched{}, err
	}
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var insertErr error
		id, insertErr = queries.InsertFetchedSubtitle(ctx, sqlcgen.InsertFetchedSubtitleParams{
			VideoFile: fileID, Provider: providerOpenSubtitles,
			ProviderFileID: providerFileID, Language: langtag.BCP47(request.Language),
			Format:   "srt",
			BodyGzip: body, FetchedByUserID: userID, FetchedAt: service.clock.Now().UnixMilli(),
			ReleaseName: &downloaded.FileName,
		})
		return insertErr
	})
	if err != nil {
		return Fetched{}, fmt.Errorf("keep the fetched subtitle: %w", err)
	}
	if err = service.convertWhileWaiting(ctx, id, fetchedKey(body)); err != nil {
		return Fetched{}, errors.Join(err, service.discardFetched(context.WithoutCancel(ctx), id, fetchedKey(body)))
	}
	track := preferences.TrackRef{FetchedSubtitleID: id}
	return Fetched{
		Track: track, RemainingDownloads: downloaded.RemainingDownloads, DownloadsResetAt: downloaded.DownloadsResetAt,
	}, nil
}

// alreadyFetched is the subtitle kept from an earlier fetch of the provider's file for file
// fileID, with what the account has left, which asking spends nothing; false when there is none
// (D244 §4).
func (service *Service) alreadyFetched(
	ctx context.Context, fileID int64, providerFileID string,
) (Fetched, bool, error) {
	id, err := store.OrNotFound(service.database.Read().GetFetchedSubtitleOfProviderFile(ctx,
		sqlcgen.GetFetchedSubtitleOfProviderFileParams{
			VideoFile: fileID, Provider: providerOpenSubtitles, ProviderFileID: providerFileID,
		}))
	if errors.Is(err, store.ErrNotFound) {
		return Fetched{}, false, nil
	}
	if err != nil {
		return Fetched{}, false, fmt.Errorf("look for the subtitle already fetched: %w", err)
	}
	allowance, err := service.subtitler.Allowance(ctx)
	if err != nil {
		return Fetched{}, false, err
	}
	return Fetched{
		Track: preferences.TrackRef{FetchedSubtitleID: id}, RemainingDownloads: allowance.RemainingDownloads,
	}, true, nil
}

// convertWhileWaiting queues the job that converts fetched subtitle id at play priority and waits
// for its text, kept where key says, for at most the core's silence limit: nil once it is made,
// ErrUnreadable when the converter could not, and nodes.ErrNodeSilent when it was not
// kept in time.
func (service *Service) convertWhileWaiting(ctx context.Context, id int64, key sqlcgen.GetSubtitleTextParams) error {
	kept := service.keptTexts.await(key)
	_, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindConvertFetchedSubtitle, Target: jobs.Target{FetchedSubtitleID: id},
		Priority: jobs.PriorityPlayNow, Reason: "a person fetched it and waits for it",
	})
	if err != nil {
		return fmt.Errorf("queue the conversion of fetched subtitle %d: %w", id, err)
	}
	limit := service.clock.NewTimer(service.maxSilence)
	defer limit.Stop()
	for {
		row, readErr := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, key))
		switch {
		case readErr == nil && row.Failure != nil:
			return fmt.Errorf("%w: %s", ErrUnreadable, *row.Failure)
		case readErr == nil:
			return nil
		case !errors.Is(readErr, store.ErrNotFound):
			return fmt.Errorf("read fetched subtitle %d's text: %w", id, readErr)
		}
		select {
		case <-kept:
			kept = service.keptTexts.await(key)
		case <-limit.C():
			return fmt.Errorf("convert fetched subtitle %d: no word from its node in %s: %w", id,
				service.maxSilence, nodes.ErrNodeSilent)
		case <-ctx.Done():
			return fmt.Errorf("convert fetched subtitle %d: %w", id, ctx.Err())
		}
	}
}

// discardFetched forgets a fetched subtitle its node did not convert, and the converter's word on
// it: only a subtitle its node converted is kept (D224 §6). Its job, if it runs later, finds it gone.
func (service *Service) discardFetched(ctx context.Context, id int64, key sqlcgen.GetSubtitleTextParams) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteFetchedSubtitle(ctx, id); err != nil {
			return err
		}
		return queries.ForgetSubtitleTextFailure(ctx, sqlcgen.ForgetSubtitleTextFailureParams(key))
	})
	if err != nil {
		return fmt.Errorf("discard fetched subtitle %d: %w", id, err)
	}
	return nil
}

// fetchedKey is where a fetched subtitle's text is kept: by the hash of its body as persistent.db
// keeps it, as a subtitle file's is by its payload hash.
func fetchedKey(body []byte) sqlcgen.GetSubtitleTextParams {
	sum := sha256.Sum256(body)
	return sqlcgen.GetSubtitleTextParams{SourceHash: sum[:]}
}

// HandleConvertFetchedSubtitle runs one conversion job (jobs.KindConvertFetchedSubtitle): the node that
// holds the video converts the fetched text, since hostile input is parsed on a node (D224 §5).
func (service *Service) HandleConvertFetchedSubtitle(ctx context.Context, job jobs.Job) error {
	fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, job.FetchedSubtitleID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.MarkPermanent(fmt.Errorf("no fetched subtitle %d, or its video is gone", job.FetchedSubtitleID))
	}
	if err != nil {
		return fmt.Errorf("read fetched subtitle %d: %w", job.FetchedSubtitleID, err)
	}
	text, err := bounded.Gunzip(fetched.BodyGzip, opensubtitles.MaxSubtitleBytes)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command := &nodeproto.ExtractSubtitle{
		FetchedSubRip: text, FetchedSha256: hex.EncodeToString(fetchedKey(fetched.BodyGzip).SourceHash),
	}
	return service.makeSubtitleText(ctx, job, fetched.NodeID, command)
}
