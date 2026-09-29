// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/playback/opensubtitles"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Subtitler is where subtitles are fetched on request: opensubtitles.Client (D224).
type Subtitler interface {
	Search(ctx context.Context, query opensubtitles.Query) ([]opensubtitles.Candidate, error)
	Download(ctx context.Context, fileID int64) (opensubtitles.Downloaded, error)
	Allowance(ctx context.Context) (opensubtitles.Allowance, error)
}

// providerOpenSubtitles is the fetched_subtitle table's name for OpenSubtitles.
const providerOpenSubtitles = "opensubtitles"

// ErrNoSubtitler is a server that fetches no subtitles: it was started without a provider.
var ErrNoSubtitler = errors.New("this server fetches no subtitles")

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
	// DidAskByFileHash is a search that asked by the file's moviehash, which finds those made for its bytes.
	DidAskByFileHash bool
}

// SubtitleCandidates lists the subtitles OpenSubtitles has in language for a file a person may
// play, found by what the file is: its work's ids or its series' and place, and its moviehash
// (D224 §3).
func (service *Service) SubtitleCandidates(ctx context.Context, fileID int64, language string) (SubtitleOffer, error) {
	if service.subtitler == nil {
		return SubtitleOffer{}, ErrNoSubtitler
	}
	if _, err := service.file(ctx, fileID); err != nil {
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
	return SubtitleOffer{
		Candidates: candidates, Allowance: allowance, DidAskByIDs: hasIDs, DidAskByFileHash: query.MovieHash != "",
	}, nil
}

// subtitleQuery is what names a file to OpenSubtitles, and whether that includes its work's ids.
func (service *Service) subtitleQuery(
	ctx context.Context, fileID int64, language string,
) (query opensubtitles.Query, hasIDs bool, err error) {
	found, err := store.One(service.database.Read().GetFileForSubtitleSearch(ctx, fileID))
	if err != nil {
		return opensubtitles.Query{}, false, fmt.Errorf("read file %d: %w", fileID, err)
	}
	query = opensubtitles.Query{
		MovieHash: ptr.ValueOrZero(found.OpensubtitlesHash), Language: strings.ToLower(language),
	}
	switch {
	case found.TitleID != nil && ptr.ValueOrZero(found.TitleKind) == "episode":
		place, placeErr := store.One(service.database.Read().GetEpisodePlace(ctx, *found.TitleID))
		if placeErr != nil && !errors.Is(placeErr, store.ErrNotFound) {
			return opensubtitles.Query{}, false, fmt.Errorf("read the place of title %d: %w", *found.TitleID, placeErr)
		}
		if placeErr == nil {
			query.Season, query.Episode = int(ptr.ValueOrZero(place.GroupNumber)),
				int(ptr.ValueOrZero(place.NumberInGroup))
			query.SeriesIMDbID, query.SeriesTMDBID, err = service.providerIDs(ctx, place.CollectionID)
			hasIDs = query.SeriesIMDbID != "" || query.SeriesTMDBID != ""
		}
	case found.TitleID != nil:
		query.IMDbID, query.TMDBID, err = service.providerIDs(ctx, *found.TitleID)
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
func (service *Service) providerIDs(ctx context.Context, itemID int64) (imdb, tmdb string, err error) {
	rows, err := service.database.Read().ListItemProviderIDs(ctx, itemID)
	if err != nil {
		return "", "", fmt.Errorf("read the ids of item %d: %w", itemID, err)
	}
	for _, row := range rows {
		switch row.Provider {
		case "imdb":
			imdb = row.ProviderID
		case "tmdb":
			tmdb = row.ProviderID
		}
	}
	return imdb, tmdb, nil
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
	Track     TrackRef
	Remaining int
	ResetAt   time.Time
}

// FetchSubtitle downloads the subtitle a person chose, which spends one of the account's downloads;
// keeps it in library.db, gzipped, since it cost that download (D224 §6); and makes it the file's
// subtitle choice for that person. The node converts it the next time the file plays.
func (service *Service) FetchSubtitle(
	ctx context.Context, userID, fileID int64, request FetchRequest,
) (Fetched, error) {
	if service.subtitler == nil {
		return Fetched{}, ErrNoSubtitler
	}
	if _, err := service.file(ctx, fileID); err != nil {
		return Fetched{}, err
	}
	downloaded, err := service.subtitler.Download(ctx, request.ProviderFileID)
	if err != nil {
		return Fetched{}, err
	}
	body, err := gzipped(downloaded.Body)
	if err != nil {
		return Fetched{}, err
	}
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var insertErr error
		id, insertErr = queries.InsertFetchedSubtitle(ctx, sqlcgen.InsertFetchedSubtitleParams{
			VideoFile: fileID, Provider: providerOpenSubtitles,
			ProviderFileID: strconv.FormatInt(request.ProviderFileID, 10), Lang: request.Language, Format: "srt",
			BodyGzip: body, FetchedBy: userID, FetchedAt: service.clock.Now().UnixMilli(),
			ReleaseName: &downloaded.FileName,
		})
		return insertErr
	})
	if err != nil {
		return Fetched{}, fmt.Errorf("keep the fetched subtitle: %w", err)
	}
	track := TrackRef{FetchedSubtitleID: id}
	if chooseErr := service.ChooseSubtitle(ctx, userID, fileID, SubtitleChoice{TrackRef: track}); chooseErr != nil {
		return Fetched{}, chooseErr
	}
	return Fetched{Track: track, Remaining: downloaded.Remaining, ResetAt: downloaded.ResetAt}, nil
}

// gzipped is a fetched subtitle's body as library.db keeps it.
func gzipped(text []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(text); err != nil {
		return nil, fmt.Errorf("compress the subtitle: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compress the subtitle: %w", err)
	}
	return buffer.Bytes(), nil
}

// gunzipped is a fetched subtitle's text, bounded as the node bounds a subtitle's.
func gunzipped(body []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitle: %w", err)
	}
	text, err := io.ReadAll(io.LimitReader(reader, maxFetchedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitle: %w", err)
	}
	if len(text) > maxFetchedBytes {
		return nil, fmt.Errorf("a fetched subtitle larger than %d bytes", maxFetchedBytes)
	}
	return text, nil
}

// maxFetchedBytes bounds a fetched subtitle's text: a film's is a few hundred kilobytes.
const maxFetchedBytes = 8 << 20

// fetchedKey is where a fetched subtitle's text is kept: by the hash of its body as library.db
// keeps it, as a subtitle file's is by its payload hash.
func fetchedKey(body []byte) cachegen.GetSubtitleTextParams {
	sum := sha256.Sum256(body)
	return cachegen.GetSubtitleTextParams{BytesPayloadHash: sum[:]}
}

// HandleFetchedSubtitle runs one conversion job (jobs.KindConvertFetchedSubtitle): the node that
// holds the video converts the fetched text, since hostile input is parsed on a node (D224 §5).
func (service *Service) HandleFetchedSubtitle(ctx context.Context, job jobs.Job) error {
	fetched, err := store.One(service.database.Read().GetFetchedSubtitle(ctx, job.FetchedSubtitleID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no fetched subtitle %d, or its video is gone", job.FetchedSubtitleID))
	}
	if err != nil {
		return fmt.Errorf("read fetched subtitle %d: %w", job.FetchedSubtitleID, err)
	}
	text, err := gunzipped(fetched.BodyGzip)
	if err != nil {
		return jobs.Permanent(err)
	}
	return service.convert(ctx, job, fetched.NodeID, &nodeproto.ExtractSubtitle{JobID: job.ID, FetchedSubRip: text},
		fetchedKey(fetched.BodyGzip))
}
