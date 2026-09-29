// SPDX-License-Identifier: GPL-3.0-or-later

package opensubtitles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
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
	// IsHashMatch is whether it was uploaded for these exact bytes.
	IsHashMatch         bool
	IsHearingImpaired   bool
	IsMachineTranslated bool
	DownloadCount       int
	UploadedAt          time.Time
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
	if err := client.callAuthorised(ctx, http.MethodGet, "/subtitles", parameters, nil, &answer); err != nil {
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
			ProviderFileID: attributes.Files[0].FileID, ReleaseName: attributes.Release, Language: attributes.Language,
			IsHashMatch: attributes.MovieHashMatch, IsHearingImpaired: attributes.HearingImpaired,
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
	// RemainingDownloads is how many downloads the account has left, and ResetAt when that count starts
	// again, as OpenSubtitles says (D224 §4).
	RemainingDownloads int
	ResetAt            time.Time
}

// Download fetches one file, which spends one of the account's downloads.
func (client *Client) Download(ctx context.Context, providerFileID int64) (Downloaded, error) {
	body, err := json.Marshal(map[string]any{"file_id": providerFileID, "sub_format": "srt"})
	if err != nil {
		return Downloaded{}, fmt.Errorf("opensubtitles: encode the download: %w", err)
	}
	var answer downloadAnswer
	if downloadErr := client.callAuthorised(ctx, http.MethodPost, "/download", nil, body, &answer); downloadErr != nil {
		return Downloaded{}, downloadErr
	}
	text, err := client.fetch(ctx, answer.Link)
	if err != nil {
		return Downloaded{}, err
	}
	return Downloaded{
		Body: text, FileName: answer.FileName, RemainingDownloads: answer.Remaining, ResetAt: answer.resetAt(),
	}, nil
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

// fetch reads the file a download's link names: an https address of OpenSubtitles' own.
func (client *Client) fetch(ctx context.Context, link string) ([]byte, error) {
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != client.base.Scheme) || parsed.Host == "" {
		return nil, fmt.Errorf("opensubtitles: a download link that is not a web address: %q", link)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: %w", err)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: fetch the subtitle: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opensubtitles: fetch the subtitle: status %d", response.StatusCode)
	}
	text, err := bounded.ReadAll(response.Body, MaxSubtitleBytes)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: fetch the subtitle: %w", err)
	}
	return text, nil
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
	if err := client.callAuthorised(ctx, http.MethodGet, "/infos/user", nil, nil, &answer); err != nil {
		return Allowance{}, err
	}
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
