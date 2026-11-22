// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import "github.com/kstenerud/orpheum/internal/platform/videofile"

// The extensions a scan reads are the code's, not settings (D360, amending D240 §8); videofile says
// why. An extension is lower case and without the dot.

// videoExtensions are the extensions of the files a scan reads as videos: videofile's, the one list
// the node's folder listing counts a folder's videos by too (D539 §7).
func videoExtensions() map[string]bool { return videofile.Extensions() }

// subtitleExtensions are the extensions of the text subtitle files a scan reads beside their videos:
// the formats the node converts to ASS and WebVTT (D217 §6), SubRip, ASS and SSA, and WebVTT. Image
// subtitles are read inside a video, not beside it.
func subtitleExtensions() map[string]bool {
	return setOf("srt", "ass", "ssa", "vtt")
}

// scanRules are the rules a scan of any library reads by.
func scanRules() ScanRules {
	return ScanRules{VideoExtensions: videoExtensions(), SubtitleExtensions: subtitleExtensions()}
}

func setOf(extensions ...string) map[string]bool {
	set := make(map[string]bool, len(extensions))
	for _, extension := range extensions {
		set[extension] = true
	}
	return set
}
