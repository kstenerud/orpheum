// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/playback/opensubtitles"
)

// SubtitleCandidatesInput asks for the subtitles that can be fetched for a file in a language.
type SubtitleCandidatesInput struct {
	AuthorizationHeader
	FileID   int64  `minimum:"1" path:"id"`
	Language string `doc:"BCP 47: en, pt-BR" maxLength:"35" minLength:"2" pattern:"^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$" query:"language" required:"true"` //nolint:lll // one struct tag
}

// SubtitleCandidatesOutput is what a person sees before choosing: every candidate unverified,
// those made for the file first, and the account's downloads left (D224).
type SubtitleCandidatesOutput struct {
	Body struct {
		Candidates []SubtitleCandidateBody `json:"candidates"`
		Allowance  AllowanceBody           `json:"allowance"`
		// DidAskByIDs and DidAskByFileHash say what the search asked by, so a client can say
		// why it found nothing: a file not identified yet is searched by its hash alone.
		DidAskByIDs      bool `doc:"Asked by the work's ids: subtitles made for any copy of it" json:"didAskByIds"`
		DidAskByFileHash bool `doc:"Asked by the file's hash: those made for these bytes" json:"didAskByFileHash"`
	}
}

// SubtitleCandidateBody is one subtitle that can be fetched. None is verified against the audio
// in Cut A (D178 §4), and a client says so.
type SubtitleCandidateBody struct {
	ProviderFileID int64  `doc:"What POST /files/{id}/subtitles takes" json:"providerFileId"`
	ReleaseName    string `doc:"The uploader's name for what it was made for" json:"releaseName"`
	Language       string `json:"language"`
	// IsHashMatch is a subtitle uploaded for these exact bytes (D224 §3).
	IsHashMatch         bool  `json:"isHashMatch"`
	IsHearingImpaired   bool  `json:"isHearingImpaired"`
	IsMachineTranslated bool  `json:"isMachineTranslated"`
	DownloadCount       int   `json:"downloadCount"`
	UploadedAt          int64 `doc:"Unix milliseconds; absent when unknown" json:"uploadedAt,omitempty"`
}

// AllowanceBody is the account's downloads, as OpenSubtitles says (D224 §4).
type AllowanceBody struct {
	RemainingDownloads int  `json:"remainingDownloads"`
	AllowedDownloads   int  `json:"allowedDownloads"`
	IsVIP              bool `json:"isVip"`
}

// FetchSubtitleInput fetches the candidate a person chose.
type FetchSubtitleInput struct {
	AuthorizationHeader
	FileID int64 `minimum:"1" path:"id"`
	Body   struct {
		ProviderFileID int64  `json:"providerFileId" minimum:"1"`
		Language       string `json:"language" maxLength:"35" minLength:"2"`
	}
}

// FetchSubtitleOutput is the fetched subtitle, now the file's subtitle choice, and what is left.
type FetchSubtitleOutput struct {
	Body struct {
		FetchedID          int64 `doc:"The new track; a playback started again offers it" json:"fetchedId"`
		RemainingDownloads int   `json:"remainingDownloads"`
		DownloadsResetAt   int64 `doc:"When the count starts again, Unix milliseconds" json:"downloadsResetAt,omitempty"` //nolint:lll // one struct tag
	}
}

func (server *Server) registerSubtitleFetch() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getSubtitleCandidates", Method: http.MethodGet, Path: "/files/{id}/subtitle-candidates",
		Summary: "The subtitles OpenSubtitles can give for a file, each unverified, and the downloads left (D224)",
		Tags:    []string{tagPlayback},
	}, server.subtitleCandidates)
	huma.Register(server.api, huma.Operation{
		OperationID: "postFileSubtitle", Method: http.MethodPost, Path: "/files/{id}/subtitles",
		Summary: "Fetch a subtitle for a file; spends one of the account's daily downloads (D224)",
		Tags:    []string{tagPlayback},
	}, server.fetchSubtitle)
}

func (server *Server) subtitleCandidates(
	ctx context.Context, input *SubtitleCandidatesInput,
) (*SubtitleCandidatesOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	offer, err := dependencies.Playback.SubtitleCandidates(ctx, input.FileID, input.Language)
	if err != nil {
		return nil, server.fetchError(ctx, "list subtitle candidates", err)
	}
	output := &SubtitleCandidatesOutput{}
	output.Body.Candidates = make([]SubtitleCandidateBody, 0, len(offer.Candidates))
	for _, candidate := range offer.Candidates {
		output.Body.Candidates = append(output.Body.Candidates, SubtitleCandidateBody{
			ProviderFileID: candidate.FileID, ReleaseName: candidate.Release, Language: candidate.Language,
			IsHashMatch: candidate.IsHashMatch, IsHearingImpaired: candidate.IsHearingImpaired,
			IsMachineTranslated: candidate.IsMachineTranslated, DownloadCount: candidate.Downloads,
			UploadedAt: unixMillisOrZero(candidate.UploadedAt),
		})
	}
	output.Body.DidAskByIDs, output.Body.DidAskByFileHash = offer.DidAskByIDs, offer.DidAskByFileHash
	output.Body.Allowance = AllowanceBody{
		RemainingDownloads: offer.Allowance.Remaining, AllowedDownloads: offer.Allowance.Allowed,
		IsVIP: offer.Allowance.IsVIP,
	}
	return output, nil
}

func (server *Server) fetchSubtitle(ctx context.Context, input *FetchSubtitleInput) (*FetchSubtitleOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	fetched, err := dependencies.Playback.FetchSubtitle(ctx, principal.UserID, input.FileID, playback.FetchRequest{
		ProviderFileID: input.Body.ProviderFileID, Language: input.Body.Language,
	})
	if err != nil {
		return nil, server.fetchError(ctx, "fetch a subtitle", err)
	}
	output := &FetchSubtitleOutput{}
	output.Body.FetchedID, output.Body.RemainingDownloads = fetched.Track.FetchedID, fetched.Remaining
	output.Body.DownloadsResetAt = unixMillisOrZero(fetched.ResetAt)
	return output, nil
}

func unixMillisOrZero(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixMilli()
}

// fetchError says why a subtitle could not be listed or fetched, in terms a client can put in
// its viewer's words: detail.reason names the case (D224).
func (server *Server) fetchError(ctx context.Context, doing string, err error) error {
	withReason := func(status int, code Code, reason, message string) error {
		failure := newError(status, code, message)
		failure.ErrorBody.Error.Detail = map[string]string{"reason": reason}
		failure.ErrorBody.Error.Hop = HopProvider
		return failure
	}
	spent, isSpent := errors.AsType[*opensubtitles.QuotaSpentError](err)
	limited, isLimited := errors.AsType[*opensubtitles.RateLimitedError](err)
	switch {
	case errors.Is(err, playback.ErrNoFile):
		return newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case errors.Is(err, playback.ErrNothingToSearchBy):
		return newError(http.StatusConflict, CodeConflict,
			"this file is not identified yet, and has not been scanned since it could be hashed for OpenSubtitles")
	case errors.Is(err, playback.ErrNoSubtitler):
		return withReason(http.StatusServiceUnavailable, CodeProviderError, "not_configured",
			"this server fetches no subtitles")
	case errors.Is(err, opensubtitles.ErrNoLogin):
		return withReason(http.StatusServiceUnavailable, CodeProviderError, "no_login",
			"no OpenSubtitles login is set; an admin can set it in Settings")
	case errors.Is(err, opensubtitles.ErrLoginRefused):
		return withReason(http.StatusServiceUnavailable, CodeProviderError, "login_refused",
			"OpenSubtitles refused the login; an admin can change it in Settings")
	case isSpent:
		failure := newError(http.StatusTooManyRequests, CodeQuotaExhausted,
			"the OpenSubtitles account's downloads for today are spent")
		failure.ErrorBody.Error.Detail = map[string]string{"resetAt": spent.ResetAt.UTC().Format(time.RFC3339)}
		failure.ErrorBody.Error.Hop = HopProvider
		return failure
	case isLimited:
		failure := newError(http.StatusServiceUnavailable, CodeRateLimited, "OpenSubtitles asks us to wait")
		failure.ErrorBody.Error.Detail = map[string]string{"retryAfter": limited.After.String()}
		failure.ErrorBody.Error.Hop = HopProvider
		return failure
	}
	return server.internal(ctx, doing, err)
}
