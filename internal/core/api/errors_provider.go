// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// RateLimitedDetailBody is rate_limited's detail when a provider asks us to wait.
type RateLimitedDetailBody struct {
	RetryAfterMs int64 `doc:"How long the provider asks us to wait" json:"retryAfterMs"`
}

func (RateLimitedDetailBody) problemCode() Code { return CodeRateLimited }

// QuotaExhaustedDetailBody is quota_exhausted's detail: when the provider's count starts again.
type QuotaExhaustedDetailBody struct {
	DownloadsResetAt int64 `doc:"Unix milliseconds" json:"downloadsResetAt"`
}

func (QuotaExhaustedDetailBody) problemCode() Code { return CodeQuotaExhausted }

// ProviderRefusalDetailBody is providerError's detail: why, in the provider's terms, one of
// providerReasons, which the document enumerates so a client can have words for each.
type ProviderRefusalDetailBody struct {
	Reason ProviderReason `json:"reason"`
	// NotAnsweringAt and NextCheckAt are, for a provider not answering, since when, and when it is next
	// looked at, in Unix milliseconds; absent when not known (D231 §12).
	NotAnsweringAt int64 `json:"notAnsweringAt,omitempty"`
	NextCheckAt    int64 `json:"nextCheckAt,omitempty"`
}

// ProviderReason is why an outside provider refused, as providerError's detail says it.
type ProviderReason string

// The reasons a provider refuses.
const (
	ReasonNoKey         ProviderReason = "noKey"
	ReasonKeyRefused    ProviderReason = "keyRefused"
	ReasonNoLogin       ProviderReason = "noLogin"
	ReasonLoginRefused  ProviderReason = "loginRefused"
	ReasonNotConfigured ProviderReason = "notConfigured"
	ReasonUnavailable   ProviderReason = "unavailable"
	// ReasonUnreadable is what the provider sent, which could not be read: a fetched subtitle its
	// node could not convert (D224 §5).
	ReasonUnreadable ProviderReason = "unreadable"
)

// providerReasons is every ProviderReason: the one list, which the document enumerates.
func providerReasons() []ProviderReason {
	return []ProviderReason{
		ReasonNoKey, ReasonKeyRefused, ReasonNoLogin, ReasonLoginRefused, ReasonNotConfigured, ReasonUnavailable,
		ReasonUnreadable,
	}
}

// Schema is a ProviderReason in the API document: a string, one of providerReasons.
func (ProviderReason) Schema(huma.Registry) *huma.Schema {
	reasons := providerReasons()
	enum := make([]any, 0, len(reasons))
	for _, reason := range reasons {
		enum = append(enum, string(reason))
	}
	return &huma.Schema{Type: huma.TypeString, Enum: enum}
}

func (ProviderRefusalDetailBody) problemCode() Code { return CodeProviderError }

// providerRateLimited is a provider asking us to wait, as every endpoint that calls one answers
// it: rate_limited, the wait in detail.retryAfterMs, and the provider as the failing leg.
func providerRateLimited(limited *outbound.RateLimitedError) *Error {
	failure := newDetailedError(http.StatusServiceUnavailable, limited.Provider+" asks us to wait",
		RateLimitedDetailBody{RetryAfterMs: limited.RetryAfter.Milliseconds()})
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}

// providerNotAnswering is an outside provider, at destination, not answering: since when, and when
// it is next looked at, as its provider service says (D231 §12).
func (server *Server) providerNotAnswering(ctx context.Context, destination jobs.Destination, message string) error {
	detail := ProviderRefusalDetailBody{Reason: ReasonUnavailable}
	if dependencies := server.dependencies.Load(); dependencies != nil {
		for _, provider := range dependencies.Providers {
			state := provider.State(ctx)
			parked := state.Parked
			if state.Destination != destination || parked == nil {
				continue
			}
			detail.NotAnsweringAt = parked.Since.UnixMilli()
			if !parked.NextProbeAt.IsZero() {
				detail.NextCheckAt = parked.NextProbeAt.UnixMilli()
			}
		}
	}
	failure := newDetailedError(http.StatusServiceUnavailable, message, detail)
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}

// providerRefusal is a provider's refusal as the API says it: providerError, why in
// detail.reason in the provider's terms, and the provider as the failing leg (cut-a §5.2),
// whichever provider.
func providerRefusal(reason ProviderReason, message string) error {
	failure := newDetailedError(http.StatusServiceUnavailable, message, ProviderRefusalDetailBody{Reason: reason})
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}
