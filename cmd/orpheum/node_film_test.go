// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A node streams its films only when its FFmpeg lists the fdproxy protocol among its inputs, and
// says at start which way its runs read their films, in its log and in its hello, so a node on the
// stock build, which reads them by path, is seen to lack the dead-share protection (D330; audit
// 2026-10-25 N9).
func TestStreamsFilms_OnlyWhenTheNodesFFmpegHasTheProtocolAndSaysWhich(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name, listing, wantSaid string
		want                    bool
	}{
		{
			"the patched build", "Supported file protocols:\nInput:\n  fdproxy\n  file\nOutput:\n  file\n",
			"through the node", true,
		},
		{
			"the stock build", "Supported file protocols:\nInput:\n  file\nOutput:\n  file\n",
			"by its path", false,
		},
	} {
		fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegStdout: example.listing})
		runner, err := execx.NewRunner(fake.Directory)
		if err != nil {
			t.Fatal(err)
		}
		var logged bytes.Buffer
		said := streamFilmsIfAble(t.Context(), runner, slog.New(slog.NewTextHandler(&logged, nil)))
		film, err := runner.FilmOf(t.Context(), "/media/film.mkv", openedFilm(t), execx.ReadsWholeFilm)
		if err != nil {
			t.Fatal(err)
		}
		got := film.Stream != nil
		if got != example.want || !strings.Contains(logged.String(), example.wantSaid) {
			t.Errorf("%s: streams films %v, said %q; want %v, saying it reads films %s", example.name, got,
				logged.String(), example.want, example.wantSaid)
		}
		hello := hardwareReport(nodeCapabilities{readsFilmsThroughNode: said})
		if hello.ReadsFilmsThroughNode != example.want {
			t.Errorf("%s: the hello says it reads films through the node: %v; want %v, so the core can see it",
				example.name, hello.ReadsFilmsThroughNode, example.want)
		}
	}
}

// openedFilm is a film opened as the node opens one: a file of the test's own, through a gate.
func openedFilm(t *testing.T) execx.OpenedFilm {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // the test's own file
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return execx.OpenedFilm{
		File: file, Facts: facts, Disks: diskwatch.NewGate(clock.System{}), Root: "/", Window: time.Minute,
	}
}
