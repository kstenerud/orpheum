// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf16"
)

// UDF's descriptors (ECMA-167 3/7.2.1 and 4/7.2.1), by their tag identifiers.
const (
	tagAnchor          = 2
	tagPartition       = 5
	tagLogicalVolume   = 6
	tagTerminating     = 8
	tagFileSet         = 256
	tagFileIdentifier  = 257
	tagAllocationExt   = 258
	tagFileEntry       = 261
	tagExtendedFileEnt = 266
)

// UDF's file types (ECMA-167 4/14.6.6) the reader tells apart.
const (
	fileTypeDirectory = 4
)

// anchorSector is where a UDF volume's Anchor Volume Descriptor Pointer is (ECMA-167 3/8.4.2.1).
const anchorSector = 256

// metadataPartitionID names UDF 2.50's metadata partition map, which a Blu-ray keeps its file
// entries and directories in (UDF 2.60 2.2.10).
const metadataPartitionID = "*UDF Metadata Partition"

// errNotFound is a path not in the image.
var errNotFound = errors.New("not in the image")

// Extent is a run of bytes of the image: Length bytes from Offset.
type Extent struct {
	Offset int64
	Length int64
}

// longAD is an allocation of a run of logical blocks in a partition (ECMA-167 4/14.14.2).
type longAD struct {
	lengthBytes int64
	block       int64
	partition   int
}

// volume is an image's UDF volume: its physical partition, its metadata partition when it has one,
// and its root directory.
type volume struct {
	image          io.ReaderAt
	sizeBytes      int64
	physicalRef    int
	physicalStart  int64 // its first sector in the image
	physicalLength int64 // in sectors
	metadataRef    int   // -1 for none
	metadata       []Extent
	readBytes      int64 // what reading its structure has read so far
	root           longAD
}

// udfFile is one file's entry: its size, whether it is a directory, and its bytes, as runs of the
// image or held in the entry itself.
type udfFile struct {
	sizeBytes   int64
	isDirectory bool
	extents     []Extent
	inline      []byte
}

// openVolume finds the image's UDF volume, its partitions and its root directory.
func openVolume(image io.ReaderAt, sizeBytes int64) (*volume, error) {
	anchor, err := readSector(image, sizeBytes, anchorSector*sectorBytes, tagAnchor)
	if err != nil {
		lastSector := sizeBytes/sectorBytes - 1
		if anchor, err = readSector(image, sizeBytes, lastSector*sectorBytes, tagAnchor); err != nil {
			return nil, notADisc(err)
		}
	}
	fields := fieldsOf(anchor)
	partition, logical, err := readVolumeDescriptors(image, sizeBytes, fields.le32(20), fields.le32(16))
	if err != nil {
		partition, logical, err = readVolumeDescriptors(image, sizeBytes, fields.le32(28), fields.le32(24))
	}
	if err != nil {
		return nil, notADisc(err)
	}
	disc := &volume{image: image, sizeBytes: sizeBytes, physicalRef: -1, metadataRef: -1}
	if mapErr := disc.mapPartitions(partition, logical); mapErr != nil {
		return nil, mapErr
	}
	fileSet, _, err := disc.readDescriptor(longADAt(fieldsOf(logical), 248), tagFileSet)
	if err != nil {
		return nil, err
	}
	disc.root = longADAt(fieldsOf(fileSet), 400)
	return disc, nil
}

// notADisc is ErrNotADisc for an image with no UDF volume, keeping a read's failure as it is.
func notADisc(err error) error {
	if readErr, isRead := errors.AsType[*ReadError](err); isRead {
		return readErr
	}
	return fmt.Errorf("%w: %w", ErrNotADisc, err)
}

// readSector reads the sector at offset, a descriptor whose tag is wantID and checks out.
func readSector(image io.ReaderAt, sizeBytes, offset int64, wantIDs ...int) ([]byte, error) {
	data, err := readExactly(image, sizeBytes, offset, sectorBytes)
	if err != nil {
		return nil, err
	}
	if _, err := checkTag(data, wantIDs...); err != nil {
		return nil, err
	}
	return data, nil
}

// checkTag is the identifier of the descriptor tag at the start of data (ECMA-167 3/7.2), once its
// checksum is right and it is one of wantIDs.
func checkTag(data []byte, wantIDs ...int) (int, error) {
	if len(data) < 16 {
		return 0, fmt.Errorf("%w: a descriptor cut short", ErrMalformed)
	}
	var sum byte
	for i, value := range data[:16] {
		if i != 4 {
			sum += value
		}
	}
	id := fieldsOf(data).le16(0)
	if sum != data[4] {
		return 0, fmt.Errorf("%w: a descriptor tag's checksum is wrong", ErrMalformed)
	}
	if slices.Contains(wantIDs, id) {
		return id, nil
	}
	return 0, fmt.Errorf("%w: descriptor %d where %v was expected", ErrMalformed, id, wantIDs)
}

// readVolumeDescriptors reads the volume descriptor sequence of lengthBytes at sector, for its
// partition descriptor and its logical volume descriptor.
func readVolumeDescriptors(
	image io.ReaderAt, sizeBytes, sector, lengthBytes int64,
) (partition, logical []byte, err error) {
	for index := range min(lengthBytes/sectorBytes, maxDescriptorSectors) {
		data, err := readExactly(image, sizeBytes, (sector+index)*sectorBytes, sectorBytes)
		if err != nil {
			return nil, nil, err
		}
		id, err := checkTag(data, tagPartition, tagLogicalVolume, tagTerminating, 1, 3, 4, 7)
		if err != nil {
			return nil, nil, err
		}
		switch id {
		case tagPartition:
			partition = data
		case tagLogicalVolume:
			logical = data
		}
		if id == tagTerminating {
			break
		}
	}
	if partition == nil || logical == nil {
		return nil, nil, fmt.Errorf("%w: no partition or logical volume descriptor", ErrMalformed)
	}
	return partition, logical, nil
}

// mapPartitions reads the logical volume's partition maps: its physical partition, and UDF 2.50's
// metadata partition, a file in the physical one that holds the file entries and directories.
func (disc *volume) mapPartitions(partition, logical []byte) error {
	partitionFields, lvd := fieldsOf(partition), fieldsOf(logical)
	disc.physicalStart, disc.physicalLength = partitionFields.le32(188), partitionFields.le32(192)
	if lvd.le32(212) != sectorBytes {
		return fmt.Errorf("%w: logical blocks of %d bytes", ErrMalformed, lvd.le32(212))
	}
	offset, metadataFileBlock := 440, int64(-1)
	for index := range min(lvd.le32(268), 8) {
		mapType, mapLength := lvd.u8(offset), lvd.u8(offset+1)
		if mapLength < 2 {
			return fmt.Errorf("%w: a partition map of %d bytes", ErrMalformed, mapLength)
		}
		identifier := strings.TrimRight(string(lvd.bytes(offset+5, 23)), "\x00")
		switch {
		case mapType == 1:
			disc.physicalRef = int(index)
		case mapType == 2 && identifier == metadataPartitionID:
			disc.metadataRef, metadataFileBlock = int(index), lvd.le32(offset+40)
		}
		offset += mapLength
	}
	err := errors.Join(partitionFields.err("the partition descriptor"), lvd.err("the logical volume descriptor"))
	if err != nil {
		return err
	}
	if disc.physicalRef < 0 {
		return fmt.Errorf("%w: no physical partition", ErrMalformed)
	}
	if disc.metadataRef < 0 {
		return nil
	}
	metadataFile, err := disc.readFileEntry(longAD{block: metadataFileBlock, partition: disc.physicalRef})
	if err != nil {
		return err
	}
	disc.metadata = metadataFile.extents
	return nil
}

// longADAt is the long allocation descriptor at offset.
func longADAt(fields *field, offset int) longAD {
	return longAD{
		lengthBytes: fields.le32(offset) & 0x3FFFFFFF,
		block:       fields.le32(offset + 4),
		partition:   fields.le16(offset + 8),
	}
}

// locate is where lengthBytes from block of partition lie in the image, as runs: one in the
// physical partition, and as many as the metadata partition's file is cut into there.
func (disc *volume) locate(partition int, block, lengthBytes int64) ([]Extent, error) {
	switch partition {
	case disc.physicalRef:
		blocks := (lengthBytes + sectorBytes - 1) / sectorBytes
		if block < 0 || block > disc.physicalLength || blocks > disc.physicalLength-block {
			return nil, fmt.Errorf("%w: blocks %d+%d outside the partition", ErrMalformed, block, blocks)
		}
		return []Extent{{Offset: (disc.physicalStart + block) * sectorBytes, Length: lengthBytes}}, nil
	case disc.metadataRef:
		return slice(disc.metadata, block*sectorBytes, lengthBytes)
	}
	return nil, fmt.Errorf("%w: partition %d is not the volume's", ErrMalformed, partition)
}

// readDescriptor reads the descriptor at ad's first block, one of wantIDs.
func (disc *volume) readDescriptor(ad longAD, wantIDs ...int) ([]byte, int, error) {
	located, err := disc.locate(ad.partition, ad.block, sectorBytes)
	if err != nil || len(located) != 1 {
		return nil, 0, fmt.Errorf("%w: a descriptor that is not one block (%w)", ErrMalformed, err)
	}
	data, err := disc.read(located[0].Offset, sectorBytes)
	if err != nil {
		return nil, 0, err
	}
	id, err := checkTag(data, wantIDs...)
	return data, id, err
}

// lookup is the file at path, its names separated by "/" and matched whatever their case.
func (disc *volume) lookup(path string) (udfFile, error) {
	file, err := disc.readFileEntry(disc.root)
	for name := range strings.SplitSeq(path, "/") {
		if err != nil {
			return udfFile{}, err
		}
		entries, listErr := disc.list(file)
		if listErr != nil {
			return udfFile{}, listErr
		}
		entry, found := entries[strings.ToUpper(name)]
		if !found {
			return udfFile{}, fmt.Errorf("%s: %w", path, errNotFound)
		}
		file, err = disc.readFileEntry(entry)
	}
	return file, err
}

// children is the entries of the directory at path.
func (disc *volume) children(path string) (map[string]longAD, error) {
	directory, err := disc.lookup(path)
	if err != nil {
		return nil, findError(path, err)
	}
	return disc.list(directory)
}

// readEntry reads the file named name among entries whole, as a structure is read.
func (disc *volume) readEntry(entries map[string]longAD, name string) ([]byte, error) {
	icb, found := entries[strings.ToUpper(name)]
	if !found {
		return nil, fmt.Errorf("%w: no %s", ErrMalformed, name)
	}
	file, err := disc.readFileEntry(icb)
	if err != nil {
		return nil, err
	}
	return disc.readWhole(file, maxStructureBytes)
}

// list is a directory's entries, by their names in upper case, each its file entry's place.
func (disc *volume) list(directory udfFile) (map[string]longAD, error) {
	if !directory.isDirectory {
		return nil, fmt.Errorf("%w: a path through a file", errNotFound)
	}
	data, err := disc.readWhole(directory, maxDirectoryBytes)
	if err != nil {
		return nil, err
	}
	entries := map[string]longAD{}
	for offset := 0; offset+38 <= len(data) && len(entries) < maxEntriesPerDir; {
		if _, err := checkTag(data[offset:], tagFileIdentifier); err != nil {
			return nil, err
		}
		fields := fieldsOf(data[offset:])
		characteristics, nameLength, useLength := fields.u8(18), fields.u8(19), fields.le16(36)
		name := decodeName(fields.bytes(38+useLength, nameLength))
		if err := fields.err("a file identifier"); err != nil {
			return nil, err
		}
		if characteristics&0x0C == 0 && name != "" { // neither the parent nor deleted
			entries[strings.ToUpper(name)] = longADAt(fields, 20)
		}
		offset += (38 + useLength + nameLength + 3) &^ 3
	}
	return entries, nil
}

// decodeName is a file identifier's OSTA compressed Unicode: 8 bits a character, or 16 big-endian.
func decodeName(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	switch data[0] {
	case 8:
		runes := make([]rune, 0, len(data)-1)
		for _, value := range data[1:] {
			runes = append(runes, rune(value))
		}
		return string(runes)
	case 16:
		units := make([]uint16, 0, len(data)/2)
		for index := 1; index+1 < len(data); index += 2 {
			units = append(units, uint16(data[index])<<8|uint16(data[index+1]))
		}
		return string(utf16.Decode(units))
	}
	return ""
}

// readWhole reads a file whole, refusing one larger than maxBytes.
func (disc *volume) readWhole(file udfFile, maxBytes int64) ([]byte, error) {
	if file.sizeBytes > maxBytes {
		return nil, fmt.Errorf("%w: a structure of %d bytes, past %d", ErrMalformed, file.sizeBytes, maxBytes)
	}
	if file.inline != nil {
		return file.inline, nil
	}
	data := make([]byte, 0, file.sizeBytes)
	for _, extent := range file.extents {
		piece, err := disc.read(extent.Offset, extent.Length)
		if err != nil {
			return nil, err
		}
		data = append(data, piece...)
	}
	return data, nil
}
