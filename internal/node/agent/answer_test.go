// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// artworkCommand is a re-encode of source as job's one attempt.
func artworkCommand(seq, job int64, source string) nodeproto.Command {
	return nodeproto.Command{
		CommandSeq: seq,
		JobID:      job,
		Work: nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(&nodeproto.Artwork{
			SourceImage: []byte(source), Shape: nodeproto.ShapePortrait,
			SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		})},
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
	result := testutil.Receive(t, core.artwork, "the answer, once taken")
	if result.JobID != 5 {
		t.Errorf("the core took job %d's answer, want job 5's", result.JobID)
	}
	// The answer names its work, as the node holds it, without the image it carried: what the
	// core records it by when no job waits for it any more.
	if echoed := result.Work; echoed == nil || echoed.Type != nodeproto.CommandEncodeArtwork ||
		echoed.EncodeArtwork == nil || len(echoed.EncodeArtwork.Images) != 1 ||
		echoed.EncodeArtwork.Images[0].Shape != nodeproto.ShapePortrait ||
		echoed.EncodeArtwork.Images[0].SourceImage != nil {
		t.Errorf("the answer echoed %+v, want job 5's work without its image", echoed)
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

// A result too large to post is held for the core to fetch from the byte socket (D234 §5): the post
// names its job, attempt, work, size and hash, the node holds its JSON until the core acknowledges
// it, and forgets it then.
func TestAgent_AResultTooLargeToPostIsHeldUntilAcknowledged(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	held := NewHeldResults()
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}, HeldResults: held})
	core.refuseFirstPosts(heldResult, 1)
	core.commands <- artworkCommand(1, 5, strings.Repeat("x", nodeproto.MaxPostedResultBytes))
	testutil.Receive(t, core.refusedPosts, "the refused post")
	encoded, isHeld := held.Result(tokenOf(5))
	if !isHeld {
		t.Fatal("the node holds no result for job 5's attempt while the core has not taken it")
	}
	var whole nodeproto.Result
	if err := json.Unmarshal(encoded, &whole); err != nil || len(onlyImage(t, whole).Images) != 1 {
		t.Fatalf("the held result = %+v, %v; want job 5's whole answer", whole, err)
	}
	posted := testutil.Receive(t, core.held, "the post naming the held result")
	sum := sha256.Sum256(encoded)
	if posted.JobID != 5 || posted.Artwork != nil || posted.Work == nil || posted.Held == nil ||
		posted.Held.SizeBytes != int64(len(encoded)) || posted.Held.Sha256 != hex.EncodeToString(sum[:]) {
		t.Errorf("the post = %+v, want job 5's attempt and work, naming the held result's size and hash", posted)
	}
	// Once taken, it is forgotten, as soon as the node reads the core's answer to the post above.
	deadline := time.Now().Add(testutil.ReceiveLimit) //nolint:forbidigo // a real request, not the fake clock
	for {
		if _, stillHeld := held.Result(tokenOf(5)); !stillHeld {
			break
		}
		if time.Now().After(deadline) { //nolint:forbidigo // as above
			t.Fatal("the node still holds job 5's result the core took")
		}
		time.Sleep(time.Millisecond)
	}
}

// oneImage is a batch of one image.
func oneImage(image *nodeproto.Artwork) *nodeproto.ArtworkBatch {
	return &nodeproto.ArtworkBatch{Images: []nodeproto.Artwork{*image}}
}

// onlyImage is the one image an artwork answer answers for, which fails the test unless there is
// exactly one.
func onlyImage(t *testing.T, result nodeproto.Result) nodeproto.ArtworkResult {
	t.Helper()
	if result.Artwork == nil || len(result.Artwork.Images) != 1 {
		t.Fatalf("the artwork answer = %+v, want one image's", result.Artwork)
	}
	return result.Artwork.Images[0]
}

// An artwork job's images are re-encoded each in turn and answered once for all, each answer naming
// its image's key: one refused does not keep the others from being made (the owner, 2026-10-02:
// artwork as a scan is).
func TestAgent_ReEncodesAJobsImagesAndAnswersOnceForAll(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	outside := &nodeproto.Path{Text: filepath.Join(t.TempDir(), "secret.jpg")}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 5, Work: nodeproto.Work{
		Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: &nodeproto.ArtworkBatch{Images: []nodeproto.Artwork{
			{
				Key: "0", SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			},
			{
				Key: "1", AbsolutePath: outside, Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			},
		}},
	}}
	result := testutil.Receive(t, core.artwork, "job 5's one answer")
	if result.Artwork == nil || len(result.Artwork.Images) != 2 {
		t.Fatalf("the answer = %+v, want both images answered at once", result.Artwork)
	}
	made, refused := result.Artwork.Images[0], result.Artwork.Images[1]
	if made.Key != "0" || len(made.Images) != 1 || string(made.Images[0].WebP) != "cba" || made.Refusal != "" {
		t.Errorf("the first image's answer = %+v, want key 0, its size made", made)
	}
	if refused.Key != "1" || refused.Refusal == "" || len(refused.Images) != 0 {
		t.Errorf("the second image's answer = %+v, want key 1, refused: it is under none of the node's roots", refused)
	}
}

// An attempt whose result the core has not acknowledged is listed as ended, for the status, until
// it is; then it is no longer listed.
func TestEndedAttempts_ListTheResultsNotYetAcknowledged(t *testing.T) {
	t.Parallel()
	ended := newEndedAttempts()
	ended.awaitAcknowledgement(attempt{jobID: 5, token: "a5"})
	listed := ended.awaitingAcknowledgement()
	if len(listed) != 1 || listed[0].JobID != 5 || listed[0].AttemptToken != "a5" ||
		listed[0].State != nodeproto.HeldEnded {
		t.Errorf("listed %+v, want job 5's attempt, ended", listed)
	}
	ended.acknowledge("a5", time.Time{}, time.Hour)
	if listed := ended.awaitingAcknowledgement(); len(listed) != 0 {
		t.Errorf("listed %+v once acknowledged, want nothing", listed)
	}
}
