// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
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
	// CodeMediaUnreachable is a play of a file that cannot be read now: its machine away, or its
	// folder or disk missing, which its detail says, since when (D241 §4).
	CodeMediaUnreachable Code = "media_unreachable"
	CodePreparing        Code = "preparing"
	CodePrepareFailed    Code = "prepare_failed"
	// CodeNoRoomToMake is a chunk of a quality whose run was evicted to keep its machine's other runs
	// faster than playback (D250 §6): the player's playback stops, and says why.
	CodeNoRoomToMake Code = "no_room_to_make"
	CodeMigrating    Code = "migrating"
	CodeInternal     Code = "internal"
)

// codes are every Code, in the order above: the one list the API document's ProblemBody names.
// TestProblemBody_TheDocumentNamesEveryCodeOnceAndTypesItsDetail keeps it in step with the consts.
func codes() []Code {
	return []Code{
		CodeSetupRequired, CodeAuthRequired, CodeForbidden, CodeNotFound, CodeInvalidRequest, CodeConflict,
		CodeStale, CodeRateLimited, CodeQuotaExhausted, CodeProviderError, CodeNodeSilent, CodeFileUnavailable,
		CodeMediaUnreachable, CodePreparing, CodePrepareFailed, CodeNoRoomToMake, CodeMigrating, CodeInternal,
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
		MediaUnreachableDetailBody{},
	}
}

// MediaUnreachableDetailBody is media_unreachable's detail: why the file cannot be read now, on
// which machine, the folder or disk missing, and since when (D241 §4).
type MediaUnreachableDetailBody struct {
	Reason   string `enum:"node_away,root_missing,disk_missing" json:"reason"`
	NodeName string `doc:"The machine that holds the file" json:"nodeName"`
	Path     string `doc:"The folder or disk missing on it; absent for a machine away" json:"path,omitempty"`
	// UnavailableAt is when it became unavailable, in Unix milliseconds; absent when not known.
	UnavailableAt int64 `json:"unavailableAt,omitempty"`
}

func (MediaUnreachableDetailBody) problemCode() Code { return CodeMediaUnreachable }

// mediaUnreachable is a play refused because its file cannot be read now, its node named by
// nodeName (D241 §4).
func mediaUnreachable(unreadable *library.UnreadableError, nodeName func(nodeID int64) string) error {
	detail := MediaUnreachableDetailBody(*unavailableBody(&unreadable.Unreadable, nodeName))
	return newDetailedError(http.StatusServiceUnavailable, "this file cannot be read now: "+unreadable.Error(), detail)
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
	Location string `doc:"Where it is: a field such as body.name or query.cursor, or body when the body as a whole is wrong" json:"location"` //nolint:lll // one struct tag
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

// ProviderRefusalDetailBody is provider_error's detail: why, in the provider's terms, one of
// providerReasons, which the document enumerates so a client can have words for each.
type ProviderRefusalDetailBody struct {
	Reason ProviderReason `json:"reason"`
	// NotAnsweringAt and NextCheckAt are, for a provider not answering, since when, and when it is next
	// looked at, in Unix milliseconds; absent when not known (D231 §12).
	NotAnsweringAt int64 `json:"notAnsweringAt,omitempty"`
	NextCheckAt    int64 `json:"nextCheckAt,omitempty"`
}

// ProviderReason is why an outside provider refused, as provider_error's detail says it.
type ProviderReason string

// The reasons a provider refuses.
const (
	ReasonNoKey         ProviderReason = "no_key"
	ReasonKeyRefused    ProviderReason = "key_refused"
	ReasonNoLogin       ProviderReason = "no_login"
	ReasonLoginRefused  ProviderReason = "login_refused"
	ReasonNotConfigured ProviderReason = "not_configured"
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

// logErrorAnswer logs every error the API answers with, at debug: a client shows a person only words
// chosen by the code, so the log is where an error's operation, code and message can be read. It
// is a huma transformer, so huma's own errors are logged too; it never changes the body.
func (server *Server) logErrorAnswer(ctx huma.Context, status string, body any) (any, error) {
	failure, isError := body.(*Error)
	if !isError {
		return body, nil
	}
	operationID := ""
	if operation := ctx.Operation(); operation != nil {
		operationID = operation.OperationID
	}
	server.logger.DebugContext(ctx.Context(), "answered with an error",
		"operation", operationID, "method", ctx.Method(), "path", ctx.URL().Path, "status", status,
		"code", failure.ErrorBody.Error.Code, "message", failure.ErrorBody.Error.Message)
	return body, nil
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

// invalidRequestAt is a refusal of what a request carries, naming where: a field such as
// body.rootPaths, query.cursor or body.valuesByKey.<key>, or body when the request as a whole is
// wrong. A client names the field in its own words from location (D188 §1), so every
// invalid_request a handler sends is built here; TestArch_EveryInvalidRequestSaysWhere holds it.
func invalidRequestAt(status int, location, message string) *Error {
	return newDetailedError(status, message, InvalidRequestDetailBody{
		Problems: []RequestProblemBody{{Location: location, Message: message}},
	})
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

// describeProblem names a validation problem by its location and message, never its value. A
// problem huma does not place, a body that is not JSON, is the body's as a whole.
func describeProblem(detail error) RequestProblemBody {
	located, isLocated := errors.AsType[*huma.ErrorDetail](detail)
	if isLocated && located.Location != "" {
		return RequestProblemBody{Location: located.Location, Message: located.Message}
	}
	return RequestProblemBody{Location: "body", Message: "the request is not valid"}
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

// providerRefusal is a provider's refusal as the API says it: provider_error, why in
// detail.reason in the provider's terms, and the provider as the failing leg (cut-a §5.2),
// whichever provider.
func providerRefusal(reason ProviderReason, message string) error {
	failure := newDetailedError(http.StatusServiceUnavailable, message, ProviderRefusalDetailBody{Reason: reason})
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}
