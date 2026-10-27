// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/artwork"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// stillAsked is what a still was asked of: the film it was given, its stream and its place.
type stillAsked struct {
	film        execx.Film
	streamIndex int
	at          time.Duration
}

// fakeStillTaker answers each still with its film's base name, and "black.mkv" with no frame.
type fakeStillTaker struct{ asked chan stillAsked }

func (taker fakeStillTaker) Still(
	_ context.Context, film execx.Film, streamIndex int, at time.Duration,
) ([]byte, error) {
	taker.asked <- stillAsked{film: film, streamIndex: streamIndex, at: at}
	if filepath.Base(film.Path) == "black.mkv" {
		return nil, artwork.ErrNoFrame
	}
	return []byte(filepath.Base(film.Path)), nil
}

// A frame still is taken from the very video the core named, checked as any run's film is, at the
// stream and the place the command says; it is re-encoded as an image is and named by its own hash,
// and a video with no frame says so (D362 §1, §5).
func TestAgent_TakesAStillFromTheVideoTheCoreNamedAndReEncodesIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	taker := fakeStillTaker{asked: make(chan stillAsked, 2)}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, StillTaker: taker,
	})
	answer := func(job int64, name string) nodeproto.ArtworkResult {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte("a video "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		core.commands <- nodeproto.Command{
			CommandSeq: job, JobID: job,
			Work: nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(&nodeproto.Artwork{
				Key: "0", Shape: nodeproto.ShapeLandscape, SizeNames: []nodeproto.ArtSizeName{nodeproto.LandscapeS},
				Video: &nodeproto.BytesFile{
					RootPath: root.Path(), Path: []byte(name), PayloadHash: nodetest.PayloadHashHex(t, "a video "+name),
				},
				FrameStreamIndex: 1, FrameAtMs: 90_000,
			})},
		}
		return onlyImage(t, testutil.Receive(t, core.artwork, "the artwork answer"))
	}
	made := answer(1, "film.mkv")
	asked := testutil.Receive(t, taker.asked, "the still asked for")
	if asked.film.Path != root.Path()+"/film.mkv" || asked.streamIndex != 1 || asked.at != 90*time.Second {
		t.Errorf("the still was asked of %+v; want film.mkv's stream 1 at 90 s", asked)
	}
	sum := sha256.Sum256([]byte("film.mkv"))
	if made.Refusal != "" || made.HasNoFrame || len(made.Images) != 1 || string(made.Images[0].WebP) != "vkm.mlif" ||
		made.SourceSha256 != hex.EncodeToString(sum[:]) {
		t.Errorf("a still: %+v; want it re-encoded and named by its own hash", made)
	}
	none := answer(2, "black.mkv")
	testutil.Receive(t, taker.asked, "the second still asked for")
	if !none.HasNoFrame || none.Refusal == "" || len(none.Images) != 0 {
		t.Errorf("a video with no frame: %+v; want it said, with no image", none)
	}
}
