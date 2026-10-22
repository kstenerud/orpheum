// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package execx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestRun_ThePinnedFFprobeRunsWithAnEmptyEnvironment(t *testing.T) {
	t.Parallel()
	runner, err := NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := runner.Run(t.Context(), FFprobe, confine.Access{}, []string{"-version"}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(stdout), "ffprobe version ") {
		t.Errorf("ffprobe -version said %q", stdout)
	}
}

// The pinned FFmpeg, its source going silent part way, logs that its demuxer's read failed and exits
// 0 with a loudness of what it read; the run is a stall on its disk, not that answer
// (ffmpeg-http-input-2026-10-22). The source is a loopback server, as the node's byte path will be.
func TestRunWatched_ThePinnedFFmpegWhoseSourceGoesSilentIsAStallOnItsDisk(t *testing.T) {
	t.Parallel()
	runner, err := NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	film := filepath.Join(t.TempDir(), "film.mkv")
	if _, err = runner.Run(t.Context(), FFmpeg, confine.Access{}, []string{
		"-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=60:sample_rate=48000",
		"-c:a", "flac", "file:" + film,
	}, 1<<20); err != nil {
		t.Fatal(err)
	}
	silent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, readErr := os.ReadFile(film)
		if readErr != nil {
			http.Error(writer, readErr.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
		if _, writeErr := writer.Write(content[:len(content)/3]); writeErr != nil {
			return
		}
		if flushErr := http.NewResponseController(writer).Flush(); flushErr != nil {
			return
		}
		select {
		case <-silent:
		case <-request.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(silent) })
	_, err = runner.RunWatched(t.Context(), WatchedRun{
		Arguments: []string{
			"-nostats", "-protocol_whitelist", "http,tcp", "-rw_timeout", "1000000", "-i", server.URL + "/film.mkv",
			"-filter_complex", "[0:0]ebur128=framelog=quiet[o]", "-map", "[o]", "-f", "null", "-",
		},
		LogLimitBytes: 4096,
		Watch:         Watch{StartGrace: time.Minute, StallWindow: time.Minute, Deadline: 2 * time.Minute},
	})
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || stalled.Cause != StallDiskFailed {
		t.Errorf("a run whose source went silent ended %v; want a stall on its disk", err)
	}
}
