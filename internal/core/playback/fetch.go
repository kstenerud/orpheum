// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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

// ErrUnreadableSubtitle is a fetched subtitle its node could not convert: it is not kept, and not
// the person's choice (D224 §5).
var ErrUnreadableSubtitle = errors.New("the fetched subtitle could not be read")

// ErrNothingToSearchBy is a file that is neither identified nor hashed for OpenSubtitles, so
// nothing names it to the provider.
var ErrNothingToSearchBy = errors.New("the file is not identified and has no OpenSubtitles hash yet")

// SubtitleOffer is what a person is shown before choosing a subtitle to fetch: the candidates,
// each unverified, those made for the file first, and what the account has left (D224 §4).
type SubtitleOffer struct {
	Candidates []opensubtitles.Candidate
	Allowance  opensubtitles.Allowance
	// DidAskByIDs is a search that asked by the work's ids, which finds subtitles made for any copy of it;
	// false for a file not identified yet.
	DidAskByIDs bool
	// DidAskByOpensubtitlesHash is a search that asked by the file's moviehash, which finds those made for its bytes.
	DidAskByOpensubtitlesHash bool
}

// SubtitleCandidates lists the subtitles OpenSubtitles has in language for a file a person may
// play, found by what the file is: its work's ids or its series' and place, and its moviehash
// (D224 §3).
func (service *Service) SubtitleCandidates(ctx context.Context, fileID int64, language string) (SubtitleOffer, error) {
	if service.subtitler == nil {
		return SubtitleOffer{}, ErrNoSubtitler
	}
	if _, err := service.playableFile(ctx, service.database.Read(), fileID); err != nil {
		return SubtitleOffer{}, err
	}
	query, hasIDs, err := service.subtitleQuery(ctx, fileID, language)
	if err != nil {
		return SubtitleOffer{}, err
	}
	allowance, err := service.subtitler.Allowance(ctx)
	if err != nil {
		return SubtitleOffer{}, err
	}
	candidates, err := service.subtitler.Search(ctx, query)
	if err != nil {
		return SubtitleOffer{}, err
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
	return SubtitleOffer{
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

// FetchSubtitle downloads the subtitle a person chose, which spends one of the account's
// downloads, and has the node that holds the file convert it while the person waits, since
// hostile text is parsed on a node (D224 §5). Only a subtitle the node converted is kept, in
// persistent.db, gzipped, since it cost that download (D224 §6), with its converted text, and made
// the file's subtitle choice for that person: nothing unparsed becomes a choice. A node that is
// away is found before a download is spent; a subtitle it cannot convert is ErrUnreadableSubtitle,
// kept nowhere. A download whose conversion never came back is not spent again by the next try
// while its link works (opensubtitles.Client.Download).
func (service *Service) FetchSubtitle(
	ctx context.Context, userID, fileID int64, request FetchRequest,
) (Fetched, error) {
	if service.subtitler == nil {
		return Fetched{}, ErrNoSubtitler
	}
	file, err := service.playableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return Fetched{}, err
	}
	if !service.nodes.IsConnected(ctx, file.NodeID) {
		return Fetched{}, fmt.Errorf("convert a fetched subtitle on node %d: %w", file.NodeID, nodes.ErrNodeSilent)
	}
	downloaded, err := service.subtitler.Download(ctx, request.ProviderFileID)
	if err != nil {
		return Fetched{}, err
	}
	converted, err := service.convertFetched(ctx, file.NodeID, downloaded.Body)
	if err != nil {
		return Fetched{}, err
	}
	body, err := bounded.Gzip(downloaded.Body)
	if err != nil {
		return Fetched{}, err
	}
	if keepErr := service.keepConvertedText(ctx, fetchedKey(body), converted); keepErr != nil {
		return Fetched{}, keepErr
	}
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var insertErr error
		id, insertErr = queries.InsertFetchedSubtitle(ctx, sqlcgen.InsertFetchedSubtitleParams{
			VideoFile: fileID, Provider: providerOpenSubtitles,
			ProviderFileID: strconv.FormatInt(request.ProviderFileID, 10), Language: langtag.BCP47(request.Language),
			Format:   "srt",
			BodyGzip: body, FetchedByUserID: userID, FetchedAt: service.clock.Now().UnixMilli(),
			ReleaseName: &downloaded.FileName,
		})
		return insertErr
	})
	if err != nil {
		return Fetched{}, fmt.Errorf("keep the fetched subtitle: %w", err)
	}
	track := preferences.TrackRef{FetchedSubtitleID: id}
	if chooseErr := service.ChooseSubtitle(ctx, userID, fileID, preferences.Used{TrackRef: track}); chooseErr != nil {
		return Fetched{}, chooseErr
	}
	return Fetched{
		Track: track, RemainingDownloads: downloaded.RemainingDownloads, DownloadsResetAt: downloaded.DownloadsResetAt,
	}, nil
}

// convertFetched has the node convert a fetched subtitle's text while a person waits, and returns
// what it made: ErrUnreadableSubtitle when the node could not convert it, and nodes.ErrNodeSilent
// when it refused or said nothing.
func (service *Service) convertFetched(
	ctx context.Context, nodeID int64, text []byte,
) (nodeproto.SubtitleText, error) {
	conversionID := -service.fetchConversions.Add(1)
	conversion := jobs.Job{ID: conversionID, Priority: jobs.PriorityPlayNow}
	exchange, err := service.exchanges.Ask(ctx, nodeID, conversion, nodeproto.Command{
		Type:            nodeproto.CommandExtractSubtitle,
		ExtractSubtitle: &nodeproto.ExtractSubtitle{FetchedSubRip: text},
	}, "a fetched subtitle")
	if err != nil {
		return nodeproto.SubtitleText{}, fmt.Errorf("convert a fetched subtitle on node %d: %w", nodeID, err)
	}
	// A person waits for this, not a job: their request ending stops the node's work too.
	defer exchange.Close()
	result, err := exchange.Next(ctx, service.maxSilence)
	switch {
	case err != nil:
		return nodeproto.SubtitleText{}, err
	case result.Subtitle == nil:
		return nodeproto.SubtitleText{}, fmt.Errorf(
			"node %d answered a fetched subtitle with something other than a subtitle: %w", nodeID, nodes.ErrNodeSilent)
	}
	exchange.MarkEndedByNode()
	answer := *result.Subtitle
	switch {
	case answer.Refusal != "":
		return answer, fmt.Errorf("node %d refused to convert a fetched subtitle: %s: %w",
			nodeID, answer.Refusal, nodes.ErrNodeSilent)
	case answer.Failure != "":
		return answer, fmt.Errorf("%w: %s", ErrUnreadableSubtitle, answer.Failure)
	}
	return answer, nil
}

// keepConvertedText keeps a fetched subtitle's converted text where a playback finds it.
func (service *Service) keepConvertedText(
	ctx context.Context, key sqlcgen.GetSubtitleTextParams, converted nodeproto.SubtitleText,
) error {
	return service.finishSubtitle(ctx, key, converted)
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
	command := &nodeproto.ExtractSubtitle{FetchedSubRip: text}
	return service.makeSubtitleText(ctx, job, fetched.NodeID, command, fetchedKey(fetched.BodyGzip))
}
