// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"fmt"
)

// The kinds of allocation descriptor a file entry holds (ECMA-167 4/14.6.8).
const (
	allocationShort    = 0
	allocationLong     = 1
	allocationEmbedded = 3
)

// The kinds of extent an allocation descriptor gives (ECMA-167 4/14.14.1.1): only recorded
// extents and a pointer to more descriptors are read.
const (
	extentRecorded     = 0
	extentContinuation = 3
)

// readFileEntry reads the file entry, or extended file entry, at icb: its size, whether it is a
// directory, and where its bytes are.
func (disc *volume) readFileEntry(icb longAD) (udfFile, error) {
	data, id, err := disc.readDescriptor(icb, tagFileEntry, tagExtendedFileEnt)
	if err != nil {
		return udfFile{}, err
	}
	fields := fieldsOf(data)
	fileType, allocation := fields.u8(27), fields.le16(34)&7
	file := udfFile{sizeBytes: fields.le64(56), isDirectory: fileType == fileTypeDirectory}
	attributesAt := 176
	if id == tagExtendedFileEnt {
		attributesAt = 216
	}
	attributesBytes, descriptorsBytes := int(fields.le32(attributesAt-8)), int(fields.le32(attributesAt-4))
	descriptors := fields.bytes(attributesAt+attributesBytes, descriptorsBytes)
	if cutShort := fields.err("a file entry"); cutShort != nil {
		return udfFile{}, cutShort
	}
	if allocation == allocationEmbedded {
		if file.sizeBytes > int64(len(descriptors)) {
			return udfFile{}, fmt.Errorf("%w: an embedded file larger than its entry", ErrMalformed)
		}
		file.inline = descriptors[:file.sizeBytes]
		return file, nil
	}
	// A short descriptor's blocks are in the partition its entry is in (ECMA-167 4/14.14.1), as
	// Linux reads them: a Blu-ray's files, whose entries are in the metadata partition, give their
	// data in the physical one by long descriptors.
	ads, err := disc.allocations(descriptors, allocation, icb.partition)
	if err != nil {
		return udfFile{}, err
	}
	file.extents, err = disc.extentsOf(ads, file.sizeBytes)
	return file, err
}

// allocations decodes a file's allocation descriptors, following each continuation to the next
// allocation extent descriptor, to at most maxExtentsPerFile.
func (disc *volume) allocations(descriptors []byte, allocation, partition int) ([]longAD, error) {
	var ads []longAD
	for range maxExtentsPerFile {
		decoded, next, err := decodeAllocations(descriptors, allocation, partition)
		if err != nil {
			return nil, err
		}
		ads = append(ads, decoded...)
		if next == nil {
			return ads, nil
		}
		if len(ads) > maxExtentsPerFile {
			break
		}
		data, _, err := disc.readDescriptor(*next, tagAllocationExt)
		if err != nil {
			return nil, err
		}
		fields := fieldsOf(data)
		descriptors = fields.bytes(24, int(fields.le32(20)))
		if err := fields.err("an allocation extent descriptor"); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: a file of more than %d extents", ErrMalformed, maxExtentsPerFile)
}

// decodeAllocations decodes one run of short or long descriptors, to the first of zero length, and
// the continuation it ends with, if any.
func decodeAllocations(descriptors []byte, allocation, partition int) ([]longAD, *longAD, error) {
	size := 8
	switch allocation {
	case allocationShort:
	case allocationLong:
		size = 16
	default:
		return nil, nil, fmt.Errorf("%w: allocation descriptors of kind %d", ErrMalformed, allocation)
	}
	fields := fieldsOf(descriptors)
	var ads []longAD
	for offset := 0; offset+size <= len(descriptors); offset += size {
		allocated := longAD{
			lengthBytes: fields.le32(offset) & 0x3FFFFFFF, block: fields.le32(offset + 4), partition: partition,
		}
		if allocation == allocationLong {
			allocated = longADAt(fields, offset)
		}
		switch kind := fields.le32(offset) >> 30; {
		case allocated.lengthBytes == 0:
			return ads, nil, nil
		case kind == extentContinuation:
			return ads, &allocated, nil
		case kind != extentRecorded:
			return nil, nil, fmt.Errorf("%w: a file with unrecorded extents", ErrMalformed)
		}
		ads = append(ads, allocated)
	}
	return ads, nil, nil
}

// extentsOf is where a file's sizeBytes lie in the image, from its allocations.
func (disc *volume) extentsOf(ads []longAD, sizeBytes int64) ([]Extent, error) {
	var extents []Extent
	remaining := sizeBytes
	for _, allocated := range ads {
		if remaining == 0 {
			break
		}
		located, err := disc.locate(allocated.partition, allocated.block, min(allocated.lengthBytes, remaining))
		if err != nil {
			return nil, err
		}
		extents = appendJoined(extents, located...)
		remaining -= min(allocated.lengthBytes, remaining)
	}
	if remaining != 0 {
		return nil, fmt.Errorf("%w: a file of %d bytes allocated %d", ErrMalformed, sizeBytes, sizeBytes-remaining)
	}
	return extents, nil
}

// appendJoined appends more to extents, joining a run that starts where the last ends.
func appendJoined(extents []Extent, more ...Extent) []Extent {
	for _, extent := range more {
		if extent.Length == 0 {
			continue
		}
		if last := len(extents) - 1; last >= 0 && extents[last].Offset+extents[last].Length == extent.Offset {
			extents[last].Length += extent.Length
			continue
		}
		extents = append(extents, extent)
	}
	return extents
}

// slice is the runs of the image that hold lengthBytes from offset of the stream extents make.
func slice(extents []Extent, offset, lengthBytes int64) ([]Extent, error) {
	if offset < 0 || lengthBytes < 0 {
		return nil, fmt.Errorf("%w: a range of %d bytes at %d", ErrMalformed, lengthBytes, offset)
	}
	var pieces []Extent
	for _, extent := range extents {
		if lengthBytes == 0 {
			break
		}
		if offset >= extent.Length {
			offset -= extent.Length
			continue
		}
		take := min(extent.Length-offset, lengthBytes)
		pieces = appendJoined(pieces, Extent{Offset: extent.Offset + offset, Length: take})
		offset, lengthBytes = 0, lengthBytes-take
	}
	if lengthBytes != 0 {
		return nil, fmt.Errorf("%w: a range past the end of what holds it", ErrMalformed)
	}
	return pieces, nil
}
