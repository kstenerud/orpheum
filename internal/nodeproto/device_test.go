// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

import "testing"

// A path is on the device of the longest mount path it is under; a disk mounted inside a root is its
// own (D240 §5).
func TestDevicesKeyOf_IsTheDeviceOfTheLongestMountPathAPathIsUnder(t *testing.T) {
	t.Parallel()
	devices := Devices{
		{Key: "zfs:tank", MountPaths: []string{"/media/films", "/media/shows"}},
		{Key: "disk:sdb", MountPaths: []string{"/media/films/Extra"}},
	}
	for path, want := range map[string]string{
		"/media/films/Wing/Wing.mkv":  "zfs:tank",
		"/media/films/Extra/Big.mkv":    "disk:sdb",
		"/media/films/Extras/Small.mkv": "zfs:tank",
		"/media/shows/Show/S01E01.mkv":  "zfs:tank",
		"/srv/elsewhere/not-a-root.mkv": "",
	} {
		if got := devices.KeyOf(path); got != want {
			t.Errorf("%s is on %q, want %q", path, got, want)
		}
	}
}

// The media file a command reads is its video's, or a sidecar subtitle's; a listing's is where it
// lists, an examining's its first file, by its raw bytes; work that reads none, an artwork batch or a
// fetched subtitle's conversion, names none.
func TestMediaFile_IsTheFileACommandReads(t *testing.T) {
	t.Parallel()
	video := BytesFile{RootPath: "/media", Path: []byte("a.mkv")}
	sidecar := BytesFile{RootPath: "/media", Path: []byte("a.srt")}
	for name, each := range map[string]struct {
		work Work
		want string
	}{
		"prepare":  {Work{Prepare: &Prepare{Video: video}}, "a.mkv"},
		"extract":  {Work{ExtractSubtitle: &ExtractSubtitle{Video: &video}}, "a.mkv"},
		"sidecar":  {Work{ExtractSubtitle: &ExtractSubtitle{Sidecar: &sidecar}}, "a.srt"},
		"read":     {Work{ReadThrough: &ReadThrough{Video: video}}, "a.mkv"},
		"artwork":  {Work{EncodeArtwork: &ArtworkBatch{}}, ""},
		"converts": {Work{ExtractSubtitle: &ExtractSubtitle{}}, ""},
		"list":     {Work{List: &List{RootPath: "/media", Path: "Films"}}, "Films"},
		"examine": {Work{Examine: &Examine{RootPath: "/media", Files: []ExamineFile{
			{Path: Path{Text: "b?.mkv", Raw: []byte("b\xff.mkv")}}, {Path: Path{Text: "c.mkv"}},
		}}}, "b\xff.mkv"},
		"examine nothing": {Work{Examine: &Examine{RootPath: "/media"}}, ""},
	} {
		file, isMedia := each.work.MediaFile()
		if string(file.Path) != each.want || isMedia != (each.want != "") {
			t.Errorf("%s reads %q (%v), want %q", name, file.Path, isMedia, each.want)
		}
	}
}
