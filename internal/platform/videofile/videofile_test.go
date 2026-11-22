// SPDX-License-Identifier: AGPL-3.0-or-later

package videofile_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/videofile"
)

func TestIsVideo_IsAVideoExtensionOrADiscsImageButNoSampleOrAppleDouble(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"Arkon (1979).mkv": true, "Arkon (1979).MKV": true, "clip.mp4": true, "old.rmvb": true, "a.iso": true,
		"Arkon (1979).nfo": false, "Arkon (1979).srt": false, "poster.jpg": false, "notes.txt": false, "mkv": false,
		"._Arkon (1979).mkv": false, "sample.mkv": false, "x.sample.mkv": false, "x-SAMPLE.mp4": false,
	} {
		if got := videofile.IsVideo(name); got != want {
			t.Errorf("IsVideo(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsSkippedFolder_IsTheTrashASampleASystemOrADiscsFolder(t *testing.T) {
	t.Parallel()
	for relative, want := range map[string]bool{
		".trash": true, "films/.trash": false, "Arkon/Sample": true, "@eaDir": true, "films/#recycle": true,
		"lost+found": true, "Arkon/VIDEO_TS": true, "Arkon/bdmv": true, "films": false, "Arkon (1979)": false,
		".hidden": false,
	} {
		if got := videofile.IsSkippedFolder(relative); got != want {
			t.Errorf("IsSkippedFolder(%q) = %v, want %v", relative, got, want)
		}
	}
}

func TestExtensions_IsACopyTheCallerMayChange(t *testing.T) {
	t.Parallel()
	videofile.Extensions()["mkv"] = false
	if !videofile.IsVideo("Arkon.mkv") {
		t.Error("changing the extensions given changed what is a video")
	}
}
