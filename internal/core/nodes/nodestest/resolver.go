// SPDX-License-Identifier: GPL-3.0-or-later

package nodestest

import (
	"context"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Resolver resolves the jobs given to a node as the server does, by a runner of their kind; and a
// job a test ran by its handler alone, not through the queue, by the answer the node's word gives
// it, which it hands the test (Start).
type Resolver struct {
	mutex   sync.Mutex
	started map[int64]startedJob // guarded by mutex: by job id
}

// startedJob is a job a test ran by its handler, and where its answer goes.
type startedJob struct {
	job    jobs.Job
	answer chan error
}

// NewResolver returns a resolver with no job started.
func NewResolver() *Resolver {
	return &Resolver{started: map[int64]startedJob{}}
}

// Of is a nodes.Resolver that answers a started job to its test, and resolves any other by
// fallback, a runner of the kind, or by nothing when it is nil.
func (resolver *Resolver) Of(fallback nodes.Resolver) nodes.Resolver {
	return resolverFunc(func(ctx context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
		resolver.mutex.Lock()
		started, isStarted := resolver.started[id]
		delete(resolver.started, id)
		resolver.mutex.Unlock()
		if isStarted {
			started.answer <- decide(started.job)
			return true, nil
		}
		if fallback == nil {
			return false, nil
		}
		return fallback.ResolveBy(ctx, id, decide)
	})
}

// Start runs job by its handler, as a runner would, and returns where its answer arrives: the
// handler's own, or, once it gave the job to a node, the answer its node's word resolves it with.
func (resolver *Resolver) Start(t *testing.T, handler jobs.Handler, job jobs.Job) <-chan error {
	t.Helper()
	answer := make(chan error, 1)
	resolver.mutex.Lock()
	resolver.started[job.ID] = startedJob{job: job, answer: answer}
	resolver.mutex.Unlock()
	go func() {
		err := handler(t.Context(), job)
		if !jobqueue.IsAssigned(err) {
			resolver.mutex.Lock()
			delete(resolver.started, job.ID)
			resolver.mutex.Unlock()
			answer <- err
		}
	}()
	return answer
}

// resolverFunc is a function as a nodes.Resolver.
type resolverFunc func(ctx context.Context, id int64, decide func(jobs.Job) error) (bool, error)

// ResolveBy is resolve.
func (resolve resolverFunc) ResolveBy(ctx context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
	return resolve(ctx, id, decide)
}
