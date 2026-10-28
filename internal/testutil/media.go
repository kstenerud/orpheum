// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// mediaCacheCapBytes is how large the generated-media cache may grow before the media used least
// recently go: far above what one run makes (some 45 MiB), so a run never trims what it uses again.
const mediaCacheCapBytes = 2 << 30

// staleMakingAge is how old a making directory a stopped run left may be before a trim removes it:
// far longer than any run takes to make a file, so a making still in progress is never removed.
const staleMakingAge = 24 * time.Hour

// pipeOutput is the output form of media FFmpeg writes to its stdout, as GenerateImage's are.
const pipeOutput = "pipe:1"

// makingPrefix names the directories media are made in before they are kept.
const makingPrefix = ".making-"

// GenerateMedia writes at path what the pinned FFmpeg makes from arguments: its inputs, its codecs
// and its muxer, as FFmpeg's command line names them, without the output, which is path (its
// extension choosing the muxer where arguments name none). The media are made once and kept in the
// generated-media cache (mediaCache), and path gets a copy, so a test may write to it or change its
// mode and the kept file never changes. It is for tests that need media FFmpeg makes, including tests
// of packages that may not run a process themselves (testing.md §7, tier 2).
func GenerateMedia(tb testing.TB, path string, arguments ...string) {
	tb.Helper()
	if err := pinnedMediaCache(tb).put(tb.Context(), path, arguments); err != nil {
		tb.Fatal(err)
	}
}

// pinnedMediaCache is the generated-media cache of the pinned FFmpeg, in the user's cache directory:
// on Linux $XDG_CACHE_HOME/orpheum/test-media, else ~/.cache/orpheum/test-media, which the dev and
// gate containers keep in a volume of their own.
func pinnedMediaCache(tb testing.TB) mediaCache {
	tb.Helper()
	ffmpeg := filepath.Join(FFmpegDirectory(tb), "ffmpeg")
	userCache, err := os.UserCacheDir()
	if err != nil {
		tb.Fatalf("no cache directory for generated media: %v", err)
	}
	return mediaCache{
		directory: filepath.Join(userCache, "orpheum", "test-media"),
		ffmpeg:    ffmpeg,
		clock:     clock.System{},
		capBytes:  mediaCacheCapBytes,
	}
}

// mediaCache is a directory of media FFmpeg made, each kept under its key (mediaKey): what made it
// and from what, so a later run asking for the same media gets them without running FFmpeg. A kept
// file is read-only and is only ever read: callers get a copy or its bytes. Each use sets its
// modification time, and when a new file makes the directory larger than capBytes, the files used
// least recently go. Its files are the cache's own, outside every media root, and only tests
// reach them: the removes and renames below are why forbidigo's [delete] is waived for each.
type mediaCache struct {
	directory string
	ffmpeg    string
	clock     clock.Clock
	capBytes  int64
}

// put writes at target a copy of the media FFmpeg makes from arguments, with target as its output.
func (cache mediaCache) put(ctx context.Context, target string, arguments []string) (err error) {
	kept, err := cache.open(ctx, filepath.Ext(target), arguments)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, kept.Close()) }()
	copied, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // a test's own path
	if err != nil {
		return fmt.Errorf("copy generated media: %w", err)
	}
	defer func() { err = errors.Join(err, copied.Close()) }()
	if _, err = io.Copy(copied, kept); err != nil {
		return fmt.Errorf("copy generated media to %s: %w", target, err)
	}
	return nil
}

// bytesOf returns what FFmpeg writes to its stdout from arguments, whose output is "pipe:1".
func (cache mediaCache) bytesOf(ctx context.Context, arguments []string) (contents []byte, err error) {
	kept, err := cache.open(ctx, pipeOutput, arguments)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, kept.Close()) }()
	contents, err = io.ReadAll(kept)
	if err != nil {
		return nil, fmt.Errorf("read generated media: %w", err)
	}
	return contents, nil
}

// open returns the kept media FFmpeg makes from arguments with the output form output (an output
// file's extension, or pipeOutput), making and keeping them first when they are not kept. The file is
// opened before its time is set, so a trim by a parallel run cannot take it from under the reader.
func (cache mediaCache) open(ctx context.Context, output string, arguments []string) (*os.File, error) {
	key, err := mediaKey(ctx, cache.ffmpeg, output, arguments)
	if err != nil {
		return nil, err
	}
	kept := filepath.Join(cache.directory, key+keptSuffix(output))
	file, err := os.Open(kept) //nolint:gosec // a file of the test-media cache
	if errors.Is(err, fs.ErrNotExist) {
		if generateErr := cache.generate(ctx, kept, output, arguments); generateErr != nil {
			return nil, generateErr
		}
		file, err = os.Open(kept) //nolint:gosec // a file of the test-media cache
	}
	if err != nil {
		return nil, fmt.Errorf("open generated media: %w", err)
	}
	now := cache.clock.Now()
	// The time records the use for the trim; a cache another user owns cannot be touched, and a file
	// whose time is old is only trimmed sooner.
	_ = os.Chtimes(kept, now, now) //nolint:errcheck // see above
	return file, nil
}

// keptSuffix is the kept file's suffix for an output form: the output file's extension, so a
// listing shows what each file is.
func keptSuffix(output string) string {
	if output == pipeOutput {
		return ".out"
	}
	return output
}

// generate runs FFmpeg into a directory of its own beside the kept files and then renames what it
// made into place, so a parallel run sees the whole file or none: never half of one. Two runs making
// the same file at once each rename a whole one over the other.
func (cache mediaCache) generate(ctx context.Context, kept, output string, arguments []string) error {
	if err := os.MkdirAll(cache.directory, 0o700); err != nil {
		return fmt.Errorf("make the test-media cache: %w", err)
	}
	making, err := os.MkdirTemp(cache.directory, makingPrefix)
	if err != nil {
		return fmt.Errorf("make generated media: %w", err)
	}
	//nolint:errcheck,forbidigo // the cache's own making directory (mediaCache); a leftover is trimmed later
	defer func() { _ = os.RemoveAll(making) }()
	made := filepath.Join(making, "media"+keptSuffix(output))
	argv := append([]string{"-nostdin", "-hide_banner", "-v", "error"}, arguments...)
	if output == pipeOutput {
		err = cache.runToFile(ctx, append(argv, pipeOutput), made)
	} else {
		err = cache.run(ctx, append(argv, "file:"+made), nil)
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(made, 0o400); err != nil {
		return fmt.Errorf("make generated media read-only: %w", err)
	}
	if err = os.Rename(made, kept); err != nil { //nolint:forbidigo // into the cache's own directory (mediaCache)
		return fmt.Errorf("keep generated media: %w", err)
	}
	return cache.trim(kept)
}

// run runs FFmpeg with argv in an empty environment from "/", so nothing but argv decides what it
// makes: the key holds all of it.
func (cache mediaCache) run(ctx context.Context, argv []string, stdout io.Writer) error {
	command := exec.CommandContext(ctx, cache.ffmpeg, argv...) //nolint:gosec // the pinned tool, argv as a slice
	command.Env = []string{}
	command.Dir = "/"
	var stderr bytes.Buffer
	command.Stderr = &stderr
	command.Stdout = stdout
	if err := command.Run(); err != nil {
		return fmt.Errorf("generate media with %v: %w: %s", argv, err, stderr.String())
	}
	return nil
}

// runToFile runs FFmpeg with argv, its stdout written to made.
func (cache mediaCache) runToFile(ctx context.Context, argv []string, made string) (err error) {
	file, err := os.Create(made) //nolint:gosec // in the cache's own directory
	if err != nil {
		return fmt.Errorf("make generated media: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return cache.run(ctx, argv, file)
}

// keptFile is a file of the cache, for the trim.
type keptFile struct {
	path      string
	sizeBytes int64
	used      time.Time
}

// trim removes the files used least recently until the cache is within its cap, never justKept,
// and the making directories a stopped run left.
func (cache mediaCache) trim(justKept string) error {
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		return fmt.Errorf("trim the test-media cache: %w", err)
	}
	now := cache.clock.Now()
	var files []keptFile
	var totalBytes int64
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue // removed by a parallel trim
		}
		path := filepath.Join(cache.directory, entry.Name())
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), makingPrefix) && now.Sub(info.ModTime()) > staleMakingAge {
				//nolint:errcheck,forbidigo // the cache's own leftover (mediaCache); another trim may have removed it
				_ = os.RemoveAll(path)
			}
			continue
		}
		totalBytes += info.Size()
		if path != justKept {
			files = append(files, keptFile{path: path, sizeBytes: info.Size(), used: info.ModTime()})
		}
	}
	slices.SortFunc(files, func(a, b keptFile) int { return a.used.Compare(b.used) })
	for _, file := range files {
		if totalBytes <= cache.capBytes {
			break
		}
		//nolint:forbidigo // the cache's own file (mediaCache)
		if removeErr := os.Remove(file.path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return fmt.Errorf("trim the test-media cache: %w", removeErr)
		}
		totalBytes -= file.sizeBytes
	}
	return nil
}

// mediaKey is the name media are kept under: a hash of the FFmpeg that makes them (ffmpegIdentity),
// the output form, and every argument, an argument naming a file by that file's contents, since its
// path is a new temporary one each run. FFmpeg runs with an empty environment from "/" (run), so
// nothing outside these decides what it makes. An argument that names a file other than as the
// whole argument, such as a filter's option, is keyed by its text alone: such a file needs its
// contents added here before a generator uses it.
func mediaKey(ctx context.Context, ffmpeg, output string, arguments []string) (string, error) {
	identity, err := ffmpegIdentity(ctx, ffmpeg)
	if err != nil {
		return "", err
	}
	parts := []string{"orpheum test media 1", identity, output}
	for _, argument := range arguments {
		contents, isFile, fileErr := fileContentsHash(argument)
		if fileErr != nil {
			return "", fileErr
		}
		if isFile {
			argument = "file contents " + contents
		}
		parts = append(parts, argument)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:32], nil
}

// fileContentsHash hashes the contents of the file argument names, as an absolute path or with
// FFmpeg's file: protocol, and reports whether it names one.
func fileContentsHash(argument string) (string, bool, error) {
	path := strings.TrimPrefix(argument, "file:")
	if !filepath.IsAbs(path) {
		return "", false, nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false, nil //nolint:nilerr // not a file: the argument is keyed by its text
	}
	contents, err := os.ReadFile(path) //nolint:gosec // an input a test named
	if err != nil {
		return "", false, fmt.Errorf("key generated media on %s: %w", path, err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), true, nil
}

// ffmpegVersions are the -version lines of the FFmpegs this process has asked, each by its path, size
// and modification time, so each is asked once and not once for each of the fixture tree's clips.
//
//nolint:gochecknoglobals // a TB carries no values, and the answer must outlive one call
var ffmpegVersions sync.Map

// ffmpegIdentity is what tells one FFmpeg build from another: the first line of its -version, and
// its executable's size and modification time, which a rebuild at the same path and version
// changes. Its path is not part of it, so checkouts at other paths share the cache. Hashing the
// executable would read its 100 MiB in every process.
func ffmpegIdentity(ctx context.Context, ffmpeg string) (string, error) {
	info, err := os.Stat(ffmpeg)
	if err != nil {
		return "", fmt.Errorf("identify FFmpeg: %w", err)
	}
	build := strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10)
	asked := ffmpeg + "\x00" + build
	if known, isKnown := ffmpegVersions.Load(asked); isKnown {
		if version, isString := known.(string); isString {
			return version + "\x00" + build, nil
		}
	}
	command := exec.CommandContext(ctx, ffmpeg, "-version")
	command.Env = []string{}
	command.Dir = "/"
	stdout, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("identify FFmpeg by its -version: %w", err)
	}
	version, _, _ := strings.Cut(string(stdout), "\n")
	ffmpegVersions.Store(asked, version)
	return version + "\x00" + build, nil
}
