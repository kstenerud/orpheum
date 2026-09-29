// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package hash

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestPayloadIdentity_D179TableWithTheRealTools is D179's measurement as a test (cut-a §8, M2): a file
// authored by mkvmerge keeps its identity through every in-place mkvpropedit edit, and gets a
// new one from an ffmpeg -c copy remux. The source is made by the pinned FFmpeg, never taken
// from a library (D203).
func TestPayloadIdentity_D179TableWithTheRealTools(t *testing.T) {
	t.Parallel()
	tools := testutil.RequireMKVToolNix(t)
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	inTest := func(name string) string { return filepath.Join(directory, name) }
	ffmpeg := func(arguments ...string) {
		t.Helper()
		quiet := append([]string{"-v", "error"}, arguments...)
		_, runErr := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, quiet, 1<<20)
		if runErr != nil {
			t.Fatal(runErr)
		}
	}

	// Twenty seconds of video and audio: a few megabytes, so the two 1 MB windows leave some
	// of the payload unread, as they do in a film.
	ffmpeg("-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=20", "-f", "lavfi", "-i", "sine=duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-c:a", "aac", "file:"+inTest("source.mp4"))
	tools.Run(t, "mkvmerge", "-q", "-o", inTest("film.mkv"), inTest("source.mp4"))
	original := identityOfFile(t, inTest("film.mkv"))
	if original.Anchor != AnchorMatroska {
		t.Fatalf("anchored on %s, want the Matroska clusters", original.Anchor)
	}

	var tags strings.Builder
	tags.WriteString(`<?xml version="1.0"?><Tags>`)
	for index := range 300 {
		fmt.Fprintf(&tags, "<Tag><Simple><Name>K%d</Name><String>value %d</String></Simple></Tag>", index, index)
	}
	tags.WriteString("</Tags>")
	writeTestFile(t, inTest("tags.xml"), []byte(tags.String()))
	writeTestFile(t, inTest("attachment.bin"), make([]byte, 300_000))

	edits := map[string][]string{
		"set title":                {"--edit", "info", "--set", "title=Short"},
		"set a 6 KB title":         {"--edit", "info", "--set", "title=" + strings.Repeat("x", 6000)},
		"add 300 global tags":      {"--tags", "global:" + inTest("tags.xml")},
		"add a 300 KB attachment":  {"--add-attachment", inTest("attachment.bin")},
		"rename and relabel track": {"--edit", "track:a1", "--set", "name=Renamed", "--set", "language=ger"},
	}
	for name, arguments := range edits {
		edited := inTest(strings.ReplaceAll(name, " ", "-") + ".mkv")
		copyTestFile(t, inTest("film.mkv"), edited)
		tools.Run(t, "mkvpropedit", append([]string{"-q", edited}, arguments...)...)
		if got := identityOfFile(t, edited); got != original {
			t.Errorf("%s: the identity changed (%x → %x)", name, original.PayloadHash[:8], got.PayloadHash[:8])
		}
	}

	ffmpeg("-i", "file:"+inTest("film.mkv"), "-c", "copy", "-map", "0", "file:"+inTest("remuxed.mkv"))
	if got := identityOfFile(t, inTest("remuxed.mkv")); got.PayloadHash == original.PayloadHash {
		t.Error("an ffmpeg -c copy remux kept the identity; D179 says a remux is a new file")
	}
}

func identityOfFile(t *testing.T, path string) Identity {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := PayloadIdentity(file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyTestFile(t *testing.T, from, destination string) {
	t.Helper()
	source, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }() //nolint:errcheck // read-only
	target, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(target, source); err != nil {
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
}
