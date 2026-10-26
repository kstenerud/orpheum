// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"crypto/rand"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A new token forgets the tokens idle past their time, and names the films left with no playback,
// whose chunks' bookkeeping can then go (the audit of 2026-10-25, C5); a film another playback still
// plays, or the new one plays, is not named.
func TestIssue_NamesTheFilmsAnIdleTokenLeftWithNoPlayback(t *testing.T) {
	t.Parallel()
	all, now, idle := newTokens(), time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC), time.Minute
	for _, fileID := range []int64{1, 2, 3} {
		if _, unplayed, err := all.issue(rand.Reader, tokenPlayback{fileID: fileID}, now, idle); err != nil ||
			len(unplayed) != 0 {
			t.Fatalf("issuing for file %d named %v, %v; want none", fileID, unplayed, err)
		}
	}
	if _, _, err := all.issue(rand.Reader, tokenPlayback{fileID: 3}, now.Add(idle), idle); err != nil {
		t.Fatal(err) // file 3 is played again, by a playback still fresh later
	}
	_, unplayed, err := all.issue(rand.Reader, tokenPlayback{fileID: 2}, now.Add(idle+time.Second), idle)
	if err != nil || !slices.Equal(unplayed, []int64{1}) {
		t.Errorf("a new playback of file 2 once the first three idled out named %v, %v; want file 1 alone",
			unplayed, err)
	}
}

// A playback issued once another's token idled out has the chunks forget that one's film.
func TestIssueToken_HasTheChunksForgetAFilmNoPlaybackIsLeftOf(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	fake := &stoppingChunks{}
	service := &Service{clock: clock, chunks: fake, random: rand.Reader, tokens: newTokens()}
	for _, fileID := range []int64{1, 2} {
		if _, err := service.issueToken(tokenPlayback{fileID: fileID}, time.Minute); err != nil {
			t.Fatal(err)
		}
		clock.Advance(2 * time.Minute)
	}
	if !slices.Equal(fake.forgotten, []int64{1}) {
		t.Errorf("the chunks forgot %v; want film 1, whose playback idled out", fake.forgotten)
	}
}
