// SPDX-License-Identifier: GPL-3.0-or-later

package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// Query is what finds subtitles for one video: the work's ids, or its series' with its place,
// the file's moviehash, and a language (D224 §3).
type Query struct {
	// IMDbID and TMDBID are a film's; either may be empty.
	IMDbID string
	TMDBID string
	// SeriesIMDbID and SeriesTMDBID, with Season and Episode, are an episode's series and place.
	SeriesIMDbID string
	SeriesTMDBID string
	Season       int
	Episode      int
	// MovieHash is the file's moviehash, which puts subtitles made for these bytes first.
	MovieHash string
	// Language is the language asked for, as OpenSubtitles names it: "en", "pt-br".
	Language string
}

// Candidate is one subtitle OpenSubtitles offers. It is unverified: nothing here measured it
// against the audio (D147, D178 §4).
type Candidate struct {
	// ProviderFileID is the provider's file, which Download takes.
	ProviderFileID int64
	// ReleaseName is the uploader's name for what it was made for.
	ReleaseName string
	Language    string
	// IsOpensubtitlesHashMatch is whether it was uploaded for these exact bytes.
	IsOpensubtitlesHashMatch bool
	IsHearingImpaired        bool
	IsMachineTranslated      bool
	DownloadCount            int
	UploadedAt               time.Time
}

// Search lists the subtitles OpenSubtitles has for a query, those made for the file first.
func (client *Client) Search(ctx context.Context, query Query) ([]Candidate, error) {
	parameters := url.Values{}
	set := func(name, value string) {
		if value != "" {
			parameters.Set(name, strings.ToLower(value))
		}
	}
	set("imdb_id", numericID(query.IMDbID))
	set("tmdb_id", numericID(query.TMDBID))
	set("parent_imdb_id", numericID(query.SeriesIMDbID))
	set("parent_tmdb_id", numericID(query.SeriesTMDBID))
	if query.Season > 0 {
		set("season_number", strconv.Itoa(query.Season))
	}
	if query.Episode > 0 {
		set("episode_number", strconv.Itoa(query.Episode))
	}
	set("moviehash", query.MovieHash)
	set("languages", query.Language)
	var answer searchAnswer
	search := apiRequest{cost: providers.CostRequest, method: http.MethodGet, path: "/subtitles", query: parameters}
	err := client.callAuthorised(ctx, search, &answer)
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(answer.Data))
	for _, result := range answer.Data {
		attributes := result.Attributes
		if len(attributes.Files) == 0 {
			continue // nothing to download
		}
		uploaded, _ := time.Parse(time.RFC3339, attributes.UploadDate) //nolint:errcheck // zero when unsaid
		candidates = append(candidates, Candidate{
			ProviderFileID: attributes.Files[0].FileID, ReleaseName: textindex.Line(attributes.Release),
			Language:                 textindex.Line(attributes.Language),
			IsOpensubtitlesHashMatch: attributes.MovieHashMatch, IsHearingImpaired: attributes.HearingImpaired,
			IsMachineTranslated: attributes.MachineTranslated || attributes.AITranslated,
			DownloadCount:       attributes.DownloadCount, UploadedAt: uploaded,
		})
	}
	return candidates, nil
}

// searchAnswer is the part of a search's answer we read.
//
//nolint:tagliatelle // OpenSubtitles' field names
type searchAnswer struct {
	Data []struct {
		Attributes struct {
			Language          string `json:"language"`
			DownloadCount     int    `json:"download_count"`
			HearingImpaired   bool   `json:"hearing_impaired"`
			MachineTranslated bool   `json:"machine_translated"`
			AITranslated      bool   `json:"ai_translated"`
			MovieHashMatch    bool   `json:"moviehash_match"`
			Release           string `json:"release"`
			UploadDate        string `json:"upload_date"`
			Files             []struct {
				FileID int64 `json:"file_id"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

// Downloaded is a subtitle fetched, and what is left of the account's downloads.
type Downloaded struct {
	// Body is the subtitle as OpenSubtitles gives it, SubRip; free accounts' may carry its
	// advertising, which is shown as delivered (D224 §8).
	Body     []byte
	FileName string
	// RemainingDownloads is how many downloads the account has left, and DownloadsResetAt when
	// that count starts again, as OpenSubtitles says (D224 §4).
	RemainingDownloads int
	DownloadsResetAt   time.Time
}

// linkLife is how long a download's link is used: OpenSubtitles' lasts at most three hours from
// the download that made it (opensubtitles-api.md §2).
const linkLife = 3 * time.Hour

// heldLink is a download's answer, kept while its link works.
type heldLink struct {
	answer    downloadAnswer
	usedUntil time.Time
}

// Download fetches one file. The first Download of a file spends one of the account's downloads;
// its link is kept for its three hours, and a Download of the same file within them fetches the
// link again rather than spending another. So a link that fails once the download is spent, as
// unavailable or rate limited, costs nothing: it is retried, with back-off, within this call and
// by the next Download of the file while it works. A link refused any other way is forgotten.
func (client *Client) Download(ctx context.Context, providerFileID int64) (Downloaded, error) {
	held, isHeld := client.heldLink(providerFileID)
	if !isHeld {
		body, err := json.Marshal(map[string]any{"file_id": providerFileID, "sub_format": "srt"})
		if err != nil {
			return Downloaded{}, fmt.Errorf("opensubtitles: encode the download: %w", err)
		}
		var answer downloadAnswer
		download := apiRequest{cost: providers.CostCounted, method: http.MethodPost, path: "/download", body: body}
		downloadErr := client.callAuthorised(ctx, download, &answer)
		if downloadErr != nil {
			return Downloaded{}, downloadErr
		}
		client.provider.RecordQuota(answer.Remaining, answer.resetAt())
		held = heldLink{answer: answer, usedUntil: client.clock.Now().Add(linkLife)}
		client.holdLink(providerFileID, held)
	}
	text, err := client.fetchRetrying(ctx, held.answer.Link)
	if err != nil {
		if !mayWorkLater(err) {
			client.forgetLink(providerFileID)
		}
		return Downloaded{}, err
	}
	answer := held.answer
	return Downloaded{
		Body: text, FileName: textindex.Line(answer.FileName), RemainingDownloads: answer.Remaining,
		DownloadsResetAt: answer.resetAt(),
	}, nil
}

// mayWorkLater is a link's failure that says nothing of the link: unavailable, or rate limited.
func mayWorkLater(err error) bool {
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	return (isLimited && limited != nil) || errors.Is(err, outbound.ErrUnavailable)
}

// heldLink is the kept link of a file's download, while it works.
func (client *Client) heldLink(providerFileID int64) (heldLink, bool) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	held, found := client.linkByProviderFileID[providerFileID]
	return held, found && client.clock.Now().Before(held.usedUntil)
}

// holdLink keeps a download's link, and lets go of those past their time.
func (client *Client) holdLink(providerFileID int64, held heldLink) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	now := client.clock.Now()
	for id, kept := range client.linkByProviderFileID {
		if !now.Before(kept.usedUntil) {
			delete(client.linkByProviderFileID, id)
		}
	}
	client.linkByProviderFileID[providerFileID] = held
}

// forgetLink lets go of a link that will not work again.
func (client *Client) forgetLink(providerFileID int64) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	delete(client.linkByProviderFileID, providerFileID)
}

// fetchRetrying fetches a link, and again after each of linkRetryDelays while it answers as
// unavailable.
func (client *Client) fetchRetrying(ctx context.Context, link string) ([]byte, error) {
	text, err := client.fetch(ctx, link)
	for _, delay := range client.linkRetryDelays {
		if !errors.Is(err, outbound.ErrUnavailable) {
			break
		}
		wait := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			wait.Stop()
			return nil, errors.Join(err, ctx.Err())
		case <-wait.C:
		}
		text, err = client.fetch(ctx, link)
	}
	return text, err
}

// downloadAnswer is what a download answers, and a spent quota's answer too.
//
//nolint:tagliatelle // OpenSubtitles' field names
type downloadAnswer struct {
	Link         string `json:"link"`
	FileName     string `json:"file_name"`
	Remaining    int    `json:"remaining"`
	ResetTimeUTC string `json:"reset_time_utc"`
}

// resetAt is when the account's count starts again; zero when OpenSubtitles did not say.
func (answer downloadAnswer) resetAt() time.Time {
	at, _ := time.Parse(time.RFC3339, answer.ResetTimeUTC) //nolint:errcheck // zero when unsaid
	return at
}

// fetch reads the file a download's link names, once OpenSubtitles' provider says it may go: an
// https address of OpenSubtitles' own, checked before it is asked (go.md §21).
func (client *Client) fetch(ctx context.Context, link string) ([]byte, error) {
	var text []byte
	err := client.provider.Call(ctx, providers.CostRequest, func(ctx context.Context) error {
		var fetchErr error
		text, fetchErr = client.fetchLink(ctx, link)
		return fetchErr
	})
	return text, err //nolint:wrapcheck // the fetch's own error, or the provider's refusal, each naming OpenSubtitles
}

// fetchLink reads the file a download's link names.
func (client *Client) fetchLink(ctx context.Context, link string) ([]byte, error) {
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != client.base.Scheme) || parsed.Host == "" {
		return nil, fmt.Errorf("opensubtitles: a download link that is not a web address: %q", link)
	}
	if parsed.User != nil || !client.isOwnHost(parsed.Host) {
		return nil, fmt.Errorf("opensubtitles: a download link to another host than OpenSubtitles': %q", parsed.Host)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: %w", err)
	}
	answer, err := outbound.Exchange(client.http, request, MaxSubtitleBytes)
	if err == nil {
		err = outbound.StatusError(providerName, answer)
	}
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: fetch the subtitle: %w", err)
	}
	return answer.Body, nil
}

// Allowance is the account's downloads: how many are left of how many a day.
type Allowance struct {
	RemainingDownloads int
	AllowedDownloads   int
	IsVIP              bool
}

// Allowance asks OpenSubtitles what the account has left, which a person sees before they choose
// (D224 §4).
func (client *Client) Allowance(ctx context.Context) (Allowance, error) {
	var answer allowanceAnswer
	allowance := apiRequest{cost: providers.CostRequest, method: http.MethodGet, path: "/infos/user"}
	if err := client.callAuthorised(ctx, allowance, &answer); err != nil {
		return Allowance{}, err
	}
	client.provider.RecordQuota(answer.Data.RemainingDownloads, time.Time{})
	return Allowance{
		RemainingDownloads: answer.Data.RemainingDownloads, AllowedDownloads: answer.Data.AllowedDownloads,
		IsVIP: answer.Data.VIP,
	}, nil
}

// allowanceAnswer is what /infos/user answers.
//
//nolint:tagliatelle // OpenSubtitles' field names
type allowanceAnswer struct {
	Data struct {
		AllowedDownloads   int  `json:"allowed_downloads"`
		RemainingDownloads int  `json:"remaining_downloads"`
		VIP                bool `json:"vip"`
	} `json:"data"`
}
