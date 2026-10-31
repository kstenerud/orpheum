// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// Each file a store holds but its manifest, its init and each chunk, has a record beside it, its size
// and CRC-32C, written as it is put (PutFile), and is checked against it before it is served and when
// a full rescan asks (D392 §4). One that is missing, damaged or without its record is dropped and
// made again when a player next asks for it, as one never made; a store made before its files were
// recorded cannot be checked and is dropped whole at its first use.
//
// CRC-32C, not SHA-256: what it guards against is a disk's or a person's accident, a truncated or
// flipped file, which a 32-bit CRC finds but for one in four billion, and which nothing able to write
// a chunk could not also write the record of; the CPU computes it in hardware at many GB/s, where
// SHA-256 would take milliseconds of every chunk served.

// chunkRecordsVersion is the version of the records a store's files carry, named in its manifest
// (nodeproto.StoreManifest.ChunkRecords).
const chunkRecordsVersion = 1

// recordSuffix ends the name of a file's record: 00012.m4s.sum.
const recordSuffix = ".sum"

// recordName is the name of name's record in its store.
func recordName(name string) string { return name + recordSuffix }

// castagnoli is CRC-32C's table, which crc32 makes once.
func castagnoli() *crc32.Table { return crc32.MakeTable(crc32.Castagnoli) }

// fileRecord is what a store's file was when it was put: its size and its CRC-32C.
type fileRecord struct {
	sizeBytes int64
	checksum  uint32
}

// recordOf is the record of contents.
func recordOf(contents []byte) fileRecord {
	return fileRecord{sizeBytes: int64(len(contents)), checksum: crc32.Checksum(contents, castagnoli())}
}

// encoded is the record as its file holds it: "size=<bytes> crc32c=<8 hex>\n".
func (record fileRecord) encoded() []byte {
	return fmt.Appendf(nil, "size=%d crc32c=%08x\n", record.sizeBytes, record.checksum)
}

// parseRecord is the record encoded holds, and false for anything that is not one, whole.
func parseRecord(encoded []byte) (fileRecord, bool) {
	var record fileRecord
	_, err := fmt.Sscanf(string(encoded), "size=%d crc32c=%x\n", &record.sizeBytes, &record.checksum)
	return record, err == nil && bytes.Equal(record.encoded(), encoded)
}

// storeLockCount is how many locks the stores share, each store hashed to one: a file's put and its
// check hold its store's, so a check never sees a file without the record put with it.
const storeLockCount = 64

// storeLocks are the stores' locks (storeLock).
type storeLocks [storeLockCount]sync.Mutex

// storeLock is the lock of the store at storeDir.
func (shelf *Shelf) storeLock(storeDir string) *sync.Mutex {
	return &shelf.locks[crc32.ChecksumIEEE([]byte(storeDir))%storeLockCount]
}

// isRecordedStore is whether the store's manifest says its files carry records.
func (shelf *Shelf) isRecordedStore(storeDir string) bool {
	manifest, found := shelf.readStoreManifest(storeDir)
	return found && manifest.ChunkRecords == chunkRecordsVersion
}

// fileCheck is what a check of one of a store's files found: whether it is made, and, if it was
// dropped, why; a file made and whole is open, for its caller to serve and close.
type fileCheck struct {
	isMade bool
	// dropped is why it was dropped, empty when it was not.
	dropped string
	file    *os.File
	info    fs.FileInfo
}

// OpenChecked opens a store's file, its init or a chunk, once it is what its record says, its size
// and every byte, and false when it is not made: never made, or dropped now, logged at warn, missing,
// damaged or without its record, or of a store made before its files were recorded (D392 §4). The
// caller serves what it opened and closes it.
func (shelf *Shelf) OpenChecked(ctx context.Context, storeDir, name string) (*os.File, fs.FileInfo, bool) {
	if !shelf.checkStore(ctx, storeDir) {
		return nil, nil, false
	}
	checked := shelf.checkFile(ctx, storeDir, name)
	return checked.file, checked.info, checked.isMade
}

// checkStore is whether a store's files can be checked: a store made before its files were recorded,
// not being written, is dropped whole, logged (D392 §4). A store that is not there, or being opened,
// is not dropped, and false too.
func (shelf *Shelf) checkStore(ctx context.Context, storeDir string) bool {
	lock := shelf.storeLock(storeDir)
	lock.Lock()
	defer lock.Unlock()
	if shelf.isRecordedStore(storeDir) {
		return true
	}
	if shelf.isWriting(storeDir) {
		return false
	}
	_ = shelf.dropUnrecordedStore(ctx, storeDir) //nolint:errcheck // logged; a store left is dropped at its next use
	return false
}

// dropUnrecordedStore drops a store made before its files were recorded, whole, and logs it at warn;
// a store that is not there is left so. Its lock is held by the caller.
func (shelf *Shelf) dropUnrecordedStore(ctx context.Context, storeDir string) error {
	names, err := shelf.storeFileNames(storeDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := shelf.removeRun(storeDir); err != nil {
		shelf.logger.WarnContext(ctx, "could not drop a store of chunks whose chunks are not recorded",
			"storeDir", storeDir, "err", err)
		return err
	}
	fileID, stream := storeNamed(storeDir)
	shelf.logger.WarnContext(ctx, "dropped a store of chunks whose chunks are not recorded, made before they were "+
		"or never finished: it cannot be checked, and is made again when asked for", "fileId", fileID,
		"stream", stream, "storeDir", storeDir, "fileCount", len(names))
	return nil
}

// checkFile checks one of a recorded store's files against its record: whole, it is left open; one
// without its record, or a record without its file, or one whose size or bytes differ from its
// record, is dropped and logged at warn. Neither there is a file not made.
func (shelf *Shelf) checkFile(ctx context.Context, storeDir, name string) fileCheck {
	lock := shelf.storeLock(storeDir)
	lock.Lock()
	encoded, recordErr := shelf.readInStore(storeDir, recordName(name))
	file, openErr := shelf.openInStore(storeDir, name)
	lock.Unlock()
	record, hasRecord := parseRecord(encoded)
	switch {
	case openErr != nil && errors.Is(recordErr, fs.ErrNotExist):
		return fileCheck{}
	case openErr != nil && hasRecord:
		return shelf.drop(ctx, storeDir, name, encoded, nil, "missing: its record says "+
			strconv.FormatInt(record.sizeBytes, 10)+" bytes")
	case openErr != nil:
		return shelf.drop(ctx, storeDir, name, encoded, nil, "missing, and its record unreadable")
	case !hasRecord:
		why := "no record"
		if recordErr == nil {
			why = "its record is unreadable"
		}
		return shelf.drop(ctx, storeDir, name, encoded, file, why)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		readonly.Close(file)
		return fileCheck{}
	}
	if info.Size() != record.sizeBytes {
		return shelf.drop(ctx, storeDir, name, encoded, file, fmt.Sprintf("its size is %d bytes; its record says %d",
			info.Size(), record.sizeBytes))
	}
	checksum, err := checksumOf(file)
	if err != nil {
		return shelf.drop(ctx, storeDir, name, encoded, file, "unreadable: "+err.Error())
	}
	if checksum != record.checksum {
		return shelf.drop(ctx, storeDir, name, encoded, file, fmt.Sprintf("its CRC-32C is %08x; its record says %08x",
			checksum, record.checksum))
	}
	return fileCheck{isMade: true, file: file, info: info}
}

// checksumOf is the CRC-32C of file's bytes, read from its start, which it is left at.
func checksumOf(file *os.File) (uint32, error) {
	hash := crc32.New(castagnoli())
	if _, err := io.Copy(hash, file); err != nil {
		return 0, fmt.Errorf("read %s: %w", file.Name(), err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("read %s: %w", file.Name(), err)
	}
	return hash.Sum32(), nil
}

// drop removes a store's file and its record, found not to agree, logged at warn with why, and closes
// opened, the file checked, if any. One put again since it was checked, its record or its file
// changed, is left: it is the new one, checked when next asked for. Either way it is not made now.
func (shelf *Shelf) drop(
	ctx context.Context, storeDir, name string, checkedRecord []byte, opened *os.File, why string,
) fileCheck {
	if opened != nil {
		defer readonly.Close(opened)
	}
	lock := shelf.storeLock(storeDir)
	lock.Lock()
	defer lock.Unlock()
	if !shelf.isUnchanged(storeDir, name, checkedRecord, opened) {
		return fileCheck{}
	}
	storePath := filepath.Join(shelf.directory, filepath.FromSlash(storeDir))
	defer shelf.sizes.forget(storeDir) // walked again when next found
	for _, each := range []string{recordName(name), name} {
		if err := os.Remove(filepath.Join(storePath, each)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			shelf.logger.WarnContext(ctx, "could not drop a chunk that did not match its record", "storeDir", storeDir,
				"name", each, "err", err)
			return fileCheck{}
		}
	}
	fileID, stream := storeNamed(storeDir)
	shelf.logger.WarnContext(ctx, "dropped a chunk that did not match its record; it is made again when asked for",
		"fileId", fileID, "stream", stream, "chunk", chunkNamed(name), "storeDir", storeDir, "why", why)
	return fileCheck{dropped: why}
}

// isUnchanged is whether a store's file and its record are still those checked: the record's bytes
// the same, and the file the one opened, or still not there when none was. Its lock is held by the
// caller.
func (shelf *Shelf) isUnchanged(storeDir, name string, checkedRecord []byte, opened *os.File) bool {
	encoded, err := shelf.readInStore(storeDir, recordName(name))
	if (err != nil && !errors.Is(err, fs.ErrNotExist)) || !bytes.Equal(encoded, checkedRecord) {
		return false
	}
	now, err := os.Lstat(filepath.Join(shelf.directory, filepath.FromSlash(storeDir), name))
	if opened == nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	openedInfo, openedErr := opened.Stat()
	return err == nil && openedErr == nil && os.SameFile(now, openedInfo)
}

// readInStore is one of a store's files' bytes, read through openInStore.
func (shelf *Shelf) readInStore(storeDir, name string) ([]byte, error) {
	file, err := shelf.openInStore(storeDir, name)
	if err != nil {
		return nil, err
	}
	defer readonly.Close(file)
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s/%s: %w", storeDir, name, err)
	}
	return contents, nil
}

// storeFileNames are the names of the files a store holds that are checked, its init and chunks,
// sorted, each once, whether the file or only its record is there: not its manifest, its runs' raw
// directories or a file being written whole (atomics' temporaries, named with a leading dot).
func (shelf *Shelf) storeFileNames(storeDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(shelf.directory, filepath.FromSlash(storeDir)))
	if err != nil {
		return nil, fmt.Errorf("list the store %s: %w", storeDir, err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == nodeproto.StoreManifestName || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, recordSuffix))
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// CheckStoresOf checks every file of every store of chunks of a film's file, by the core's id of it,
// against its record, dropping as a serve does what is missing, damaged or without its record, and a
// store made before its files were recorded, whole; a store being opened is passed over. It says how
// many files it checked and what it dropped, for a full rescan (D392 §4).
func (shelf *Shelf) CheckStoresOf(ctx context.Context, fileID int64) nodeproto.ChunksChecked {
	checked := nodeproto.ChunksChecked{FileID: fileID}
	stores, err := filepath.Glob(filepath.Join(shelf.directory, strconv.FormatInt(fileID, 10), "chunks-*", "*"))
	if err != nil {
		return checked
	}
	for _, path := range stores {
		relative, err := filepath.Rel(shelf.directory, path)
		if err != nil || ctx.Err() != nil {
			return checked
		}
		shelf.checkStoreFiles(ctx, filepath.ToSlash(relative), &checked)
	}
	return checked
}

// checkStoreFiles checks every file of one store as CheckStoresOf does, adding what it found to checked.
func (shelf *Shelf) checkStoreFiles(ctx context.Context, storeDir string, checked *nodeproto.ChunksChecked) {
	names, err := shelf.storeFileNames(storeDir)
	if err != nil {
		return // not a store's directory, or gone meanwhile
	}
	if !shelf.checkStore(ctx, storeDir) {
		if !shelf.isWriting(storeDir) {
			checked.Dropped = append(checked.Dropped, nodeproto.DroppedChunk{
				StoreDir: storeDir, FileCount: len(names), Why: "made before its chunks were recorded",
			})
		}
		return
	}
	for _, name := range names {
		one := shelf.checkFile(ctx, storeDir, name)
		if one.file != nil {
			readonly.Close(one.file)
		}
		if one.isMade || one.dropped != "" {
			checked.CheckedCount++
		}
		if one.dropped != "" {
			checked.Dropped = append(checked.Dropped, nodeproto.DroppedChunk{
				StoreDir: storeDir, Name: name, FileCount: 1, Why: one.dropped,
			})
		}
	}
}

// storeNamed is the file id and the stream a store's directory, <file_id>/chunks-<stream>/<grid_id>,
// names, as its log lines say them.
func storeNamed(storeDir string) (fileID, stream string) {
	parts := strings.SplitN(storeDir, "/", 3)
	if len(parts) < 2 {
		return storeDir, ""
	}
	return parts[0], strings.TrimPrefix(parts[1], "chunks-")
}

// chunkNamed is a store's file as its log lines say it: "init", or a chunk's index.
func chunkNamed(name string) string {
	number, isChunk := strings.CutSuffix(name, ".m4s")
	if !isChunk {
		return strings.TrimSuffix(name, ".mp4")
	}
	if index, err := strconv.Atoi(number); err == nil {
		return strconv.Itoa(index)
	}
	return name
}
