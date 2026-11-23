// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/artwork"
	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeFilms makes a film of the file opened by its path, saying how it was asked to read it.
type fakeFilms struct {
	mutex    sync.Mutex
	readings []execx.FilmReading
}

func (films *fakeFilms) FilmOf(
	_ context.Context, path string, opened execx.OpenedFilm, reading execx.FilmReading,
) (execx.Film, error) {
	films.mutex.Lock()
	defer films.mutex.Unlock()
	films.readings = append(films.readings, reading)
	return execx.Film{Path: path, Size: opened.Facts.Size()}, nil
}

// fakeFrame is one ask of fakeFrames.
type fakeFrame struct {
	film        execx.Film
	streamIndex int
	at          time.Duration
}

// fakeFrames answers each key frame with frame, or err, after release closes when it is set.
type fakeFrames struct {
	mutex   sync.Mutex
	asks    []fakeFrame
	frame   []byte
	err     error
	started chan struct{}
	release chan struct{}
}

func (frames *fakeFrames) KeyFrame(
	ctx context.Context, film execx.Film, streamIndex int, at time.Duration,
) ([]byte, error) {
	frames.mutex.Lock()
	frames.asks = append(frames.asks, fakeFrame{film: film, streamIndex: streamIndex, at: at})
	frames.mutex.Unlock()
	if frames.release != nil {
		frames.started <- struct{}{}
		select {
		case <-frames.release:
		case <-ctx.Done():
		}
	}
	return frames.frame, frames.err
}

func (frames *fakeFrames) asked() []fakeFrame {
	frames.mutex.Lock()
	defer frames.mutex.Unlock()
	return append([]fakeFrame(nil), frames.asks...)
}

// frameTarget is the frame route's target for movie's stream at a place, naming payloadHash.
func frameTarget(movie film, payloadHash string, streamIndex int, atMs int64) string {
	return nodeproto.FrameRequest{
		Video: nodeproto.BytesFile{
			RootPath: movie.root.Path(), Path: []byte(movie.path), PayloadHash: payloadHash,
		},
		StreamIndex: streamIndex, AtMs: atMs,
	}.Target()
}

// A key frame is made of the file the payload hash names, read as a probe reads a film, of the stream
// and at the place asked, and answered as the WebP it is (D545).
func TestServeFrame_MakesTheKeyFrameOfTheFileItsHashNamesAtThePlace(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "Films/Film (1999)/Film (1999).mkv")
	films, frames := &fakeFilms{}, &fakeFrames{frame: []byte("a webp")}
	handler := byteserver.New([]*confine.MediaRoot{movie.root}).ServingFrames(films, frames).Handler()
	got := request(t, handler, http.MethodGet, frameTarget(movie, movie.payloadHash, 2, 754_250), nil)
	if got.Code != http.StatusOK || got.Body.String() != "a webp" || got.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("GET = %d %q (%s), want 200 with the key frame as image/webp",
			got.Code, got.Body.String(), got.Header().Get("Content-Type"))
	}
	asks := frames.asked()
	want := fakeFrame{
		film:        execx.Film{Path: movie.root.Path() + "/" + movie.path, Size: int64(len(movie.contents))},
		streamIndex: 2, at: 754*time.Second + 250*time.Millisecond,
	}
	if len(asks) != 1 || asks[0] != want {
		t.Errorf("key frames asked %+v, want one: %+v", asks, want)
	}
	if len(films.readings) != 1 || films.readings[0] != execx.ReadsIndex {
		t.Errorf("films read as %v, want once, as a probe reads one (execx.ReadsIndex)", films.readings)
	}
}

// A file that is not the one named, a place or stream that is not a whole number from 0, a video with
// no picture there, a node that makes no frames and a failed run each answer what they are, and none
// but the last two makes a frame.
func TestServeFrame_RefusesWhatItCannotMakeAFrameOf(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "Films/Film (1999)/Film (1999).mkv")
	roots := []*confine.MediaRoot{movie.root}
	makes := &fakeFrames{frame: []byte("a webp")}
	handler := byteserver.New(roots).ServingFrames(&fakeFilms{}, makes).Handler()
	otherHash := strings.Repeat("0", len(movie.payloadHash))
	for name, target := range map[string]string{
		"another file's hash": frameTarget(movie, otherHash, 0, 0),
		"a place before 0":    frameTarget(movie, movie.payloadHash, 0, -1),
		"a stream before 0":   frameTarget(movie, movie.payloadHash, -1, 0),
		"no place":            strings.Replace(frameTarget(movie, movie.payloadHash, 0, 0), "atMs=0", "atMs=", 1),
	} {
		want := map[bool]int{true: http.StatusConflict, false: http.StatusBadRequest}[name == "another file's hash"]
		if got := request(t, handler, http.MethodGet, target, nil); got.Code != want {
			t.Errorf("%s: GET = %d, want %d", name, got.Code, want)
		}
	}
	if asks := makes.asked(); len(asks) != 0 {
		t.Errorf("key frames asked %+v, want none for what is refused", asks)
	}
	for name, answer := range map[string]struct {
		server *byteserver.Server
		want   int
	}{
		"no picture there": {
			byteserver.New(roots).ServingFrames(&fakeFilms{}, &fakeFrames{err: artwork.ErrNoKeyFrame}),
			http.StatusNotFound,
		},
		"a failed run": {
			byteserver.New(roots).ServingFrames(&fakeFilms{}, &fakeFrames{err: errors.New("exit 1")}),
			http.StatusInternalServerError,
		},
		"no frames made here": {byteserver.New(roots), http.StatusNotFound},
	} {
		got := request(t, answer.server.Handler(), http.MethodGet, frameTarget(movie, movie.payloadHash, 0, 0), nil)
		if got.Code != answer.want {
			t.Errorf("%s: GET = %d, want %d", name, got.Code, answer.want)
		}
	}
}

// One key frame is made at a time: an ask while one is being made waits its turn, and one whose
// asker stops waiting first makes none.
func TestServeFrame_MakesOneAtATime(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "Films/Film (1999)/Film (1999).mkv")
	frames := &fakeFrames{frame: []byte("a webp"), started: make(chan struct{}), release: make(chan struct{})}
	handler := byteserver.New([]*confine.MediaRoot{movie.root}).ServingFrames(&fakeFilms{}, frames).Handler()
	target := frameTarget(movie, movie.payloadHash, 0, 0)
	first := make(chan int)
	go func() { first <- request(t, handler, http.MethodGet, target, nil).Code }()
	testutil.Receive(t, frames.started, "the first key frame to start")
	ctx, stop := context.WithCancel(t.Context())
	stop()
	waiting := httptest.NewRecorder()
	handler.ServeHTTP(waiting, httptest.NewRequestWithContext(ctx, http.MethodGet, target, nil))
	close(frames.release)
	if code := testutil.Receive(t, first, "the first ask's answer"); code != http.StatusOK {
		t.Errorf("the first ask = %d, want 200", code)
	}
	if asks := frames.asked(); len(asks) != 1 {
		t.Errorf("key frames asked %d times, want once: the second waited while the first was made, and "+
			"its asker stopped waiting", len(asks))
	}
}
