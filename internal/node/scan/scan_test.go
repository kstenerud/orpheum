// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
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

func (fake *fakeProber) Probe(_ context.Context, absolutePath string) (nodeproto.Probe, error) {
	fake.probed = append(fake.probed, absolutePath)
	if fake.onProbe != nil {
		fake.onProbe()
	}
	if err := fake.failing[filepath.Base(absolutePath)]; err != nil {
		return nodeproto.Probe{}, err
	}
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{Names: []string{"matroska"}}}, nil
}

// tree writes files (path → contents) under a new directory and returns its path.
func tree(t *testing.T, files map[string]string) string {
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

func openRoot(t *testing.T, path string) *confine.MediaRoot {
	t.Helper()
	media, err := confine.OpenMediaRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := media.Close(); err != nil {
			t.Error(err)
		}
	})
	return media
}

var options = Options{
	VideoExtensions: []string{"mkv", "mp4"}, SubtitleExtensions: []string{"srt"}, MaxErrors: 100, UID: 1000, GID: 1000,
}

// scanAll scans root and returns every record, failing the test if the scan fails.
func scanAll(t *testing.T, scanner *Scanner, root *confine.MediaRoot) []nodeproto.Record {
	t.Helper()
	records, err := scanRecords(t.Context(), scanner, root)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func scanRecords(ctx context.Context, scanner *Scanner, root *confine.MediaRoot) ([]nodeproto.Record, error) {
	var records []nodeproto.Record
	err := scanner.Scan(ctx, root, func(record nodeproto.Record) error {
		records = append(records, record)
		return nil
	})
	return records, err
}

// summary is one line per record, "type path detail", for comparing whole reports.
func summary(records []nodeproto.Record) []string {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		switch record.Type {
		case nodeproto.RecordScanBegin:
			lines = append(lines, "scan_begin")
		case nodeproto.RecordObserved:
			lines = append(lines, "observed "+record.Observed.Path.Text)
		case nodeproto.RecordSkip:
			lines = append(lines, "skip "+record.Skip.Path.Text+" "+string(record.Skip.Rule))
		case nodeproto.RecordError:
			failure := record.Error
			lines = append(lines, "error "+failure.Path.Text+" "+string(failure.Scope)+" "+string(failure.Code))
		case nodeproto.RecordScanComplete:
			lines = append(lines, "scan_complete")
		case nodeproto.RecordStatus:
			lines = append(lines, "status")
		case nodeproto.RecordMount:
			lines = append(lines, "mount "+record.Mount.Path.Text)
		case nodeproto.RecordSidecar:
			lines = append(lines, "sidecar "+record.Sidecar.Path.Text+" "+string(record.Sidecar.Kind))
		}
	}
	return lines
}

func TestScan_ReportsEveryEntryWithAFilingInByteOrder(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{
		"Movies/Wing (1979)/Wing (1979).mkv":    "alien",
		"Movies/Wing (1979)/Wing (1979).nfo":    "<movie/>",
		"Movies/Wing (1979)/Wing (1979).en.srt": "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
		"Movies/Wing (1979)/sample.mkv":          "debris",
		"Movies/Metropolis (1995)/metropolis-SAMPLE.mp4":      "debris",
		"Movies/Metropolis (1995)/Metropolis.mkv":             "heat",
		"Movies/Metropolis (1995)/Sample/clip.mkv":      "debris",
		"Movies/Metropolis (1995)/x.sample.mkv":         "debris",
		"Movies/Metropolis (1995)/._Metropolis.mkv":           "\x00\x05\x16\x07",
		".trash/2026-09-01/old.mkv":               "old",
		"Shows/.trash/kept.mkv":                   "only the root's .trash is ours",
	})
	if err := syscall.Mkfifo(filepath.Join(root, "Movies", "pipe.mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := openRoot(t, root)
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)
	want := []string{
		"scan_begin",
		"skip .trash trash",
		"sidecar Movies/Wing (1979)/Wing (1979).en.srt subtitle",
		"observed Movies/Wing (1979)/Wing (1979).mkv",
		"sidecar Movies/Wing (1979)/Wing (1979).nfo nfo",
		"skip Movies/Wing (1979)/sample.mkv sample",
		"skip Movies/Metropolis (1995)/._Metropolis.mkv appledouble",
		"observed Movies/Metropolis (1995)/Metropolis.mkv",
		"skip Movies/Metropolis (1995)/Sample sample",
		"skip Movies/Metropolis (1995)/metropolis-SAMPLE.mp4 sample",
		"skip Movies/Metropolis (1995)/x.sample.mkv sample",
		"skip Movies/pipe.mkv not_regular",
		"observed Shows/.trash/kept.mkv",
		"scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("report:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	checkCountsAndFacts(t, records)
	checkSidecarBodies(t, records)
	observed := records[3].Observed
	if observed.Size != int64(len("alien")) || observed.Identity.Scheme == "" || observed.Identity.Hash == "" ||
		len(observed.Probe.Format.Names) == 0 {
		t.Errorf("observed = %+v", observed)
	}
}

func checkCountsAndFacts(t *testing.T, records []nodeproto.Record) {
	t.Helper()
	complete := records[len(records)-1].ScanComplete
	if complete.Observed != 3 || complete.Sidecars != 2 || complete.Skipped != 7 || complete.Errors != 0 ||
		complete.Stopped {
		t.Errorf("scan_complete = %+v", complete)
	}
	begin := records[0].ScanBegin
	if begin.UID != 1000 || begin.GID != 1000 {
		t.Errorf("scan_begin = %+v, want the node's ids", begin)
	}
	// A test's temporary directory is never a mount point, and it is on some device: the facts
	// the core compares to catch an unmounted disk (D208).
	if begin.Device == 0 || begin.MountPoint {
		t.Errorf("scan_begin = %+v, want the root's device and that it is not a mount point", begin)
	}
}

func checkSidecarBodies(t *testing.T, records []nodeproto.Record) {
	t.Helper()
	for _, record := range records {
		if sidecar := record.Sidecar; sidecar != nil {
			wantBody := sidecar.Kind == nodeproto.SidecarNFO
			if (string(sidecar.Body) == "<movie/>") != wantBody {
				t.Errorf("sidecar %s carries %q; an NFO carries its bytes, a subtitle does not",
					sidecar.Path.Text, sidecar.Body)
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
	want, err := hash.Payload(strings.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	if sidecar.Identity == nil || sidecar.Identity.Hash != hex.EncodeToString(want.Hash[:]) {
		t.Errorf("subtitle %s carries identity %+v, want its bytes' payload hash", sidecar.Path.Text, sidecar.Identity)
	}
}

func TestScan_RefusesAnNFOPastTheBound(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"Film/film.nfo": strings.Repeat("x", maxNFOBytes+1)})
	media := openRoot(t, root)
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)
	want := []string{"scan_begin", "error Film/film.nfo entry io_error", "scan_complete"}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("report %q, want %q", got, want)
	}
	if detail := records[1].Error.Raw.Text; !strings.Contains(detail, "larger than the node sends") {
		t.Errorf("the error says %q, want the bound named", detail)
	}
}

func TestScan_FollowsASymlinkOnlyIntoAMediaRoot(t *testing.T) {
	t.Parallel()
	films := tree(t, map[string]string{"Metropolis (1995)/Metropolis.mkv": "heat"})
	shows := tree(t, map[string]string{"Shared/Wing.mkv": "alien"})
	elsewhere := tree(t, map[string]string{"secret.mkv": "not media", "shadow": "root:x:0:0"})
	links := map[string]string{
		"into another root.mkv": filepath.Join(shows, "Shared", "Wing.mkv"),
		"a whole directory":     filepath.Join(shows, "Shared"),
		"out of the roots.mkv":  filepath.Join(elsewhere, "secret.mkv"),
		"etc shadow.mkv":        filepath.Join(elsewhere, "shadow"),
		"loop":                  films,
		"dangling.mkv":          filepath.Join(films, "gone.mkv"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(films, name)); err != nil {
			t.Fatal(err)
		}
	}
	filmsRoot, showsRoot := openRoot(t, films), openRoot(t, shows)
	prober := &fakeProber{}
	records := scanAll(t, NewScanner([]*confine.MediaRoot{filmsRoot, showsRoot}, prober, options), filmsRoot)

	want := []string{
		"scan_begin",
		"observed Metropolis (1995)/Metropolis.mkv",
		"observed a whole directory/Wing.mkv",
		"error dangling.mkv entry not_found",
		"skip etc shadow.mkv symlink_outside_roots",
		"observed into another root.mkv",
		"skip out of the roots.mkv symlink_outside_roots",
		"scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("report:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	for _, probed := range prober.probed {
		if strings.HasPrefix(probed, elsewhere) {
			t.Errorf("probed %s, outside every media root (D206)", probed)
		}
	}
}

func TestScan_CollapsesRepeatedErrorsAndSaysWhoCouldNotRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		testutil.Require(t, "unprivileged-user", errors.New("the tests run as root, which can read a mode-000 file"))
	}
	root := tree(t, map[string]string{
		"Locked/a.mkv": "a", "Locked/b.mkv": "b", "Locked/c.mkv": "c", "Open/d.mkv": "d",
		"Closed/e.mkv": "e",
	})
	for _, name := range []string{"a.mkv", "b.mkv", "c.mkv"} {
		if err := os.Chmod(filepath.Join(root, "Locked", name), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "Closed"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(filepath.Join(root, "Closed"), 0o700); err != nil { //nolint:gosec // a directory needs x
			t.Error(err)
		}
	})
	media := openRoot(t, root)
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)

	want := []string{
		"scan_begin",
		"error Closed subtree permission_denied",
		"error Locked/a.mkv entry permission_denied",
		"observed Open/d.mkv",
		"scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("report:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	locked := records[2].Error
	if locked.RepeatCount != 3 || locked.Raw.Mode != "----------" || locked.Raw.OwnerUID != os.Getuid() ||
		locked.Raw.Text == "" {
		t.Errorf("the collapsed error = %+v, want a count of 3 with owner and mode", locked)
	}
	if complete := records[len(records)-1].ScanComplete; complete.Errors != 4 || complete.Observed != 1 {
		t.Errorf("scan_complete = %+v, want 4 errors and 1 observed: a scan that could not read is not clean", complete)
	}
}

func TestScan_StopsARootThatKeepsFailing(t *testing.T) {
	t.Parallel()
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		files["Dir"+name+"/"+name+".mkv"] = name
	}
	root := tree(t, files)
	failing := map[string]error{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		failing[name+".mkv"] = errors.New("Invalid data found when processing input")
	}
	media := openRoot(t, root)
	limited := options
	limited.MaxErrors = 3
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{failing: failing}, limited), media)

	want := []string{
		"scan_begin",
		"error Dira/a.mkv entry probe_failed",
		"error Dirb/b.mkv entry probe_failed",
		"error Dirc/c.mkv entry probe_failed",
		"error . root probe_failed",
		"scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("report:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if complete := records[len(records)-1].ScanComplete; !complete.Stopped || complete.Errors != 3 {
		t.Errorf("scan_complete = %+v, want stopped after 3 errors", complete)
	}
}

func TestScan_ACancelledScanNeverSaysComplete(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"a.mkv": "a", "b.mkv": "b", "c.mkv": "c"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	media := openRoot(t, root)
	scanner := NewScanner([]*confine.MediaRoot{media}, &fakeProber{onProbe: cancel}, options)
	records, err := scanRecords(ctx, scanner, media)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	got := summary(records)
	if slices.Contains(got, "scan_complete") {
		t.Errorf("a cancelled scan said complete: %q; the core would apply removals (D39)", got)
	}
	if last := got[len(got)-1]; last != "error . run cancelled" {
		t.Errorf("the report ends %q, want a run-scoped cancelled error", last)
	}
}

func TestScan_StopsWhenNobodyIsListening(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"a.mkv": "a", "b.mkv": "b"})
	media := openRoot(t, root)
	gone := errors.New("the core hung up")
	sent := 0
	err := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options).Scan(t.Context(), media,
		func(nodeproto.Record) error {
			sent++
			if sent == 2 {
				return gone
			}
			return nil
		})
	if !errors.Is(err, gone) || sent != 2 {
		t.Errorf("error %v after %d records, want the emit error after 2", err, sent)
	}
}

func TestScan_ANameThatIsNotUTF8KeepsItsBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "Am\xe9lie.mkv" // Latin-1, as an old Windows copy leaves it
	if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := openRoot(t, root)
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)
	observed := records[1].Observed
	if observed == nil || string(observed.Path.Raw) != name || observed.Path.Text != "Am�lie.mkv" {
		t.Errorf("observed = %+v, want the raw bytes beside a readable text form (D49)", observed)
	}
}

func TestScan_ReportsAnImageBesideTheMediaWithoutItsBytes(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{
		"Film (1999)/poster.jpg": "jpeg", "Film (1999)/Film (1999)-fanart.PNG": "png", "Film (1999)/notes.gif": "gif",
	})
	media := openRoot(t, root)
	records := scanAll(t, NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options), media)
	var images []string
	for _, record := range records {
		if sidecar := record.Sidecar; sidecar != nil && sidecar.Kind == nodeproto.SidecarImage {
			images = append(images, sidecar.Path.Text)
			if sidecar.Body != nil || sidecar.Size == 0 {
				t.Errorf("%s: body %q, size %d; an image's bytes never travel in a record (D215)",
					sidecar.Path.Text, sidecar.Body, sidecar.Size)
			}
		}
	}
	slices.Sort(images)
	if want := []string{"Film (1999)/Film (1999)-fanart.PNG", "Film (1999)/poster.jpg"}; !slices.Equal(images, want) {
		t.Errorf("images %v, want %v (a GIF is not artwork anyone saves)", images, want)
	}
}
