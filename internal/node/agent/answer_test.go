// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// artworkCommand is a re-encode of source as job's one attempt.
func artworkCommand(seq, job int64, source string) nodeproto.Command {
	return nodeproto.Command{
		CommandSeq: seq, Type: nodeproto.CommandEncodeArtwork, JobID: job, EncodeArtwork: &nodeproto.Artwork{
			SourceImage: []byte(source), Shape: nodeproto.ShapePortrait,
			SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		},
	}
}

// A result is sent until the core acknowledges it, however long that takes, and never given up
// (D234 §5): here the core refuses it many times, for longer than the node once kept trying.
func TestAgent_AResultIsSentUntilTheCoreAcknowledgesIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}},
		func(node *Agent) {
			node.answerRetry = answerRetry{firstWait: time.Millisecond, longestWait: 2 * time.Millisecond}
		})
	core.refuseFirstPosts(artworkResult, 600)
	core.commands <- artworkCommand(1, 5, "abc")
	if result := testutil.Receive(t, core.artwork, "the answer, once taken"); result.JobID != 5 {
		t.Errorf("the core took job %d's answer, want job 5's", result.JobID)
	}
}

// An unacknowledged result is sent again at once when the node says hello again, not after its
// wait: a core that came back hears it first.
func TestAgent_AnUnacknowledgedResultIsSentAgainAtTheNextHello(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	_, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}},
		func(node *Agent) { node.answerRetry = answerRetry{firstWait: time.Hour, longestWait: time.Hour} })
	testutil.Receive(t, core.helloHeard, "the first hello")
	core.refuseFirstPosts(artworkResult, 1)
	core.commands <- artworkCommand(1, 5, "abc")
	testutil.Receive(t, core.refusedPosts, "the refused post")
	core.endSession <- struct{}{}
	// The ended session's status loop may take a tick before it stops: tick until the next hello.
	ticking, stopTicking := context.WithCancel(t.Context())
	defer stopTicking()
	go func() {
		for {
			select {
			case tick <- time.Time{}:
			case <-ticking.Done():
				return
			}
		}
	}()
	testutil.Receive(t, core.helloHeard, "the next hello")
	if result := testutil.Receive(t, core.artwork, "the answer, sent again"); result.JobID != 5 {
		t.Errorf("after the hello the core took job %d's answer, want job 5's", result.JobID)
	}
}

// A command the core sends again for an attempt that ended here is not run again (D238 §2): its
// result was sent, and the next answer the core hears is the next job's.
func TestAgent_ACommandSentAgainForAnAttemptThatEndedIsNotRunAgain(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}},
		func(node *Agent) { node.workers.EndedWorkKept = time.Hour })
	core.commands <- artworkCommand(1, 5, "abc")
	if first := testutil.Receive(t, core.artwork, "job 5's answer"); first.JobID != 5 {
		t.Fatalf("the first answer was job %d's, want job 5's", first.JobID)
	}
	core.commands <- artworkCommand(2, 5, "abc")
	core.commands <- artworkCommand(3, 6, "def")
	if next := testutil.Receive(t, core.artwork, "the next answer"); next.JobID != 6 {
		t.Errorf("the next answer was job %d's, want job 6's: job 5's attempt ended and is not run again",
			next.JobID)
	}
}
