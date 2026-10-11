// SPDX-License-Identifier: AGPL-3.0-or-later

package providers

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Importance is how much a request to an outside host matters: who waits on its answer (D231
// §5). A provider weighs it: background work keeps below its share of the host's rate and never
// spends what is held back for people, and someone waiting always gets a try.
type Importance int

// The importances, least first.
const (
	// Background is the server's own work: a first match, a rolling refresh, an image owed.
	Background Importance = iota
	// PersonsRequest is work a person asked for and will look for later: a refresh.
	PersonsRequest
	// SomeoneWaiting is a person waiting on the answer now: the chooser's search, a subtitle's
	// search or download.
	SomeoneWaiting
)

// String is the importance as the API names it.
func (importance Importance) String() string {
	switch importance {
	case Background:
		return "background"
	case PersonsRequest:
		return "persons_request"
	case SomeoneWaiting:
		return "someone_waiting"
	}
	return "unknown"
}

// importanceContextKey carries a request's importance in its context.
type importanceContextKey struct{}

// WithImportance is ctx carrying how much the requests made under it matter.
func WithImportance(ctx context.Context, importance Importance) context.Context {
	return context.WithValue(ctx, importanceContextKey{}, importance)
}

// ImportanceOf is how much a request under ctx matters: what WithImportance said; otherwise
// background within a job, and someone waiting outside one, where a person's request is answered
// while they wait.
func ImportanceOf(ctx context.Context) Importance {
	if importance, isSet := ctx.Value(importanceContextKey{}).(Importance); isSet {
		return importance
	}
	if jobqueue.IsInJob(ctx) {
		return Background
	}
	return SomeoneWaiting
}

// JobImportance is how much a job's requests matter, by its priority: someone waiting to watch,
// a person's request, or the server's own work.
func JobImportance(priority int64) Importance {
	switch {
	case priority >= jobs.PriorityPlayNow:
		return SomeoneWaiting
	case priority >= jobs.PriorityPersonsRequest:
		return PersonsRequest
	}
	return Background
}
