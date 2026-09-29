// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// Code is the one error enumeration the server and the SDK share (cut-a §5.2).
type Code string

// Every code an error may carry.
const (
	CodeSetupRequired   Code = "setup_required"
	CodeAuthRequired    Code = "auth_required"
	CodeForbidden       Code = "forbidden"
	CodeNotFound        Code = "not_found"
	CodeInvalidRequest  Code = "invalid_request"
	CodeConflict        Code = "conflict"
	CodeStale           Code = "stale"
	CodeRateLimited     Code = "rate_limited"
	CodeQuotaExhausted  Code = "quota_exhausted"
	CodeProviderError   Code = "provider_error"
	CodeNodeSilent      Code = "node_silent"
	CodeFileUnavailable Code = "file_unavailable"
	CodePreparing       Code = "preparing"
	CodePrepareFailed   Code = "prepare_failed"
	CodeMigrating       Code = "migrating"
	CodeInternal        Code = "internal"
)

// Hop names the leg of a request that failed, for reachability errors (cut-a §5.2).
type Hop string

// The legs a request can fail on that the server names.
const (
	// HopNode names the node as the leg that failed.
	HopNode Hop = "node"
	// HopProvider is the failing leg when a provider, TMDB or OpenSubtitles, is (cut-a §5.2).
	HopProvider Hop = "provider"
)

// ErrorBody is the one shape every error leaves in: {"error": {...}}.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the inside of an error.
type ErrorDetail struct {
	Code      Code              `json:"code"`
	Message   string            `json:"message"`
	Detail    map[string]string `json:"detail,omitempty"`
	FailedHop Hop               `json:"failedHop,omitempty"`
}

// Error is an API error with its HTTP status. Handlers return it; huma writes its body.
type Error struct {
	status int
	// headers go out with the error, such as a 425's Retry-After.
	headers http.Header
	ErrorBody
}

// Error returns the message.
func (failure *Error) Error() string {
	return string(failure.ErrorBody.Error.Code) + ": " + failure.ErrorBody.Error.Message
}

// GetHeaders are the headers huma writes with the error.
func (failure *Error) GetHeaders() http.Header {
	return failure.headers
}

// GetStatus is the HTTP status huma writes.
func (failure *Error) GetStatus() int {
	return failure.status
}

// newError builds an Error.
func newError(status int, code Code, message string) *Error {
	return &Error{status: status, ErrorBody: ErrorBody{Error: ErrorDetail{Code: code, Message: message}}}
}

// providerRateLimited is a provider asking us to wait, as every endpoint that calls one answers
// it: rate_limited, the wait in detail.retryAfterMs, and the provider as the failing leg.
func providerRateLimited(limited *outbound.RateLimitedError) *Error {
	failure := newError(http.StatusServiceUnavailable, CodeRateLimited, limited.Provider+" asks us to wait")
	failure.ErrorBody.Error.Detail = map[string]string{
		"retryAfterMs": strconv.FormatInt(limited.RetryAfter.Milliseconds(), 10),
	}
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}

// UseErrorShape makes huma's own errors — a malformed body, an unknown route's method, a panic
// it recovered — leave in the same shape as ours. The huma package keeps its error constructor
// in a package variable, so the composition root calls this once, before serving. It never echoes the
// submitted value, which could be a secret (D194 §5); it names where the problem is instead.
func UseErrorShape() {
	huma.NewError = func(status int, message string, details ...error) huma.StatusError {
		failure := newError(status, codeForStatus(status), message)
		if status >= http.StatusInternalServerError {
			failure.ErrorBody.Error.Message = "internal error"
			return failure
		}
		for index, detail := range details {
			if detail == nil {
				continue
			}
			if failure.ErrorBody.Error.Detail == nil {
				failure.ErrorBody.Error.Detail = map[string]string{}
			}
			key, text := describeDetail(index, detail)
			failure.ErrorBody.Error.Detail[key] = text
		}
		return failure
	}
}

// describeDetail names a validation problem by its location and message, never its value.
func describeDetail(index int, detail error) (location, message string) {
	unnamed := "problem" + strconv.Itoa(index+1)
	located, isLocated := errors.AsType[*huma.ErrorDetail](detail)
	switch {
	case isLocated && located.Location != "":
		return located.Location, located.Message
	case isLocated:
		return unnamed, located.Message
	}
	return unnamed, "the request is not valid"
}

// codeForStatus maps huma's statuses to our codes.
func codeForStatus(status int) Code {
	switch status {
	case http.StatusUnauthorized:
		return CodeAuthRequired
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusTooManyRequests:
		return CodeRateLimited
	case http.StatusServiceUnavailable:
		return CodeMigrating
	}
	if status >= http.StatusInternalServerError {
		return CodeInternal
	}
	return CodeInvalidRequest
}
