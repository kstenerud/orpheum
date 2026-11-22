// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/videofile"
)

// A scan reads as a video what videofile calls one, and skips the folders it does, so the node's
// count of a folder's videos counts what a scan of it would read (D539 §7).
func TestKindOf_ReadsAsAVideoWhatVideofileCallsOne(t *testing.T) {
	t.Parallel()
	rules := scanRules()
	for _, name := range []string{
		"Arkon (1979).mkv", "Arkon (1979).MP4", "old.rmvb", "a.iso", "Arkon (1979).nfo", "Arkon (1979).srt",
		"poster.jpg", "notes.txt", "._Arkon (1979).mkv", "sample.mkv", "x.sample.mkv", "x-sample.avi",
		"a.ogg", "noextension",
	} {
		kind, _ := rules.kindOf(name)
		if isVideo := kind == nodeproto.ExamineVideo; isVideo != videofile.IsVideo(name) {
			t.Errorf("%q: a scan reads it as a video %v, videofile says %v", name, isVideo, videofile.IsVideo(name))
		}
	}
	for _, relative := range []string{
		".trash", "films/.trash", "Arkon/sample", "@eaDir", "films/#recycle", "Arkon/VIDEO_TS", "Arkon/BDMV",
		"films", ".hidden",
	} {
		if _, isSkipped := directoryRule(relative); isSkipped != videofile.IsSkippedFolder(relative) {
			t.Errorf("%q: a scan skips it %v, videofile says %v", relative, isSkipped,
				videofile.IsSkippedFolder(relative))
		}
	}
}
