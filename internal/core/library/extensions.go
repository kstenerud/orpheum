// SPDX-License-Identifier: AGPL-3.0-or-later

package library

// The extensions a scan reads are the code's, not settings (D360, amending D240 §8): which
// containers FFmpeg reads is no admin's judgement, and a stored list kept a container added later
// from reaching an install that had one. An extension is lower case and without the dot.

// videoExtensions are the extensions of the files a scan reads as videos: the containers people keep
// films in, Matroska and MP4 first, the broadcast and disc formats, and the older ones still found
// in libraries, RealMedia among them (the owner, 2026-10-27). A file with another extension is never
// probed, so a missing one hides films; one also used by files that are no video costs a probe of
// each. Those D360 added are each proven listed, probed, decided and their first chunk made by the
// integration tier's TestPlayback_EachContainerAddedIsReadProbedDecidedAndItsFirstChunkMade.
func videoExtensions() map[string]bool {
	return setOf(
		"mkv", "mp4", "m4v", "avi", "divx", "mov", "wmv", "ts", "m2ts", "mts", "webm", "mpg", "mpeg", "vob",
		"flv", "ogv", "3gp",
		"rm", "rmvb",
	)
}

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
