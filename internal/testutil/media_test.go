// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// cacheOnFake is a generated-media cache in a directory of the test's own, whose FFmpeg is the fake
// answering reply, on a clock the test moves.
func cacheOnFake(t *testing.T, reply FakeReply) (mediaCache, FakeFFmpeg, *clocktest.Fake) {
	t.Helper()
	fake := InstallFakeFFmpeg(t, reply)
	moment := clocktest.NewFake(time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC))
	return mediaCache{
		directory: filepath.Join(t.TempDir(), "test-media"),
		ffmpeg:    filepath.Join(fake.Directory, "ffmpeg"),
		clock:     moment,
		capBytes:  1 << 20,
	}, fake, moment
}

// makingRuns are the fake's runs that made media: every run but the -version that identifies it.
func makingRuns(t *testing.T, fake FakeFFmpeg) []FakeCall {
	t.Helper()
	return slices.DeleteFunc(fake.Calls(t), func(call FakeCall) bool {
		return slices.Equal(call.Arguments, []string{"-version"})
	})
}

// kept lists the files the cache keeps, by name.
func kept(t *testing.T, cache mediaCache) []string {
	t.Helper()
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func putOrFail(t *testing.T, cache mediaCache, target string, arguments ...string) {
	t.Helper()
	if err := cache.put(t.Context(), target, arguments); err != nil {
		t.Fatal(err)
	}
}

// onPinnedFake makes the fake the job's pinned FFmpeg and gives the user a cache directory of the
// test's own, returning the fake and where the generated-media cache keeps its files. Not for a
// parallel test: it sets the environment.
func onPinnedFake(t *testing.T, reply FakeReply) (FakeFFmpeg, string) {
	t.Helper()
	fake := InstallFakeFFmpeg(t, reply)
	t.Setenv(FFmpegDirectoryVariable, fake.Directory)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	userCache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return fake, filepath.Join(userCache, "orpheum", "test-media")
}

func TestGenerateMedia_KeepsThePinnedFFmpegsMediaInTheUsersCacheDirectory(t *testing.T) {
	fake, directory := onPinnedFake(t, FakeReply{FFmpegOutput: "a film"})
	target := filepath.Join(t.TempDir(), "film.mkv")
	GenerateMedia(t, target, "-f", "lavfi", "-i", "testsrc2")
	GenerateMedia(t, target, "-f", "lavfi", "-i", "testsrc2")
	if got := string(readFile(t, target)); got != "a film" || len(makingRuns(t, fake)) != 1 {
		t.Errorf("the film is %q, made %d times; want it made once", got, len(makingRuns(t, fake)))
	}
	if names := kept(t, mediaCache{directory: directory}); len(names) != 1 {
		t.Errorf("%s keeps %q, want the film", directory, names)
	}
}

func TestGenerateImage_IsMadeOnceOnItsStdoutAndKept(t *testing.T) {
	fake, _ := onPinnedFake(t, FakeReply{Stdout: "an image"})
	for range 2 {
		if image := GenerateImage(t, "64x36", "libwebp", "webp"); string(image) != "an image" {
			t.Errorf("the image is %q, want what FFmpeg wrote to its stdout", image)
		}
	}
	runs := makingRuns(t, fake)
	if len(runs) != 1 || !slices.Contains(runs[0].Arguments, "testsrc2=size=64x36:duration=1") ||
		runs[0].Arguments[len(runs[0].Arguments)-1] != pipeOutput {
		arguments := make([][]string, 0, len(runs))
		for _, run := range runs {
			arguments = append(arguments, run.Arguments)
		}
		t.Errorf("FFmpeg ran with %q; want one run of the test pattern onto its stdout", arguments)
	}
}

func TestBuildFixtureTree_ClipsAreKeptAsGeneratedMedia(t *testing.T) {
	fake, _ := onPinnedFake(t, FakeReply{FFmpegOutput: "a clip"})
	videos := []string{"Films/A (2001)/A (2001).mkv", "Films/B (2002)/B (2002).mkv"}
	for range 2 {
		root := t.TempDir()
		generateClips(t, root, videos)
		for _, video := range videos {
			if got := string(readFile(t, filepath.Join(root, filepath.FromSlash(video)))); got != "a clip" {
				t.Errorf("%s holds %q, want the clip", video, got)
			}
		}
	}
	if runs := makingRuns(t, fake); len(runs) != len(videos) {
		t.Errorf("FFmpeg made %d clips for two trees of %d, want each colour once", len(runs), len(videos))
	}
}

func TestGenerateMedia_IsMadeOnceAndEachCallGetsACopy(t *testing.T) {
	t.Parallel()
	cache, fake, _ := cacheOnFake(t, FakeReply{Stdout: "ffmpeg version 8.1\n", FFmpegOutput: "a film"})
	directory := t.TempDir()
	first, second := filepath.Join(directory, "first.mkv"), filepath.Join(directory, "second.mkv")
	putOrFail(t, cache, first, "-f", "lavfi", "-i", "testsrc2")
	putOrFail(t, cache, second, "-f", "lavfi", "-i", "testsrc2")

	runs := makingRuns(t, fake)
	if len(runs) != 1 {
		t.Fatalf("FFmpeg made the media %d times, want once", len(runs))
	}
	wantArguments := []string{"-nostdin", "-hide_banner", "-v", "error", "-f", "lavfi", "-i", "testsrc2"}
	if arguments := runs[0].Arguments; !slices.Equal(arguments[:len(arguments)-1], wantArguments) ||
		!strings.HasPrefix(arguments[len(arguments)-1], "file:") ||
		slices.ContainsFunc(runs[0].Environment, func(variable string) bool {
			return strings.HasPrefix(variable, "HOME=")
		}) {
		t.Errorf("FFmpeg ran with %q in %q; want %q and a file: output, in an empty environment",
			arguments, runs[0].Environment, wantArguments)
	}
	if err := os.WriteFile(first, []byte("changed by a test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := string(readFile(t, second)); got != "a film" {
		t.Errorf("the second copy holds %q, want the film", got)
	}
	names := kept(t, cache)
	if len(names) != 1 || !strings.HasSuffix(names[0], ".mkv") {
		t.Fatalf("the cache keeps %q, want one Matroska file", names)
	}
	keptPath := filepath.Join(cache.directory, names[0])
	info, err := os.Stat(keptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(readFile(t, keptPath)) != "a film" || info.Mode().Perm() != 0o400 {
		t.Errorf("the kept file holds %q at mode %v; want the film, read-only", readFile(t, keptPath), info.Mode())
	}
}

func TestMediaKey_ChangesWithEveryArgumentAndTheOutput(t *testing.T) {
	t.Parallel()
	cache, fake, _ := cacheOnFake(t, FakeReply{Stdout: "an image", FFmpegOutput: "a film"})
	directory := t.TempDir()
	putOrFail(t, cache, filepath.Join(directory, "a.mkv"), "-i", "testsrc2=size=64x36")
	putOrFail(t, cache, filepath.Join(directory, "b.mkv"), "-i", "testsrc2=size=64x38")
	putOrFail(t, cache, filepath.Join(directory, "c.mp4"), "-i", "testsrc2=size=64x36")
	image, err := cache.bytesOf(t.Context(), []string{"-i", "testsrc2=size=64x36"})
	if err != nil {
		t.Fatal(err)
	}
	if string(image) != "an image" {
		t.Errorf("the image is %q, want what FFmpeg wrote to its stdout", image)
	}
	if runs := makingRuns(t, fake); len(runs) != 4 {
		t.Errorf("FFmpeg ran %d times, want once for each argument list and output", len(runs))
	}
	if again, againErr := cache.bytesOf(t.Context(), []string{"-i", "testsrc2=size=64x36"}); againErr != nil ||
		string(again) != "an image" || len(makingRuns(t, fake)) != 4 {
		t.Errorf("the image asked again: %q, %v; want it kept, FFmpeg not run", again, againErr)
	}
}

func TestMediaKey_KeysAFileArgumentByItsContents(t *testing.T) {
	t.Parallel()
	cache, fake, _ := cacheOnFake(t, FakeReply{FFmpegOutput: "a film"})
	directory := t.TempDir()
	sidecar, elsewhere := filepath.Join(directory, "film.en.srt"), filepath.Join(directory, "other.srt")
	writeFile(t, sidecar, []byte("1\nHello.\n"), 0o600)
	writeFile(t, elsewhere, []byte("1\nHello.\n"), 0o600)
	putOrFail(t, cache, filepath.Join(directory, "a.mkv"), "-i", "file:"+sidecar)
	// The same contents at another path, as each run's temporary directory is: kept.
	putOrFail(t, cache, filepath.Join(directory, "b.mkv"), "-i", "file:"+elsewhere)
	if runs := makingRuns(t, fake); len(runs) != 1 {
		t.Fatalf("FFmpeg ran %d times for one input's contents at two paths, want once", len(runs))
	}
	writeFile(t, sidecar, []byte("1\nGoodbye.\n"), 0o600)
	putOrFail(t, cache, filepath.Join(directory, "c.mkv"), "-i", "file:"+sidecar)
	if runs := makingRuns(t, fake); len(runs) != 2 {
		t.Errorf("FFmpeg ran %d times after the input changed, want again", len(runs))
	}
}

// Not parallel: it writes an executable, which a parallel test's fork would inherit open and make
// fail to run ("text file busy").
func TestMediaKey_ChangesWithTheFFmpegBuild(t *testing.T) {
	keyOf := func(ffmpeg string) string {
		t.Helper()
		key, err := mediaKey(t.Context(), ffmpeg, ".mkv", []string{"-i", "testsrc2"})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	eight := InstallFakeFFmpeg(t, FakeReply{Stdout: "ffmpeg version 8.1\n"})
	nine := InstallFakeFFmpeg(t, FakeReply{Stdout: "ffmpeg version 9.0\n"})
	if keyOf(filepath.Join(eight.Directory, "ffmpeg")) == keyOf(filepath.Join(nine.Directory, "ffmpeg")) {
		t.Error("two FFmpeg versions key the same media alike")
	}
	// The same version built again: an executable of its own, beside the fake's replies, and then
	// rebuilt in place, its modification time another.
	linked := keyOf(filepath.Join(eight.Directory, "ffmpeg"))
	rebuilt := filepath.Join(eight.Directory, "ffmpeg-rebuilt")
	writeFile(t, rebuilt, readFile(t, fakeScript(t)), 0o700)
	keys := map[string]bool{linked: true}
	for _, seconds := range []int64{1_800_000_000, 1_800_000_060} {
		if err := os.Chtimes(rebuilt, time.Unix(seconds, 0), time.Unix(seconds, 0)); err != nil {
			t.Fatal(err)
		}
		key := keyOf(rebuilt)
		if keys[key] {
			t.Errorf("a rebuilt FFmpeg of the same version, modified at %d, keys the same media alike", seconds)
		}
		if keyOf(rebuilt) != key {
			t.Error("one FFmpeg keys the same media two ways")
		}
		keys[key] = true
	}
}

func TestGenerateMedia_KeepsNothingOfAFailedRun(t *testing.T) {
	t.Parallel()
	cache, fake, _ := cacheOnFake(t, FakeReply{Stdout: "half an image", FFmpegOutput: "half a film"})
	if _, err := ffmpegIdentity(t.Context(), cache.ffmpeg); err != nil {
		t.Fatal(err)
	}
	// Its -version answered, every run from now on writes half its media and fails.
	writeFile(t, filepath.Join(fake.Directory, "reply.status"), []byte("1"), 0o600)
	if err := cache.put(t.Context(), filepath.Join(t.TempDir(), "film.mkv"), []string{"-i", "x"}); err == nil {
		t.Error("a failed run's media were put")
	}
	if image, err := cache.bytesOf(t.Context(), []string{"-i", "x"}); err == nil {
		t.Errorf("a failed run's image was given: %q", image)
	}
	if names := kept(t, cache); len(names) != 0 {
		t.Errorf("the cache keeps %q after failed runs, want nothing", names)
	}
}

func TestGenerateMedia_ParallelMakingsOfOneFileEachGetItWhole(t *testing.T) {
	t.Parallel()
	cache, _, _ := cacheOnFake(t, FakeReply{FFmpegOutput: "a whole film"})
	directory := t.TempDir()
	var group sync.WaitGroup
	errs := make([]error, 4)
	for index := range errs {
		group.Go(func() {
			errs[index] = cache.put(t.Context(), filepath.Join(directory, strings.Repeat("x", index+1)+".mkv"),
				[]string{"-i", "testsrc2"})
		})
	}
	group.Wait()
	for index, err := range errs {
		got := readFile(t, filepath.Join(directory, strings.Repeat("x", index+1)+".mkv"))
		if err != nil || string(got) != "a whole film" {
			t.Errorf("making %d: %q, %v; want the whole film", index, got, err)
		}
	}
	if names := kept(t, cache); len(names) != 1 {
		t.Errorf("the cache keeps %q, want the one film", names)
	}
}

func TestMediaCache_TrimsTheLeastRecentlyUsedPastItsCap(t *testing.T) {
	t.Parallel()
	cache, fake, moment := cacheOnFake(t, FakeReply{FFmpegOutput: "four"})
	cache.capBytes = 10
	directory := t.TempDir()
	keptAs := map[string]string{}
	use := func(name string) {
		t.Helper()
		moment.Advance(time.Minute)
		putOrFail(t, cache, filepath.Join(directory, name+".mkv"), "-i", name)
		key, err := mediaKey(t.Context(), cache.ffmpeg, ".mkv", []string{"-i", name})
		if err != nil {
			t.Fatal(err)
		}
		keptAs[name] = key + ".mkv"
	}
	use("a")
	use("b")
	use("a")
	for name, age := range map[string]time.Duration{"stale": 25 * time.Hour, "fresh": time.Hour} {
		path := filepath.Join(cache.directory, makingPrefix+name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, moment.Now().Add(-age), moment.Now().Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	use("c") // twelve bytes against a cap of ten: b, used least recently, goes

	want := []string{keptAs["a"], keptAs["c"], makingPrefix + "fresh"}
	slices.Sort(want)
	if names := kept(t, cache); !slices.Equal(names, want) {
		t.Errorf("the cache keeps %q; want a, c and the making still in progress: %q", names, want)
	}
	use("b")
	if runs := makingRuns(t, fake); len(runs) != 4 {
		t.Errorf("FFmpeg ran %d times; want b made again once trimmed, a and c once each", len(runs))
	}
}
