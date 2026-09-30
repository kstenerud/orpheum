// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"reflect"

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

// codes are every Code, in the order above: the one list the API document's ProblemBody names.
// TestProblemBody_TheDocumentNamesEveryCodeOnceAndTypesItsDetail keeps it in step with the consts.
func codes() []Code {
	return []Code{
		CodeSetupRequired, CodeAuthRequired, CodeForbidden, CodeNotFound, CodeInvalidRequest, CodeConflict,
		CodeStale, CodeRateLimited, CodeQuotaExhausted, CodeProviderError, CodeNodeSilent, CodeFileUnavailable,
		CodePreparing, CodePrepareFailed, CodeMigrating, CodeInternal,
	}
}

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
	Error ProblemBody `json:"error"`
}

// ProblemBody is the inside of an error: its code, a note for developers, the leg that failed,
// and what the code carries in its detail. The API document gives it one form per code that has
// detail, and one for the codes that have none, so a client that narrows by code reads its detail
// typed: numbers are numbers.
type ProblemBody struct {
	Code      Code          `json:"code"`
	Message   string        `json:"message"`
	Detail    problemDetail `json:"detail,omitempty"`
	FailedHop Hop           `json:"failedHop,omitempty"`
}

// problemDetail is the detail one code carries; the code comes from the detail's type, so an
// error cannot carry another code's detail.
type problemDetail interface {
	problemCode() Code
}

// problemDetails are one of each detail type: the API document's form for each code with detail.
func problemDetails() []problemDetail {
	return []problemDetail{
		SignedOutDetailBody{},
		InvalidRequestDetailBody{},
		RateLimitedDetailBody{},
		QuotaExhaustedDetailBody{},
		ProviderRefusalDetailBody{},
	}
}

// SignedOutDetailBody is auth_required's detail: why there is no session, so a client can tell
// the person (cut-a §5.2).
type SignedOutDetailBody struct {
	Reason string `enum:"session_ended,session_expired,no_session" json:"reason"`
}

func (SignedOutDetailBody) problemCode() Code { return CodeAuthRequired }

// InvalidRequestDetailBody is invalid_request's detail: each problem found in the request, by
// where it is, never by the value submitted (D194 §5).
type InvalidRequestDetailBody struct {
	Problems []RequestProblemBody `json:"problems"`
}

func (InvalidRequestDetailBody) problemCode() Code { return CodeInvalidRequest }

// RequestProblemBody is one problem in a request.
type RequestProblemBody struct {
	Location string `doc:"Where it is, such as body.name; absent when the request as a whole is wrong" json:"location,omitempty"` //nolint:lll // one struct tag
	Message  string `json:"message"`
}

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

// ProviderRefusalDetailBody is provider_error's detail: why, in the provider's terms.
type ProviderRefusalDetailBody struct {
	Reason string `doc:"Such as no_key, key_refused, no_login, login_refused, not_configured, unavailable" json:"reason"` //nolint:lll // one struct tag
}

func (ProviderRefusalDetailBody) problemCode() Code { return CodeProviderError }

// problemBodySchemaName is ProblemBody's name among the document's schemas.
const problemBodySchemaName = "ProblemBody"

// Schema is ProblemBody's schema: one form per code with detail, typed by its detail's struct, and
// one for every other code, which carries none. It is registered once under its own name, so the
// SDK names it, and referred to from where it is used.
func (ProblemBody) Schema(registry huma.Registry) *huma.Schema {
	if _, isRegistered := registry.Map()[problemBodySchemaName]; !isRegistered {
		detailed := map[Code]bool{}
		details := problemDetails()
		forms := make([]*huma.Schema, 0, len(details)+1)
		for _, detail := range details {
			detailed[detail.problemCode()] = true
			forms = append(forms, problemForm([]Code{detail.problemCode()},
				registry.Schema(reflect.TypeOf(detail), true, "")))
		}
		var plain []Code
		for _, code := range codes() {
			if !detailed[code] {
				plain = append(plain, code)
			}
		}
		forms = append(forms, problemForm(plain, nil))
		registry.Map()[problemBodySchemaName] = &huma.Schema{
			Description: "An error: a person is shown words chosen by its code, never its message (cut-a §5.2)",
			OneOf:       forms,
		}
	}
	return &huma.Schema{Ref: schemaRefPrefix + problemBodySchemaName}
}

// schemaRefPrefix is where the document's schemas are, as huma's default configuration puts them.
const schemaRefPrefix = "#/components/schemas/"

// problemForm is one form of ProblemBody: its codes, and the detail they carry, if any.
func problemForm(formCodes []Code, detail *huma.Schema) *huma.Schema {
	names := make([]any, 0, len(formCodes))
	for _, code := range formCodes {
		names = append(names, string(code))
	}
	properties := map[string]*huma.Schema{
		"code":    {Type: huma.TypeString, Enum: names},
		"message": {Type: huma.TypeString, Description: "For developers, in English"},
		"failedHop": {
			Type: huma.TypeString, Enum: []any{string(HopNode), string(HopProvider)},
			Description: "The leg of the request that failed, for reachability errors",
		},
	}
	if detail != nil {
		properties["detail"] = detail
	}
	return &huma.Schema{
		Type: huma.TypeObject, Properties: properties, Required: []string{"code", "message"},
		AdditionalProperties: false,
	}
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
	return &Error{status: status, ErrorBody: ErrorBody{Error: ProblemBody{Code: code, Message: message}}}
}

// newDetailedError builds an Error whose code is its detail's.
func newDetailedError(status int, message string, detail problemDetail) *Error {
	failure := newError(status, detail.problemCode(), message)
	failure.ErrorBody.Error.Detail = detail
	return failure
}

// providerRateLimited is a provider asking us to wait, as every endpoint that calls one answers
// it: rate_limited, the wait in detail.retryAfterMs, and the provider as the failing leg.
func providerRateLimited(limited *outbound.RateLimitedError) *Error {
	failure := newDetailedError(http.StatusServiceUnavailable, limited.Provider+" asks us to wait",
		RateLimitedDetailBody{RetryAfterMs: limited.RetryAfter.Milliseconds()})
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
		if failure.ErrorBody.Error.Code != CodeInvalidRequest {
			return failure
		}
		var problems []RequestProblemBody
		for _, detail := range details {
			if detail != nil {
				problems = append(problems, describeProblem(detail))
			}
		}
		if len(problems) > 0 {
			failure.ErrorBody.Error.Detail = InvalidRequestDetailBody{Problems: problems}
		}
		return failure
	}
}

// describeProblem names a validation problem by its location and message, never its value.
func describeProblem(detail error) RequestProblemBody {
	located, isLocated := errors.AsType[*huma.ErrorDetail](detail)
	if isLocated {
		return RequestProblemBody{Location: located.Location, Message: located.Message}
	}
	return RequestProblemBody{Message: "the request is not valid"}
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

// providerRefusal is a provider's refusal as the API says it: provider_error, why in
// detail.reason in the provider's terms, and the provider as the failing leg (cut-a §5.2),
// whichever provider.
func providerRefusal(reason, message string) error {
	failure := newDetailedError(http.StatusServiceUnavailable, message, ProviderRefusalDetailBody{Reason: reason})
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}
