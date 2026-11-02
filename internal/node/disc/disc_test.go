// SPDX-License-Identifier: AGPL-3.0-or-later

package disc_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
)

// served is the title's stream as the node serves it: the image's runs Locate names, joined.
func served(t *testing.T, image []byte, title disc.Title) string {
	t.Helper()
	pieces, err := title.Locate(0, title.SizeBytes())
	if err != nil {
		t.Fatal(err)
	}
	var stream []byte
	for _, piece := range pieces {
		stream = append(stream, image[piece.Offset:piece.Offset+piece.Length]...)
	}
	return string(stream)
}

// filmDVD is a DVD with a trailer and a film, the film's cells stored last first and cut across two
// VOB files, with two audio tracks and a subtitle.
func filmDVD() disctest.DVD {
	return disctest.DVD{
		Titles: []disctest.DVDTitle{
			{Cells: []disctest.DVDCell{{Data: disctest.Sectors("trailer", 2), DurationMs: 30_000}}},
			{
				Cells: []disctest.DVDCell{
					{Data: disctest.Sectors("film-a", 3), DurationMs: 3_000_000},
					{Data: disctest.Sectors("film-b", 2), DurationMs: 2_000_000},
					{Data: disctest.Sectors("film-c", 4), DurationMs: 1_200_000},
				},
				ChapterCells: []int{1, 2, 3}, StoredBackwards: true, VOBBytes: 5 * 2048,
			},
		},
		Audio: []disctest.DVDAudio{
			{Coding: 0, Stream: 0, Language: "en"}, {Coding: 6, Stream: 1, Language: "fr"}, {Coding: 4, Stream: 0},
		},
		Subpictures: []disctest.DVDSubpicture{{Stream: 0, Language: "de"}},
	}
}

// A DVD's main title is its longest, served as its cells in play order wherever on the disc they
// are, with its chapters at its programs' cells and its streams' languages by the identifiers
// FFmpeg's program stream demuxer gives them (D350 §1, §4).
func TestMainTitle_ADVDsLongestTitleIsItsCellsInPlayOrder(t *testing.T) {
	t.Parallel()
	for _, volume := range []disctest.Volume{
		{Files: filmDVD().Files()},
		{Files: filmDVD().Files(), SplitBytes: 3 * 2048, LongAllocations: true, Extended: true},
	} {
		image := volume.Bytes()
		title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image)))
		if err != nil {
			t.Fatal(err)
		}
		want := string(disctest.Sectors("film-a", 3)) + string(disctest.Sectors("film-b", 2)) +
			string(disctest.Sectors("film-c", 4))
		if title.Kind != disc.DVD || title.Number != 2 || served(t, image, title) != want {
			t.Errorf("the main title is %s %d, serving %.40q…; want the DVD's title 2, its cells in play order",
				title.Kind, title.Number, served(t, image, title))
		}
		if title.DurationMs != 6_200_000 || !slices.Equal(title.ChaptersMs, []int64{0, 3_000_000, 5_000_000}) {
			t.Errorf("it plays %d ms with chapters at %v; want 6,200,000 with chapters at 0, 3,000,000 and 5,000,000",
				title.DurationMs, title.ChaptersMs)
		}
		wantTracks := []disc.Track{
			{ID: 0x20, Kind: "subtitle", Language: "de"},
			{ID: 0x80, Kind: "audio", Language: "en"},
			{ID: 0x89, Kind: "audio", Language: "fr"},
			{ID: 0xA0, Kind: "audio"},
		}
		if !slices.Equal(title.Tracks, wantTracks) {
			t.Errorf("its tracks are %+v; want %+v", title.Tracks, wantTracks)
		}
	}
}

// Of a cell block of angles, only the first angle is served; a title whose cells are interleaved,
// whose sectors hold the other angles too, is refused as interleaved (D350 §5).
func TestMainTitle_ADVDsOtherAnglesAreLeftOutAndInterleavedCellsRefused(t *testing.T) {
	t.Parallel()
	angles := disctest.DVD{Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
		{Data: disctest.Sectors("opening", 1), DurationMs: 1000},
		{Data: disctest.Sectors("angle-1", 1), DurationMs: 1000, Flags: 0x50},
		{Data: disctest.Sectors("angle-2", 1), DurationMs: 1000, Flags: 0x90},
		{Data: disctest.Sectors("angle-3", 1), DurationMs: 1000, Flags: 0xD0},
		{Data: disctest.Sectors("closing", 1), DurationMs: 1000},
	}}}}
	image := disctest.Volume{Files: angles.Files()}.Bytes()
	title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	want := string(disctest.Sectors("opening", 1)) + string(disctest.Sectors("angle-1", 1)) +
		string(disctest.Sectors("closing", 1))
	if served(t, image, title) != want || title.DurationMs != 3000 {
		t.Errorf("a title with three angles serves %q for %d ms; want its first angle between the others, 3000 ms",
			served(t, image, title), title.DurationMs)
	}
	angles.Titles[0].Cells[1].Flags |= 0x04
	image = disctest.Volume{Files: angles.Files()}.Bytes()
	if _, err := disc.MainTitle(bytes.NewReader(image), int64(len(image))); !errors.Is(err, disc.ErrInterleaved) {
		t.Errorf("a title with an interleaved cell read as %v; want ErrInterleaved", err)
	}
}

// A DVD title whose cells' clock starts again partway, as a disc of two halves does, is served on
// one clock: from the cell whose clock does not carry on from the one before (it starts before that
// one ended, or more than a second after), every SCR, PTS and DTS is moved to carry on, so FFmpeg
// seeking by time lands where the title plays it; a title on one clock is served as it is (D404).
func TestMainTitle_ADVDsClockStartedAgainIsMovedToCarryOn(t *testing.T) {
	t.Parallel()
	const second = 90_000
	first, carriedOn := disctest.VOBU(25_200, 25_200+600*second), disctest.VOBU(25_200+600*second, 25_200+1200*second)
	again := disctest.VOBU(10_000, 10_000+300*second)
	ahead := disctest.VOBU(25_200+1600*second, 25_200+1700*second)
	dvd := disctest.DVD{Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
		{Data: first, DurationMs: 600_000},
		{Data: carriedOn, DurationMs: 600_000},
		{Data: again, DurationMs: 300_000},
		{Data: ahead, DurationMs: 100_000},
	}}}}
	image := disctest.Volume{Files: dvd.Files()}.Bytes()
	title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	cell := int64(len(first))
	againTicks := int64(25_200+1200*second) - 10_000
	aheadTicks := int64(10_000+300*second) + againTicks - (25_200 + 1600*second)
	want := []disc.ClockShift{{FromByte: 2 * cell, Ticks: againTicks}, {FromByte: 3 * cell, Ticks: aheadTicks}}
	if !slices.Equal(title.ClockShifts, want) {
		t.Errorf("a title whose clock starts again, then jumps ahead, is shifted %+v; want %+v",
			title.ClockShifts, want)
	}
	stream := []byte(served(t, image, title))
	title.Restamp(stream, 0)
	for index, check := range []struct {
		vobu  []byte
		ticks int64
	}{{first, 0}, {carriedOn, 0}, {again, againTicks}, {ahead, aheadTicks}} {
		scr, pts, dts := disctest.VOBUTicks(stream[int64(index)*cell:])
		wantSCR, wantPTS, wantDTS := disctest.VOBUTicks(check.vobu)
		if scr != wantSCR+check.ticks || pts != wantPTS+check.ticks || dts != wantDTS+check.ticks {
			t.Errorf("cell %d is served at SCR %d, PTS %d, DTS %d; want %d, %d, %d", index+1, scr, pts, dts,
				wantSCR+check.ticks, wantPTS+check.ticks, wantDTS+check.ticks)
		}
	}
	part := []byte(served(t, image, title))[3*cell:]
	title.Restamp(part, 3*cell)
	if !bytes.Equal(part, stream[3*cell:]) {
		t.Error("the title's last cell restamped on its own differs from the title restamped whole")
	}
	dvd.Titles[0].Cells = dvd.Titles[0].Cells[:2]
	image = disctest.Volume{Files: dvd.Files()}.Bytes()
	if title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image))); err != nil || title.ClockShifts != nil {
		t.Errorf("a title on one clock is shifted %+v (%v); want none", title.ClockShifts, err)
	}
}

// filmBluRay is a Blu-ray with a menu loop, a trailer, a playlist that plays its clip twice (the
// longest, as an obfuscated disc's decoys are), and the film: two clips, its chapters across them.
func filmBluRay() disctest.BluRay {
	return disctest.BluRay{
		Playlists: []disctest.BluRayPlaylist{
			{Number: 0, Items: []disctest.BluRayItem{{Clip: "00009", OutMs: 60_000}}},
			{Number: 1, Items: []disctest.BluRayItem{
				{Clip: "00001", OutMs: 4_000_000}, {Clip: "00001", OutMs: 4_000_000},
			}},
			{
				Number: 800,
				Items: []disctest.BluRayItem{
					{Clip: "00002", InMs: 1000, OutMs: 3_601_000},
					{Clip: "00003", OutMs: 2_400_000, IsMultiAngle: true},
				},
				Chapters: [][2]int64{{0, 0}, {0, 1_800_000}, {1, 600_000}},
				Sounds: []disctest.BluRayStream{
					{PID: 0x1100, Coding: 0x83, Language: "eng"}, {PID: 0x1101, Coding: 0x81, Language: "jpn"},
				},
				Graphics: []disctest.BluRayStream{{PID: 0x1200, Coding: 0x90, Language: "fra"}},
			},
		},
		Clips: map[string][]byte{
			"00001": disctest.M2TS("decoy", 40), "00002": disctest.M2TS("film-1", 50),
			"00003": disctest.M2TS("film-2", 40), "00009": disctest.M2TS("menu", 40),
		},
	}
}

// A Blu-ray's main title is its longest playlist that plays no clip twice, served as its clips
// whole in play order, with its chapters at its entry marks across them and its streams' languages
// by their PIDs, read through UDF 2.50's metadata partition (D350 §1, §4, §5).
func TestMainTitle_ABluRaysLongestPlaylistIsItsClipsInOrder(t *testing.T) {
	t.Parallel()
	for _, volume := range []disctest.Volume{
		{Files: filmBluRay().Files(), Metadata: true, Extended: true},
		{Files: filmBluRay().Files(), Metadata: true, SplitBytes: 7 * 2048},
	} {
		image := volume.Bytes()
		title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image)))
		if err != nil {
			t.Fatal(err)
		}
		want := string(disctest.M2TS("film-1", 50)) + string(disctest.M2TS("film-2", 40))
		if title.Kind != disc.BluRay || title.Number != 800 || served(t, image, title) != want {
			t.Errorf("the main title is %s %d, serving %.40q…; want the Blu-ray's playlist 800, its clips in order",
				title.Kind, title.Number, served(t, image, title))
		}
		if title.DurationMs != 6_000_000 || !slices.Equal(title.ChaptersMs, []int64{0, 1_800_000, 4_200_000}) {
			t.Errorf("it plays %d ms with chapters at %v; want 6,000,000 with chapters at 0, 1,800,000 and 4,200,000",
				title.DurationMs, title.ChaptersMs)
		}
		wantTracks := []disc.Track{
			{ID: 0x1100, Kind: "audio", Language: "eng"},
			{ID: 0x1101, Kind: "audio", Language: "jpn"},
			{ID: 0x1200, Kind: "subtitle", Language: "fra"},
		}
		if !slices.Equal(title.Tracks, wantTracks) {
			t.Errorf("its tracks are %+v; want %+v", title.Tracks, wantTracks)
		}
	}
}

// A broken title, a DVD's title set without its IFO or a playlist that is no MPLS file, is passed
// over for the others: the film is still found.
func TestMainTitle_ABrokenTitleIsPassedOverForTheOthers(t *testing.T) {
	t.Parallel()
	dvd := slices.DeleteFunc(filmDVD().Files(), func(file disctest.File) bool {
		return file.Path == "VIDEO_TS/VTS_01_0.IFO"
	})
	bluRay := append(filmBluRay().Files(), disctest.File{Path: "BDMV/PLAYLIST/00002.mpls", Data: []byte("junk")})
	for name, example := range map[string]struct {
		image  []byte
		number int
	}{
		"a DVD":     {disctest.Volume{Files: dvd}.Bytes(), 2},
		"a Blu-ray": {disctest.Volume{Files: bluRay, Metadata: true}.Bytes(), 800},
	} {
		title, err := disc.MainTitle(bytes.NewReader(example.image), int64(len(example.image)))
		if err != nil || title.Number != example.number {
			t.Errorf("%s with a broken title read as title %d (%v); want its film, %d", name, title.Number, err,
				example.number)
		}
	}
}

// A title still encrypted, an AACS clip or a CSS-scrambled program stream, is refused as encrypted,
// since the node does not decrypt (D350 §5).
func TestMainTitle_AnEncryptedTitleIsRefused(t *testing.T) {
	t.Parallel()
	bluRay := filmBluRay()
	encrypted := disctest.M2TS("film-1", 50)
	encrypted[0] = 0xC0 // the copy permission indicator of an AACS unit
	bluRay.Clips["00002"] = encrypted
	dvd := filmDVD()
	pack := make([]byte, 2048)
	copy(pack, []byte{0, 0, 1, 0xBA, 0x44, 0, 4, 0, 4, 1, 1, 0x89, 0xC3, 0xF8})
	copy(pack[14:], []byte{0, 0, 1, 0xE0, 0x07, 0xEC, 0x90, 0x80, 0})
	dvd.Titles[1].Cells[0].Data = slices.Concat(pack, disctest.Sectors("film-a", 2))
	for name, image := range map[string][]byte{
		"an AACS Blu-ray": disctest.Volume{Files: bluRay.Files(), Metadata: true}.Bytes(),
		"a CSS DVD":       disctest.Volume{Files: dvd.Files()}.Bytes(),
	} {
		if _, err := disc.MainTitle(bytes.NewReader(image), int64(len(image))); !errors.Is(err, disc.ErrEncrypted) {
			t.Errorf("%s read as %v; want ErrEncrypted", name, err)
		}
	}
}

// A file that is not a video disc's image is ErrNotADisc, and a read the storage fails is a
// *disc.ReadError, not the image's fault.
func TestMainTitle_SaysWhatIsNotADiscAndWhatCouldNotBeRead(t *testing.T) {
	t.Parallel()
	data := disctest.Volume{Files: []disctest.File{{Path: "README.TXT", Data: []byte("data")}}}.Bytes()
	for name, image := range map[string][]byte{
		"a Matroska file": append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 600_000)...),
		"a data disc":     data,
		"nothing":         nil,
	} {
		if _, err := disc.MainTitle(bytes.NewReader(image), int64(len(image))); !errors.Is(err, disc.ErrNotADisc) {
			t.Errorf("%s read as %v; want ErrNotADisc", name, err)
		}
	}
	image := disctest.Volume{Files: filmDVD().Files()}.Bytes()
	_, err := disc.MainTitle(failingReader{image: image, failAt: int64(len(image) - 4*2048)}, int64(len(image)))
	if readErr, isRead := errors.AsType[*disc.ReadError](err); !isRead || !strings.Contains(readErr.Error(), "gone") {
		t.Errorf("an image whose storage failed read as %v; want a *disc.ReadError with the storage's error", err)
	}
}

// failingReader is an image whose reads at or past failAt fail, as a share that went away does.
type failingReader struct {
	image  []byte
	failAt int64
}

func (reader failingReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset+int64(len(buffer)) > reader.failAt {
		return 0, errors.New("the share is gone")
	}
	return copy(buffer, reader.image[offset:]), nil
}

// IsImageName is an .iso, whatever its case.
func TestIsImageName_IsAnISOWhateverItsCase(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"Film (2001).iso": true, "FILM.ISO": true, "film.mkv": false, "iso": false, "film.iso.part": false,
	} {
		if disc.IsImageName(name) != want {
			t.Errorf("IsImageName(%q) = %v; want %v", name, !want, want)
		}
	}
}

// Whatever an image holds, reading it ends with a title or an error, and never panics: every
// structure it walks is bounded and every offset checked (D126).
func FuzzMainTitle(f *testing.F) {
	f.Add(disctest.Volume{Files: filmDVD().Files()}.Bytes())
	f.Add(disctest.Volume{Files: filmBluRay().Files(), Metadata: true}.Bytes())
	f.Fuzz(func(t *testing.T, image []byte) {
		title, err := disc.MainTitle(bytes.NewReader(image), int64(len(image)))
		if err != nil {
			return
		}
		var stream []byte
		for _, extent := range title.Extents {
			if extent.Offset < 0 || extent.Length <= 0 || extent.Offset+extent.Length > int64(len(image)) {
				t.Fatalf("a title of %+v, outside an image of %d bytes", title.Extents, len(image))
			}
			stream = append(stream, image[extent.Offset:extent.Offset+extent.Length]...)
		}
		title.Restamp(stream, 0)
	})
}
