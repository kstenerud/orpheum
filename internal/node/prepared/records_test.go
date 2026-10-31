// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// recordedShelf is a shelf over a fresh directory, logging to logs, with testStore opened and its init
// and chunks 0 to 2 put in it.
func recordedShelf(t *testing.T, logs *bytes.Buffer) *Shelf {
	t.Helper()
	shelf := New(t.TempDir(), Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 95}, clocktest.NewFake(now),
		slog.New(slog.NewTextHandler(logs, nil)))
	shelf.diskUsedPercent = func(string) (int, error) { return 50, nil }
	putChunks(t, shelf, testStore)
	return shelf
}

// putChunks opens the store and puts its init and chunks 0 to 2 in it, as a run does.
func putChunks(t *testing.T, shelf *Shelf, storeDir string) {
	t.Helper()
	rawPath, err := shelf.OpenStore(t.Context(), nodeproto.StoreManifest{
		StoreDir: storeDir, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := shelf.CloseRun(rawPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"init.mp4", "00000.m4s", "00001.m4s", "00002.m4s"} {
		if err := shelf.PutFile(storeDir, name, []byte("the bytes of "+name)); err != nil {
			t.Fatal(err)
		}
	}
}

// inStore is a file's path in the store at storeDir on the shelf.
func inStore(shelf *Shelf, storeDir, name string) string {
	return filepath.Join(shelf.directory, filepath.FromSlash(storeDir), name)
}

// openedBytes are what OpenChecked opened of a store's file, and whether it opened it.
func openedBytes(t *testing.T, shelf *Shelf, storeDir, name string) ([]byte, bool) {
	t.Helper()
	file, info, isMade := shelf.OpenChecked(t.Context(), storeDir, name)
	if !isMade {
		return nil, false
	}
	defer readonly.Close(file)
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(contents)) {
		t.Errorf("%s's info says %d bytes, its file %d", name, info.Size(), len(contents))
	}
	return contents, true
}

// Each file a store holds is put with its record, its size and CRC-32C, beside it, and opened whole
// once it is what its record says; one not made is not opened, and nothing is dropped or logged
// (D392 §4).
func TestPutFile_PutsEachFileWithItsRecordAndItIsOpenedOnceItMatches(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	shelf := recordedShelf(t, &logs)
	record, err := os.ReadFile(inStore(shelf, testStore, "00001.m4s.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if want := recordOf([]byte("the bytes of 00001.m4s")).encoded(); !bytes.Equal(record, want) ||
		!strings.HasPrefix(string(record), "size=22 crc32c=") {
		t.Errorf("chunk 1's record is %q, want %q", record, want)
	}
	for _, name := range []string{"init.mp4", "00001.m4s"} {
		if got, isMade := openedBytes(t, shelf, testStore, name); !isMade || string(got) != "the bytes of "+name {
			t.Errorf("%s opened as %q, %v; want its bytes", name, got, isMade)
		}
	}
	if _, isMade := openedBytes(t, shelf, testStore, "00007.m4s"); isMade {
		t.Error("chunk 7, never made, was opened")
	}
	if logs.Len() != 0 {
		t.Errorf("a store whole logged %s", logs.String())
	}
}

// flipByte flips one bit of the sixth byte of the file at path, in place.
func flipByte(path string) (err error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	one := make([]byte, 1)
	if _, err = file.ReadAt(one, 5); err != nil {
		return err
	}
	_, err = file.WriteAt([]byte{one[0] ^ 0x01}, 5)
	return err
}

// A chunk served is checked first, its size from the filesystem and every byte against its record:
// one truncated, one with a byte flipped, one whose file is missing and one without its record are
// each dropped, file and record, logged at warn with its file, stream, chunk and what differed, and is
// as one never made, so it is made again when asked for; the store's other chunks are served (D392
// §4).
func TestOpenChecked_DropsAChunkMissingOrDamagedAgainstItsRecord(t *testing.T) {
	t.Parallel()
	for _, damage := range []struct {
		name, why, file string
		do              func(path string) error
	}{
		{
			"truncated", "its size is 10 bytes; its record says 22", "00001.m4s",
			func(path string) error { return os.Truncate(path, 10) },
		},
		{"a byte flipped", "its CRC-32C is", "00001.m4s", flipByte},
		{"missing", "missing: its record says 22 bytes", "00001.m4s", os.Remove},
		{"without its record", "no record", "00001.m4s.sum", os.Remove},
	} {
		t.Run(damage.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			shelf := recordedShelf(t, &logs)
			if err := damage.do(inStore(shelf, testStore, damage.file)); err != nil {
				t.Fatal(err)
			}
			if got, isMade := openedBytes(t, shelf, testStore, "00001.m4s"); isMade {
				t.Fatalf("a chunk %s was served: %q", damage.name, got)
			}
			assertDropped(t, shelf, logs.String(), damage.why)
		})
	}
}

// assertDropped holds that chunk 1 of testStore was dropped, file and record, and logged at warn with
// why, its store's next chunk served still, and that made again it is served.
func assertDropped(t *testing.T, shelf *Shelf, logged, why string) {
	t.Helper()
	for _, name := range []string{"00001.m4s", "00001.m4s.sum"} {
		if _, err := os.Stat(inStore(shelf, testStore, name)); !os.IsNotExist(err) {
			t.Errorf("the chunk dropped left %s: %v", name, err)
		}
	}
	for _, want := range []string{
		"level=WARN", "dropped a chunk that did not match its record", "fileId=7", "stream=q1200k480p",
		"chunk=1", why,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the chunk dropped logged %q; want %q in it", logged, want)
		}
	}
	if _, isMade := openedBytes(t, shelf, testStore, "00002.m4s"); !isMade {
		t.Error("the chunk dropped took its store's next chunk with it")
	}
	if err := shelf.PutFile(testStore, "00001.m4s", []byte("made again")); err != nil {
		t.Fatal(err)
	}
	if got, isMade := openedBytes(t, shelf, testStore, "00001.m4s"); !isMade || string(got) != "made again" {
		t.Errorf("chunk 1 made again opened as %q, %v", got, isMade)
	}
}

// writeUnrecordedStore writes a store as one made before its files were recorded: its manifest names
// no records, and its init and a chunk carry none.
func writeUnrecordedStore(t *testing.T, shelf *Shelf, storeDir string) {
	t.Helper()
	encoded, err := json.Marshal(nodeproto.StoreManifest{
		StoreDir: storeDir, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{
		nodeproto.StoreManifestName: encoded, "init.mp4": []byte("init"), "00000.m4s": []byte("chunk"),
	} {
		path := inStore(shelf, storeDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A store made before its files were recorded cannot be checked: at its first use, a chunk asked for
// or a run opening it, it is dropped whole, logged at warn, and a run then makes it anew with its
// records; one being written is left (D392 §4).
func TestOpenChecked_DropsAStoreMadeBeforeItsFilesWereRecordedAtItsFirstUse(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	shelf := recordedShelf(t, &logs)
	const old = "8/chunks-picture/fedcba9876543210"
	writeUnrecordedStore(t, shelf, old)
	finishWriting := shelf.MarkWriting(old)
	if _, isMade := openedBytes(t, shelf, old, "00000.m4s"); isMade {
		t.Error("a chunk of a store that cannot be checked was served")
	}
	finishWriting()
	if _, err := os.Stat(inStore(shelf, old, "00000.m4s")); err != nil {
		t.Errorf("a store being written was dropped: %v", err)
	}
	if _, isMade := openedBytes(t, shelf, old, "00000.m4s"); isMade {
		t.Error("a chunk of a store that cannot be checked was served")
	}
	if _, err := os.Stat(inStore(shelf, old, "")); !os.IsNotExist(err) {
		t.Errorf("a store that cannot be checked was left at its first use: %v", err)
	}
	if logged := logs.String(); !strings.Contains(logged, "level=WARN") ||
		!strings.Contains(logged, "dropped a store of chunks whose chunks are not recorded") ||
		!strings.Contains(logged, "fileId=8") || !strings.Contains(logged, "fileCount=2") {
		t.Errorf("the store dropped logged %q", logged)
	}
	// A run opening one drops it first, and makes it anew with its records.
	writeUnrecordedStore(t, shelf, old)
	putChunks(t, shelf, old)
	if kept, found := shelf.readStoreManifest(old); !found || kept.ChunkRecords != chunkRecordsVersion {
		t.Errorf("the store opened again has the manifest %+v, %v; want one naming its records", kept, found)
	}
	if got, isMade := openedBytes(t, shelf, old, "00000.m4s"); !isMade || string(got) != "the bytes of 00000.m4s" {
		t.Errorf("the store made anew serves chunk 0 as %q, %v", got, isMade)
	}
}

// A full rescan's check of a file's stores checks every init and chunk of each against its record,
// drops the missing and damaged and a store made before its files were recorded, and says how many it
// checked and each it dropped; another file's stores are not touched (D392 §4).
func TestCheckStoresOf_ChecksEveryFileOfAFilesStoresAndDropsTheMissingAndDamaged(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	shelf := recordedShelf(t, &logs)
	const sound, unrecorded, other = "7/chunks-sound1/0123456789abcdef", "7/chunks-picture/0123456789abcdef",
		"9/chunks-picture/0123456789abcdef"
	putChunks(t, shelf, sound)
	putChunks(t, shelf, other)
	writeUnrecordedStore(t, shelf, unrecorded)
	if err := os.Truncate(inStore(shelf, testStore, "00002.m4s"), 3); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(inStore(shelf, sound, "init.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(inStore(shelf, other, "00000.m4s"), 3); err != nil {
		t.Fatal(err)
	}
	checked := shelf.CheckStoresOf(t.Context(), 7)
	if checked.FileID != 7 || checked.CheckedCount != 8 {
		t.Errorf("checked %+v; want file 7's two recorded stores' eight files checked", checked)
	}
	var dropped []string
	for _, each := range checked.Dropped {
		dropped = append(dropped, each.StoreDir+"/"+each.Name)
	}
	slices.Sort(dropped)
	want := []string{unrecorded + "/", testStore + "/00002.m4s", sound + "/init.mp4"}
	slices.Sort(want)
	if !slices.Equal(dropped, want) {
		t.Errorf("dropped %v (%+v), want %v", dropped, checked.Dropped, want)
	}
	for _, gone := range []string{inStore(shelf, testStore, "00002.m4s"), inStore(shelf, unrecorded, "")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s was left: %v", gone, err)
		}
	}
	if _, err := os.Stat(inStore(shelf, other, "00000.m4s")); err != nil {
		t.Errorf("another file's damaged chunk was touched: %v", err)
	}
	if again := shelf.CheckStoresOf(t.Context(), 7); len(again.Dropped) != 0 || again.CheckedCount != 6 {
		t.Errorf("a second check found %+v; want the six files left, whole", again)
	}
}

// The cost of checking a chunk before it is served: a 4 MiB chunk opened, its size compared and its
// every byte summed.
func BenchmarkOpenChecked_AFourMiBChunk(b *testing.B) {
	shelf := New(b.TempDir(), Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 95}, clocktest.NewFake(time.Time{}),
		slog.New(slog.DiscardHandler))
	if _, err := shelf.OpenStore(b.Context(), nodeproto.StoreManifest{StoreDir: testStore}); err != nil {
		b.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("orpheum!"), 4<<20/8)
	if err := shelf.PutFile(testStore, "00000.m4s", chunk); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(chunk)))
	for b.Loop() {
		file, _, isMade := shelf.OpenChecked(b.Context(), testStore, "00000.m4s")
		if !isMade {
			b.Fatal("the chunk was not served")
		}
		readonly.Close(file)
	}
}
