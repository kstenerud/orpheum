// SPDX-License-Identifier: AGPL-3.0-or-later

package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeProber records what it was asked to probe and answers with a fixed record, or fails for
// the paths in failing.
type fakeProber struct {
	probed  []string
	failing map[string]error
	onProbe func()
}

func (fake *fakeProber) Probe(_ context.Context, film execx.Film) (nodeproto.Probe, error) {
	absolutePath := film.Path
	fake.probed = append(fake.probed, absolutePath)
	if fake.onProbe != nil {
		fake.onProbe()
	}
	if err := fake.failing[filepath.Base(absolutePath)]; err != nil {
		return nodeproto.Probe{}, err
	}
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}}}, nil
}

// writeTree writes files (path → contents) under a new directory and returns its path.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var options = Options{UID: 1000, GID: 1000}

// everything is a listing of a whole root, which stops at a hundred errors.
var everything = nodeproto.List{Path: ".", ErrorLimit: 100}

// summary is one line per record, "type path detail", for comparing whole reports.
func summary(records []nodeproto.ScanRecord) []string {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		switch record.Type {
		case nodeproto.RecordScanBegin:
			lines = append(lines, "scanBegin")
		case nodeproto.RecordObserved:
			lines = append(lines, "observed "+record.Observed.Path.Text)
		case nodeproto.RecordSkip:
			lines = append(lines, "skip "+record.Skip.Path.Text+" "+string(record.Skip.Rule))
		case nodeproto.RecordError:
			failure := record.Error
			lines = append(lines, "error "+failure.Path.Text+" "+string(failure.Scope)+" "+string(failure.Code))
		case nodeproto.RecordScanComplete:
			lines = append(lines, "scanComplete")
		case nodeproto.RecordMount:
			lines = append(lines, "mount "+record.Mount.Path.Text)
		case nodeproto.RecordSidecar:
			lines = append(lines, "sidecar "+record.Sidecar.Path.Text+" "+string(record.Sidecar.Kind))
		case nodeproto.RecordChunksChecked:
			lines = append(lines, "chunksChecked "+strconv.FormatInt(record.ChunksChecked.FileID, 10))
		case nodeproto.RecordListed:
			line := "listed " + record.Listed.Path.Text
			if record.Listed.IsDirectory {
				line += "/"
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func checkSidecarBodies(t *testing.T, records []nodeproto.ScanRecord) {
	t.Helper()
	for _, record := range records {
		if sidecar := record.Sidecar; sidecar != nil {
			wantBody := sidecar.Kind == nodeproto.SidecarNFO
			if (string(sidecar.NFOBody) == "<movie/>") != wantBody {
				t.Errorf("sidecar %s carries %q; an NFO carries its bytes, a subtitle does not",
					sidecar.Path.Text, sidecar.NFOBody)
			}
			checkSidecarIdentity(t, sidecar)
		}
	}
}

// checkSidecarIdentity fails unless a subtitle carries the payload hash of its bytes, hashed
// whole, and nothing else carries one.
func checkSidecarIdentity(t *testing.T, sidecar *nodeproto.Sidecar) {
	t.Helper()
	if sidecar.Kind != nodeproto.SidecarSubtitle {
		if sidecar.Identity != nil {
			t.Errorf("sidecar %s carries an identity; only a subtitle does", sidecar.Path.Text)
		}
		return
	}
	const contents = "1\n00:00:01,000 --> 00:00:02,000\nHello\n"
	if sidecar.Identity == nil || sidecar.Identity.PayloadHash != nodetest.PayloadHashHex(t, contents) {
		t.Errorf("subtitle %s carries identity %+v, want its bytes' payload hash", sidecar.Path.Text, sidecar.Identity)
	}
}

// listAll lists all of root and returns every record, failing the test if the listing fails.
func listAll(t *testing.T, scanner *Scanner, root *confine.MediaRoot) []nodeproto.ScanRecord {
	t.Helper()
	records, err := scanner.List(t.Context(), root, everything)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// examineAll examines files under root and returns every record, failing the test if it stops.
func examineAll(
	t *testing.T, scanner *Scanner, root *confine.MediaRoot, files ...nodeproto.ExamineFile,
) []nodeproto.ScanRecord {
	t.Helper()
	records, err := scanner.Examine(t.Context(), root, files, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// video, nfo and subtitle name a file to examine, by its path relative to the root.
func video(path string) nodeproto.ExamineFile {
	return nodeproto.ExamineFile{Path: nodeproto.Path{Text: path}, Kind: nodeproto.ExamineVideo}
}

func nfo(path string) nodeproto.ExamineFile {
	return nodeproto.ExamineFile{Path: nodeproto.Path{Text: path}, Kind: nodeproto.ExamineNFO}
}

func TestExamine_RefusesAnNFOPastTheBound(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"Film/film.nfo": strings.Repeat("x", maxNFOBytes+1)})
	media := nodetest.OpenMediaRoot(t, root)
	records := examineAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media,
		nfo("Film/film.nfo"))
	want := []string{"error Film/film.nfo entry ioError"}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("examined %q, want %q", got, want)
	}
	if detail := records[0].Error.PlatformError.Text; !strings.Contains(detail, "larger than the node sends") {
		t.Errorf("the error says %q, want the bound named", detail)
	}
}

func TestList_CollapsesRepeatedErrorsAndSaysWhoCouldNotRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		testutil.Require(t, "unprivileged-user",
			errors.New("the tests run as root, which can read a mode-000 directory"))
	}
	root := writeTree(t, map[string]string{"Open/d.mkv": "d", "Closed/e.mkv": "e"})
	if err := os.Mkdir(filepath.Join(root, "Broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Each link in Broken leads nowhere, so none of them can be described; Closed cannot be listed.
	for _, name := range []string{"a.mkv", "b.mkv", "c.mkv"} {
		if err := os.Symlink(filepath.Join(root, "gone", name), filepath.Join(root, "Broken", name)); err != nil {
			t.Fatal(err)
		}
	}
	closed := filepath.Join(root, "Closed")
	if err := os.Chmod(closed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(closed, 0o700); err != nil { //nolint:gosec // a directory needs x
			t.Error(err)
		}
	})
	media := nodetest.OpenMediaRoot(t, root)
	records := listAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)

	want := []string{
		"scanBegin",
		"listed Broken/",
		"error Broken/a.mkv entry notFound",
		"listed Closed/",
		"error Closed subtree permissionDenied",
		"listed Open/",
		"listed Open/d.mkv",
		"scanComplete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if closed := records[4].Error; closed.PlatformError.ModeText != "d---------" ||
		closed.PlatformError.OwnerUID != os.Getuid() || closed.PlatformError.Text == "" {
		t.Errorf("the closed directory's error = %+v, want its owner and mode", closed)
	}
	if broken := records[2].Error; broken.OccurrenceCount != 3 || broken.PlatformError.Text == "" {
		t.Errorf("the collapsed error = %+v, want a count of 3 with the platform's words", broken)
	}
	if complete := records[len(records)-1].ScanComplete; complete.ErrorCount != 4 || complete.ListedCount != 4 {
		t.Errorf("scanComplete = %+v, want 4 errors and 4 listed: a listing that could not read is not clean",
			complete)
	}
}

func TestList_StopsARootThatKeepsFailing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		testutil.Require(t, "unprivileged-user",
			errors.New("the tests run as root, which can read a mode-000 directory"))
	}
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		files["Dir"+name+"/"+name+".mkv"] = name
	}
	root := writeTree(t, files)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		closed := filepath.Join(root, "Dir"+name)
		if err := os.Chmod(closed, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(closed, 0o700); err != nil { //nolint:gosec // a directory needs x
				t.Error(err)
			}
		})
	}
	media := nodetest.OpenMediaRoot(t, root)
	records, err := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options).List(t.Context(), media,
		nodeproto.List{Path: ".", ErrorLimit: 3})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"scanBegin",
		"listed Dira/",
		"error Dira subtree permissionDenied",
		"listed Dirb/",
		"error Dirb subtree permissionDenied",
		"listed Dirc/",
		"error Dirc subtree permissionDenied",
		"error . root permissionDenied",
		"scanComplete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if complete := records[len(records)-1].ScanComplete; !complete.IsStoppedByErrors || complete.ErrorCount != 3 {
		t.Errorf("scanComplete = %+v, want stopped after 3 errors", complete)
	}
}

func TestList_ACancelledListingNeverSaysComplete(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"a.mkv": "a", "b.mkv": "b", "c.mkv": "c"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	media := nodetest.OpenMediaRoot(t, root)
	records, err := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options).List(ctx, media, everything)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	got := summary(records)
	if slices.Contains(got, "scanComplete") {
		t.Errorf("a cancelled listing said complete: %q; the core would apply removals (D39)", got)
	}
	if last := got[len(got)-1]; last != "error . run cancelled" {
		t.Errorf("the listing ends %q, want a run-scoped cancelled error", last)
	}
}

func TestListAndExamine_ANameThatIsNotUTF8KeepsItsBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "Am\xe9lie.mkv" // Latin-1, as an old Windows copy leaves it
	if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := nodetest.OpenMediaRoot(t, root)
	scanner := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options)
	listed := listAll(t, scanner, media)[1].Listed
	if listed == nil || string(listed.Path.Raw) != name || listed.Path.Text != "Am\ufffdlie.mkv" {
		t.Fatalf("listed = %+v, want the raw bytes beside a readable text form (D49)", listed)
	}
	examined := examineAll(t, scanner, media, nodeproto.ExamineFile{Path: listed.Path, Kind: nodeproto.ExamineVideo})
	if observed := examined[0].Observed; observed == nil || string(observed.Path.Raw) != name {
		t.Errorf("examined %+v, want the file read by its raw bytes and reported by them", examined)
	}
}

func TestExamine_ReportsEachVideosOpenSubtitlesHash(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("orpheum!", 25_000) // 200,000 bytes, past OpenSubtitles' 128 KiB floor
	root := writeTree(t, map[string]string{"Movies/Big.mkv": body, "Movies/Small.mkv": "small"})
	media := nodetest.OpenMediaRoot(t, root)
	hashes := map[string]string{}
	for _, record := range examineAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media,
		video("Movies/Big.mkv"), video("Movies/Small.mkv")) {
		if record.Type == nodeproto.RecordObserved {
			hashes[record.Observed.Path.Text] = record.Observed.OpensubtitlesHash
		}
	}
	want, err := hash.OpenSubtitlesHash(strings.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if hashes["Movies/Big.mkv"] != want || want == "" || hashes["Movies/Small.mkv"] != "" {
		t.Errorf("the reported hashes are %v; want Big.mkv's to be %s, and none for a file too small (D224 §3)",
			hashes, want)
	}
}
