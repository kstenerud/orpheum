// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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
	// CodeCannotConvert is a play of a file its device cannot play as it is that cannot be cut into
	// chunks either: it has no picture, or neither a known length nor keyframes (D344 §5); or a chunk
	// whose runs FFmpeg failed on the same way each time (chunks.ErrCannotBeMade).
	CodeCannotConvert Code = "cannot_convert"
	// CodeNoRoomToMake is a chunk of a quality whose run was evicted to keep its machine's other runs
	// faster than playback (D250 §6): the player's playback stops, and says why.
	CodeNoRoomToMake Code = "no_room_to_make"
	// CodePlaybackGone is a playback the server no longer has: it restarted since the playback began,
	// which ends every playback, or the playback ended. The player starts it again from where it is.
	CodePlaybackGone Code = "playback_gone"
	CodeMigrating    Code = "migrating"
	// CodeInAnotherSession is a join to a Watch Together session refused because its person is in
	// another, which they have not said to leave; its detail says whose (D372 §8).
	CodeInAnotherSession Code = "in_another_session"
	// CodeReceivingACast is a cast to a device that receives another cast now, which no device may
	// take over until that cast ends (D386 §1).
	CodeReceivingACast Code = "receiving_a_cast"
	// CodeCastingHere is a cast to a device that casts to the asking one, directly or through the
	// devices it casts to: the cast back would close a loop (live.ErrCastingHere).
	CodeCastingHere Code = "casting_here"
	CodeInternal    Code = "internal"
)

// codes are every Code, in the order above: the one list the API document's ProblemBody names.
// TestProblemBody_TheDocumentNamesEveryCodeOnceAndTypesItsDetail keeps it in step with the consts.
func codes() []Code {
	return []Code{
		CodeSetupRequired, CodeAuthRequired, CodeForbidden, CodeNotFound, CodeInvalidRequest, CodeConflict,
		CodeStale, CodeRateLimited, CodeQuotaExhausted, CodeProviderError, CodeNodeSilent, CodeFileUnavailable,
		CodeMediaUnreachable, CodePreparing, CodePrepareFailed, CodeCannotConvert, CodeNoRoomToMake, CodePlaybackGone,
		CodeMigrating, CodeInAnotherSession, CodeReceivingACast, CodeCastingHere, CodeInternal,
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
		InAnotherSessionDetailBody{},
	}
}

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
