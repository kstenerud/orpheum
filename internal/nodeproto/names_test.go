// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import "testing"

// Every value the node protocol names is camelCase, as its keys are (the owner, 2026-11-09): the
// commands, records, skip rules, error codes, stall causes, a slot, a share's wait and a refused
// sound. FFmpeg's encoder names (h264_qsv) are FFmpeg's, and stay.
func TestNames_TheValuesTheWireSaysAreCamelCase(t *testing.T) {
	t.Parallel()
	for _, each := range []struct{ name, want string }{
		{string(CommandMakeChunks), "makeChunks"},
		{string(CommandMakeSystemVideos), "makeSystemVideos"},
		{string(CommandEncodeArtwork), "encodeArtwork"},
		{string(CommandReadThrough), "readThrough"},
		{string(CommandExtractSubtitle), "extractSubtitle"},
		{string(CommandHashWhole), "hashWhole"},
		{string(SlotReadThrough), "readThrough"},
		{string(ShareWaitsForever), "waitsForever"},
		{string(ShareMayWaitForever), "mayWaitForever"},
		{string(RecordScanBegin), "scanBegin"},
		{string(RecordScanComplete), "scanComplete"},
		{string(RecordChunksChecked), "chunksChecked"},
		{string(SkipDiscFolder), "discFolderNotSupportedYet"},
		{string(SkipSymlinkOutsideRoots), "symlinkOutsideRoots"},
		{string(SkipNotRegular), "notRegular"},
		{string(CodePermissionDenied), "permissionDenied"},
		{string(CodeNotFound), "notFound"},
		{string(CodeIOError), "ioError"},
		{string(CodeProbeFailed), "probeFailed"},
		{string(StallNeverMoved), "neverMoved"},
		{string(StallStoodStill), "stoodStill"},
		{string(StallPastDeadline), "pastDeadline"},
		{string(StallDiskFailed), "diskFailed"},
		{string(ConditionWaitingOnDisk), "waitingOnDisk"},
		{string(RefusedNoRoom), "noRoom"},
	} {
		if each.name != each.want {
			t.Errorf("the wire says %q; want %q", each.name, each.want)
		}
	}
}
