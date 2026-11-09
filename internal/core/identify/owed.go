// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

// owedPasses are the libraries a match or a refresh left owing a pass, each marked once however
// many asked: a burst of series fetches is followed by one pass of their library, not one each
// (D303, DL14). The marks live in memory: a stop loses them, and the passes every library gets
// at start, and on derivedWorkSweepEverySeconds after, are their backstop.
type owedPasses struct {
	mutex     sync.Mutex
	libraries map[int64]bool // guarded by mutex
	wake      func()         // set before the passes run
}

// OnPassOwed calls wake when a library is marked as owing a pass, for the owed passes to run soon:
// the job queue's request for their sweep. Set it before the pass runs.
func (pass *Pass) OnPassOwed(wake func()) {
	pass.owed.wake = wake
}

// OwePass marks a library as owing a pass, which IdentifyOwed runs, and wakes what runs it.
func (pass *Pass) OwePass(libraryID int64) {
	pass.owed.mutex.Lock()
	if pass.owed.libraries == nil {
		pass.owed.libraries = map[int64]bool{}
	}
	pass.owed.libraries[libraryID] = true
	pass.owed.mutex.Unlock()
	if pass.owed.wake != nil {
		pass.owed.wake()
	}
}

// IdentifyOwed runs the pass of each library marked as owing one, once each, and says how many
// ran. A library that fails is logged, as IdentifyAll logs it; one a cancel left unrun stays
// owed.
func (pass *Pass) IdentifyOwed(ctx context.Context) (int, error) {
	pass.owed.mutex.Lock()
	libraryIDs := make([]int64, 0, len(pass.owed.libraries))
	for libraryID := range pass.owed.libraries {
		libraryIDs = append(libraryIDs, libraryID)
	}
	pass.owed.libraries = nil
	pass.owed.mutex.Unlock()
	slices.Sort(libraryIDs)
	passed := 0
	for index, libraryID := range libraryIDs {
		if _, err := pass.IdentifyLibrary(ctx, libraryID); err != nil {
			if ctx.Err() != nil {
				pass.oweAgain(libraryIDs[index:])
				return passed, fmt.Errorf("identify library %d: %w", libraryID, ctx.Err())
			}
			pass.logger.ErrorContext(ctx, "a library's owed pass failed", "libraryId", libraryID, "err", err)
			continue
		}
		passed++
	}
	return passed, nil
}

// oweAgain marks again the libraries a cancelled run left unrun, without waking anything.
func (pass *Pass) oweAgain(libraryIDs []int64) {
	pass.owed.mutex.Lock()
	defer pass.owed.mutex.Unlock()
	if pass.owed.libraries == nil {
		pass.owed.libraries = map[int64]bool{}
	}
	for _, libraryID := range libraryIDs {
		pass.owed.libraries[libraryID] = true
	}
}
