// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// FixtureTreeFileCount is how many files BuildFixtureTree writes: the 200-file tree of cut-a's M2
// acceptance.
const FixtureTreeFileCount = 200

// FixtureTree is a generated media root: films and series as people name them, with what sits
// beside them and what should not be there. Each video is a distinct one-second clip from the
// pinned FFmpeg, so no two share a payload identity. The generator is the specification
// (testing.md §7, tier 2); nothing it writes is committed, and every name is synthetic (D187).
type FixtureTree struct {
	Root string
	// Files are every regular file written, relative to Root, as the node reports paths.
	Files []string
	// UnreadablePath is a file left at mode 000: the node reports it as errored.
	UnreadablePath string
}

// titleWords make the synthetic titles.
func titleWords() []string {
	return []string{
		"Harbour", "Lantern", "Quiet", "Orchard", "Copper", "Winter", "Paper", "Signal", "Hollow", "Meridian",
		"Salt", "Violet", "Granite", "Ember", "Tidal", "Northern", "Glass", "Velvet", "Iron", "Amber",
	}
}

// treeBuilder writes a fixture tree's files and collects its videos, which are generated last.
type treeBuilder struct {
	tb     testing.TB
	tree   FixtureTree
	videos []string
}

func (builder *treeBuilder) write(relativePath string, contents []byte) {
	path := filepath.Join(builder.tree.Root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		builder.tb.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		builder.tb.Fatal(err)
	}
	builder.tree.Files = append(builder.tree.Files, relativePath)
}

func (builder *treeBuilder) addVideo(relativePath string) {
	builder.tree.Files = append(builder.tree.Files, relativePath)
	builder.videos = append(builder.videos, relativePath)
}

func (builder *treeBuilder) isFull() bool { return len(builder.tree.Files) >= FixtureTreeFileCount }

// subRipBody is a one-line SubRip file.
func subRipBody() []byte { return []byte("1\n00:00:00,000 --> 00:00:01,000\nHello.\n") }

// BuildFixtureTree writes the tree under a new temporary directory and returns it. Files the
// node should skip or report as errors are written as plain bytes; every video is a real clip.
func BuildFixtureTree(tb testing.TB) FixtureTree {
	tb.Helper()
	builder := &treeBuilder{tb: tb, tree: FixtureTree{Root: tb.TempDir()}}
	builder.writeSeries()
	builder.writeJunk()
	builder.writeFilms()
	builder.tree.Files = builder.tree.Files[:FixtureTreeFileCount]
	generateClips(tb, builder.tree.Root, videosIn(builder.videos, builder.tree.Files))
	unreadable := filepath.Join(builder.tree.Root, filepath.FromSlash(builder.tree.UnreadablePath))
	if err := os.Chmod(unreadable, 0o000); err != nil {
		tb.Fatal(err)
	}
	return builder.tree
}

// writeSeries writes three series of two seasons of six episodes, a special each, subtitles for
// each season's first episode, and one tvshow.nfo.
func (builder *treeBuilder) writeSeries() {
	for index, show := range []string{"Paper Lantern", "The Copper Signal", "Northern Orchard"} {
		base := "Shows/" + show
		if index == 0 {
			builder.write(base+"/tvshow.nfo", []byte("<tvshow><title>Paper Lantern</title><year>2011</year></tvshow>"))
		}
		for season := 1; season <= 2; season++ {
			for episode := 1; episode <= 6; episode++ {
				stem := fmt.Sprintf("%s/Season %02d/%s - S%02dE%02d", base, season, show, season, episode)
				builder.addVideo(stem + ".mkv")
				if episode == 1 {
					builder.write(stem+".en.srt", subRipBody())
				}
			}
		}
		builder.addVideo(fmt.Sprintf("%s/Specials/%s - S00E01.mkv", base, show))
	}
}

// writeJunk writes what should not be in a library, what cannot be read, and names in another
// script and in no encoding at all.
func (builder *treeBuilder) writeJunk() {
	for _, junk := range []struct {
		relativePath string
		contents     string
	}{
		{"Films/notes.txt", "a list of films"},
		{"Films/Harbour Winter (1979)/._Harbour Winter (1979).mkv", "AppleDouble"},
		{"Films/Harbour Winter (1979)/cover.jpg", "not really a JPEG"},
		{"Films/Thumbs.db", "thumbnail cache"},
		{"Films/Harbour Winter (1979)/Harbour Winter (1979)-sample.mkv", "a sample"},
		{"Films/Empty (2001)/Empty (2001).mkv", ""},
		{"Films/Broken (2002)/Broken (2002).mkv", "these bytes are not a Matroska file"},
		{"Films/Downloading (2003).mkv.part", "half a file"},
		{"Shows/Paper Lantern/Season 01/desktop.ini", "[.ShellClassInfo]"},
	} {
		builder.write(junk.relativePath, []byte(junk.contents))
	}
	builder.tree.UnreadablePath = "Films/Locked (2004)/Locked (2004).mkv"
	builder.write(builder.tree.UnreadablePath, []byte("locked"))
	builder.addVideo("Films/Caf\xe9 Nights (1999)/Caf\xe9 Nights (1999).mkv")
	builder.addVideo("Films/Аэлита (1972)/Аэлита (1972).mkv")
}

// writeFilms writes films until the tree holds FixtureTreeFileCount files: every fifth with an NFO, every
// fourth with subtitles, every seventh with a trailer and a featurette, every eleventh in two
// parts.
func (builder *treeBuilder) writeFilms() {
	words := titleWords()
	for index := 0; !builder.isFull(); index++ {
		name := words[index%len(words)] + " " + words[(index*7+3)%len(words)]
		year := 1950 + (index*13)%70
		folder := fmt.Sprintf("Films/%s (%d)", name, year)
		stem := fmt.Sprintf("%s/%s (%d)", folder, name, year)
		if index%11 == 10 {
			builder.addVideo(stem + " - cd1.mkv")
			builder.addVideo(stem + " - cd2.mkv")
		} else {
			builder.addVideo(stem + ".mkv")
		}
		if index%5 == 0 && !builder.isFull() {
			builder.write(stem+".nfo", fmt.Appendf(nil, "<movie><title>%s</title><year>%d</year></movie>", name, year))
		}
		if index%4 == 0 && !builder.isFull() {
			builder.write(stem+".en.srt", subRipBody())
		}
		if index%7 == 0 && !builder.isFull() {
			builder.addVideo(folder + "/Trailers/trailer.mkv")
			builder.addVideo(stem + "-featurette.mkv")
		}
	}
}

// videosIn keeps the videos still in the tree after it was cut to size.
func videosIn(videos, files []string) []string {
	kept := map[string]bool{}
	for _, file := range files {
		kept[file] = true
	}
	var in []string
	for _, video := range videos {
		if kept[video] {
			in = append(in, video)
		}
	}
	return in
}

// generateClips writes each video as a distinct one-second Matroska clip of one colour, made once and
// kept in the generated-media cache (GenerateMedia).
func generateClips(tb testing.TB, root string, videos []string) {
	tb.Helper()
	cache := pinnedMediaCache(tb)
	var group sync.WaitGroup
	limit := make(chan struct{}, 8)
	failures := make(chan error, len(videos))
	for index, relativePath := range videos {
		path := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			tb.Fatal(err)
		}
		group.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			colour := fmt.Sprintf("color=c=0x%06x:s=64x36:r=1:d=1", (index*2654435761)&0xffffff)
			arguments := []string{"-f", "lavfi", "-i", colour, "-c:v", "mpeg4", "-f", "matroska"}
			if err := cache.put(tb.Context(), path, arguments); err != nil {
				failures <- fmt.Errorf("generate %s: %w", relativePath, err)
			}
		})
	}
	group.Wait()
	close(failures)
	for err := range failures {
		tb.Fatal(err)
	}
}
