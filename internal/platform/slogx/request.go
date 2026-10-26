// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx

import (
	"context"
	"log/slog"
)

// RequestIDKey is the attribute every line logged with a request's context carries: the request's
// ID, which its answer names too (D347).
const RequestIDKey = "requestId"

// requestIDOf is the context key of a request's ID.
type requestIDOf struct{}

// WithRequestID is ctx carrying a request's ID, which every line logged with it then carries.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDOf{}, id)
}

// RequestID is the ID of the request ctx belongs to, or "" when it belongs to none.
func RequestID(ctx context.Context) string {
	if id, isID := ctx.Value(requestIDOf{}).(string); isID {
		return id
	}
	return ""
}

// requestHandler adds the request's ID to every record logged with a request's context.
type requestHandler struct {
	slog.Handler
}

// Handle adds the request's ID, when ctx carries one, and hands the record on.
func (handler requestHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := RequestID(ctx); id != "" {
		record = record.Clone()
		record.AddAttrs(slog.String(RequestIDKey, id))
	}
	return handler.Handler.Handle(ctx, record)
}

// WithAttrs keeps adding request IDs below the attributes.
func (handler requestHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestHandler{handler.Handler.WithAttrs(attrs)}
}

// WithGroup keeps adding request IDs below the group.
func (handler requestHandler) WithGroup(name string) slog.Handler {
	return requestHandler{handler.Handler.WithGroup(name)}
}
