// SPDX-License-Identifier: AGPL-3.0-or-later

package disctest

import (
	"slices"
	"strings"
)

const sector = 2048

// Where the writer puts a volume's structures, in sectors of the image.
const (
	descriptorsAt = 32  // the main volume descriptor sequence
	reserveAt     = 48  // its reserve copy
	anchorAt      = 256 // the Anchor Volume Descriptor Pointer
	partitionAt   = 257 // the physical partition's first sector
)

// File is a file in an image: its path, its names separated by "/", and its bytes.
type File struct {
	Path string
	Data []byte
}

// Volume is a UDF volume to write.
type Volume struct {
	Files []File
	// Metadata keeps the file entries and directories in a metadata partition, as a Blu-ray does.
	Metadata bool
	// Extended writes extended file entries (UDF 2.x) rather than file entries.
	Extended bool
	// LongAllocations gives files' data by long allocation descriptors rather than short ones. A
	// volume with a metadata partition always does, its entries being in another partition than
	// its files' data.
	LongAllocations bool
	// SplitBytes, when set, cuts each file's data into runs of at most this many bytes, a free
	// sector between each, so a file is held by several extents.
	SplitBytes int
}

// node is a file or directory being laid out: where its entry and data are.
type node struct {
	name      string
	data      []byte
	children  []*node
	parent    *node
	entry     int // its file entry's block, in the partition entries go in
	dataStart int // a directory's data block, in that partition
	runs      [][2]int
}

func (item *node) isDirectory() bool { return item.children != nil }

// layout is the volume's blocks as they are allocated: those of the metadata partition (or the
// physical one when there is none), and the physical partition's.
type layout struct {
	volume   Volume
	physical [][]byte // blocks of the physical partition
	meta     [][]byte // blocks of the metadata partition; the physical partition's when not Metadata
	root     *node
	fileSet  int
}

// Bytes writes the volume as an image.
func (volume Volume) Bytes() []byte {
	plan := &layout{volume: volume, root: &node{children: []*node{}}}
	for _, file := range volume.Files {
		plan.add(file)
	}
	if volume.Metadata {
		plan.physical = append(plan.physical, nil) // block 0: the metadata file's entry
	}
	plan.fileSet = plan.allocateEntries(1)
	plan.place(plan.root)
	plan.writeEntries(plan.root)
	plan.writeFileSet()
	return plan.image()
}

// add puts file in the tree, making its directories.
func (plan *layout) add(file File) {
	parent := plan.root
	names := strings.Split(file.Path, "/")
	for _, name := range names[:len(names)-1] {
		index := slices.IndexFunc(parent.children, func(child *node) bool { return child.name == name })
		if index < 0 {
			parent.children = append(parent.children, &node{name: name, children: []*node{}, parent: parent})
			index = len(parent.children) - 1
		}
		parent = parent.children[index]
	}
	parent.children = append(parent.children, &node{name: names[len(names)-1], data: file.Data, parent: parent})
}

// allocateEntries allocates count blocks where entries go, returning the first.
func (plan *layout) allocateEntries(count int) int {
	blocks := &plan.physical
	if plan.volume.Metadata {
		blocks = &plan.meta
	}
	first := len(*blocks)
	for range count {
		*blocks = append(*blocks, make([]byte, sector))
	}
	return first
}

// place allocates each node's entry, a directory's data, and a file's data in the physical
// partition, cut into runs when SplitBytes says so.
func (plan *layout) place(item *node) {
	item.entry = plan.allocateEntries(1)
	if item.isDirectory() {
		item.dataStart = plan.allocateEntries((len(plan.directoryData(item)) + sector - 1) / sector)
		for _, child := range item.children {
			plan.place(child)
		}
		return
	}
	run := len(item.data)
	if plan.volume.SplitBytes > 0 {
		run = plan.volume.SplitBytes
	}
	for start := 0; start < len(item.data); start += run {
		piece := item.data[start:min(start+run, len(item.data))]
		block := len(plan.physical)
		for at := 0; at < len(piece); at += sector {
			plan.physical = append(plan.physical, padded(piece[at:min(at+sector, len(piece))]))
		}
		item.runs = append(item.runs, [2]int{block, len(piece)})
		if plan.volume.SplitBytes > 0 {
			plan.physical = append(plan.physical, make([]byte, sector)) // a gap between runs
		}
	}
}

func padded(data []byte) []byte {
	block := make([]byte, sector)
	copy(block, data)
	return block
}

// entriesRef is the partition reference of the partition entries go in.
func (plan *layout) entriesRef() int {
	if plan.volume.Metadata {
		return 1
	}
	return 0
}

// directoryData is a directory's file identifier descriptors: its parent, then each child.
func (plan *layout) directoryData(item *node) []byte {
	parent := item.parent
	if parent == nil {
		parent = item
	}
	data := fileIdentifier(nil, 0x0A, parent.entry, plan.entriesRef())
	for _, child := range item.children {
		characteristics := byte(0)
		if child.isDirectory() {
			characteristics = 0x02
		}
		data = append(data, fileIdentifier([]byte(child.name), characteristics, child.entry, plan.entriesRef())...)
	}
	return data
}

// fileIdentifier is one file identifier descriptor (ECMA-167 4/14.4), its name 8 bits a character.
func fileIdentifier(name []byte, characteristics byte, entry, partition int) []byte {
	encoded := []byte{}
	if len(name) > 0 {
		encoded = append([]byte{8}, name...)
	}
	data := make([]byte, (38+len(encoded)+3)&^3)
	putLE16(data[16:], 1)
	data[18], data[19] = characteristics, octet(len(encoded))
	putLongAD(data[20:], sector, entry, partition)
	copy(data[38:], encoded)
	tag(data, 257, 0)
	return data
}

// writeEntries writes every node's file entry and every directory's data.
func (plan *layout) writeEntries(item *node) {
	blocks := plan.physical
	if plan.volume.Metadata {
		blocks = plan.meta
	}
	if item.isDirectory() {
		data := plan.directoryData(item)
		for index := 0; index*sector < len(data); index++ {
			copy(blocks[item.dataStart+index], data[index*sector:])
		}
		runs := [][2]int{{item.dataStart, len(data)}}
		blocks[item.entry] = plan.fileEntry(item.entry, 4, int64(len(data)), runs, false)
		for _, child := range item.children {
			plan.writeEntries(child)
		}
		return
	}
	long := plan.volume.LongAllocations || plan.volume.Metadata
	blocks[item.entry] = plan.fileEntry(item.entry, 5, int64(len(item.data)), item.runs, long)
}

// fileEntry is a file entry, or an extended one, of fileType and size, its data in runs, at block.
func (plan *layout) fileEntry(block, fileType int, size int64, runs [][2]int, long bool) []byte {
	data := make([]byte, sector)
	putLE16(data[20:], 4) // ICB strategy 4
	putLE16(data[48:], 1) // one link
	data[27] = octet(fileType)
	allocation := 0
	if long {
		allocation = 1
	}
	putLE16(data[34:], allocation)
	putLE64(data[56:], size)
	descriptorsAt, id := 176, 261
	if plan.volume.Extended {
		descriptorsAt, id = 216, 266
		putLE64(data[64:], size)
	}
	offset := descriptorsAt
	for _, run := range runs {
		if long {
			putLongAD(data[offset:], run[1], run[0], 0)
			offset += 16
			continue
		}
		putLE32(data[offset:], run[1])
		putLE32(data[offset+4:], run[0])
		offset += 8
	}
	putLE32(data[descriptorsAt-4:], offset-descriptorsAt)
	tag(data, id, block)
	return data
}

// putLongAD writes a long allocation descriptor.
func putLongAD(data []byte, length, block, partition int) {
	putLE32(data, length)
	putLE32(data[4:], block)
	putLE16(data[8:], partition)
}

// writeFileSet writes the file set descriptor, which names the root directory.
func (plan *layout) writeFileSet() {
	data := make([]byte, sector)
	putLongAD(data[400:], sector, plan.root.entry, plan.entriesRef())
	osta(data[416:])
	tag(data, 256, plan.fileSet)
	if plan.volume.Metadata {
		plan.meta[plan.fileSet] = data
		return
	}
	plan.physical[plan.fileSet] = data
}

// osta writes the domain identifier of OSTA's UDF, revision 2.50 (UDF 2.60 2.1.5.2).
func osta(regid []byte) {
	copy(regid[1:], "*OSTA UDF Compliant")
	putLE16(regid[24:], 0x0250)
}

// tag fills a descriptor's tag (ECMA-167 3/7.2): its identifier, version, CRC over the rest of the
// descriptor, location and checksum.
func tag(data []byte, id, location int) {
	putLE16(data[0:], id)
	putLE16(data[2:], 2)
	body := data[16:]
	putLE16(data[8:], int(crc(body)))
	putLE16(data[10:], len(body))
	putLE32(data[12:], location)
	var sum byte
	for index, value := range data[:16] {
		if index != 4 {
			sum += value
		}
	}
	data[4] = sum
}

// crc is the CRC-ITU-T (polynomial 0x1021, initial 0) ECMA-167 tags carry.
func crc(data []byte) uint16 {
	var value uint16
	for _, octet := range data {
		value ^= uint16(octet) << 8
		for range 8 {
			if value&0x8000 != 0 {
				value = value<<1 ^ 0x1021
			} else {
				value <<= 1
			}
		}
	}
	return value
}

// image lays the volume out: its recognition sequence, its volume descriptors and their reserve,
// its anchor, then its physical partition, with the metadata partition's blocks at its end.
func (plan *layout) image() []byte {
	physical := plan.physical
	if plan.volume.Metadata {
		start := len(physical)
		physical = append(physical, plan.meta...)
		size := len(plan.meta) * sector
		physical[0] = plan.fileEntry(0, 250, int64(size), [][2]int{{start, size}}, false)
	}
	image := make([]byte, (partitionAt+len(physical))*sector)
	for index, identifier := range []string{"BEA01", "NSR03", "TEA01"} {
		copy(image[(16+index)*sector+1:], identifier)
		image[(16+index)*sector+6] = 1
	}
	for _, at := range []int{descriptorsAt, reserveAt} {
		copy(image[at*sector:], primaryVolumeDescriptor(at))
		copy(image[(at+1)*sector:], plan.partitionDescriptor(at+1, len(physical)))
		copy(image[(at+2)*sector:], plan.logicalVolumeDescriptor(at+2))
		terminating := make([]byte, sector)
		tag(terminating, 8, at+3)
		copy(image[(at+3)*sector:], terminating)
	}
	anchor := make([]byte, sector)
	putLE32(anchor[16:], 16*sector)
	putLE32(anchor[20:], descriptorsAt)
	putLE32(anchor[24:], 16*sector)
	putLE32(anchor[28:], reserveAt)
	tag(anchor, 2, anchorAt)
	copy(image[anchorAt*sector:], anchor)
	for index, block := range physical {
		copy(image[(partitionAt+index)*sector:], block)
	}
	return image
}

// primaryVolumeDescriptor is the volume's primary descriptor (ECMA-167 3/10.1), at sector: its
// identifier, and the one volume of a set of one.
func primaryVolumeDescriptor(location int) []byte {
	data := make([]byte, sector)
	data[24], data[55] = 8, 5
	copy(data[25:], "DISC")
	putLE16(data[88:], 1) // volume sequence number
	putLE16(data[90:], 1) // maximum volume sequence number
	putLE16(data[92:], 2) // interchange level
	putLE16(data[94:], 3) // maximum interchange level
	putLE32(data[96:], 1) // character set list
	putLE32(data[100:], 1)
	tag(data, 1, location)
	return data
}

// partitionDescriptor is the physical partition's descriptor (ECMA-167 3/10.5), at sector.
func (plan *layout) partitionDescriptor(location, blocks int) []byte {
	data := make([]byte, sector)
	copy(data[25:], "+NSR03")
	putLE32(data[188:], partitionAt)
	putLE32(data[192:], blocks)
	tag(data, 5, location)
	return data
}

// logicalVolumeDescriptor is the logical volume's descriptor (ECMA-167 3/10.6), at sector: its
// block size, its file set, and its partition maps, a type 1 map and, for a Blu-ray's layout, UDF
// 2.50's metadata partition map.
func (plan *layout) logicalVolumeDescriptor(location int) []byte {
	data := make([]byte, sector)
	putLE32(data[212:], sector)
	osta(data[216:])
	putLongAD(data[248:], sector, plan.fileSet, plan.entriesRef())
	maps := []byte{1, 6, 1, 0, 0, 0}
	if plan.volume.Metadata {
		metadataMap := make([]byte, 64)
		metadataMap[0], metadataMap[1] = 2, 64
		copy(metadataMap[5:], "*UDF Metadata Partition")
		putLE16(metadataMap[36:], 1)
		putLE32(metadataMap[40:], 0) // the metadata file's entry
		putLE32(metadataMap[44:], 0xFFFFFFFF)
		maps = append(maps, metadataMap...)
	}
	putLE32(data[264:], len(maps))
	putLE32(data[268:], 1+plan.entriesRef())
	copy(data[440:], maps)
	tag(data, 6, location)
	return data
}
