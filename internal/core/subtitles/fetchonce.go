// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"context"
	"fmt"
	"sync"
)

// fetchesUnderway holds one fetch of a subtitle for a file at a time, so a second ask for it, a
// double click or another person's, waits for the first and is given what it kept rather than
// spending another download (D244 §4). The unique index on fetched_subtitle holds it as well.
type fetchesUnderway struct {
	mutex sync.Mutex
	// done is closed when the fetch underway for its key ends; guarded by mutex.
	done map[string]chan struct{}
}

func newFetchesUnderway() *fetchesUnderway {
	return &fetchesUnderway{done: map[string]chan struct{}{}}
}

// fetchKey names a subtitle fetched for a file: the file, and the provider's file.
func fetchKey(fileID int64, providerFileID string) string {
	return fmt.Sprintf("%d/%s/%s", fileID, providerOpenSubtitles, providerFileID)
}

// begin waits until no fetch of key is underway and makes this one the fetch underway, until end
// is called; an error when ctx ends first.
func (underway *fetchesUnderway) begin(ctx context.Context, key string) (func(), error) {
	for {
		underway.mutex.Lock()
		other, isUnderway := underway.done[key]
		if !isUnderway {
			done := make(chan struct{})
			underway.done[key] = done
			underway.mutex.Unlock()
			return func() {
				underway.mutex.Lock()
				defer underway.mutex.Unlock()
				delete(underway.done, key)
				close(done)
			}, nil
		}
		underway.mutex.Unlock()
		select {
		case <-other:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for the same subtitle's fetch: %w", ctx.Err())
		}
	}
}
