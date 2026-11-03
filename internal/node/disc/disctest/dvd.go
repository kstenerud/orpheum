// SPDX-License-Identifier: AGPL-3.0-or-later

package disctest

import (
	"fmt"
)

// DVDCell is a cell of a DVD title: its bytes, whole sectors of program stream, and how long it
// plays.
type DVDCell struct {
	Data       []byte
	DurationMs int64
	// Flags are the cell's playback flags (C_PBI's first byte): its place in an angle block, and
	// 0x04 for an interleaved cell.
	Flags byte
	// LastVOBU is the sector of Data its last VOBU starts at, as its C_PBI says.
	LastVOBU int
}

// DVDTitle is a title of a DVD, in a title set of its own with one program chain.
type DVDTitle struct {
	Cells []DVDCell
	// ChapterCells are the cells, 1-based, its chapters start at; none is one chapter at cell 1.
	ChapterCells []int
	// StoredBackwards stores its cells in the title VOBs last first, so play order is not the
	// order on the disc.
	StoredBackwards bool
	// VOBBytes, when set, cuts the title VOBs into files of at most this many bytes, as a disc's
	// VTS_nn_1.VOB to VTS_nn_9.VOB are cut at 1 GiB.
	VOBBytes int
}

// DVDAudio is an audio stream of every title set: its coding (0 AC-3, 4 LPCM, 6 DTS), its stream
// number and its language, "" for none.
type DVDAudio struct {
	Coding   byte
	Stream   int
	Language string
}

// DVDSubpicture is a subpicture stream of every title set: its stream number and language.
type DVDSubpicture struct {
	Stream   int
	Language string
}

// DVD is a DVD's VIDEO_TS: its titles and the streams each title set has.
type DVD struct {
	Titles      []DVDTitle
	Audio       []DVDAudio
	Subpictures []DVDSubpicture
}

// Files are the DVD's VIDEO_TS files: VIDEO_TS.IFO, and each title's set's IFO and VOBs.
func (dvd DVD) Files() []File {
	files := []File{{Path: "VIDEO_TS/VIDEO_TS.IFO", Data: dvd.manager()}}
	for index, title := range dvd.Titles {
		set := index + 1
		vobs, sectors := title.vobs()
		ifo := dvd.titleSet(title, sectors)
		files = append(files, File{Path: fmt.Sprintf("VIDEO_TS/VTS_%02d_0.IFO", set), Data: ifo})
		part := title.VOBBytes
		if part == 0 {
			part = len(vobs) + 1
		}
		for number, start := 1, 0; start < len(vobs); number, start = number+1, start+part {
			path := fmt.Sprintf("VIDEO_TS/VTS_%02d_%d.VOB", set, number)
			files = append(files, File{Path: path, Data: vobs[start:min(start+part, len(vobs))]})
		}
	}
	return files
}

// manager is VIDEO_TS.IFO: its title table, each title the first of its own set.
func (dvd DVD) manager() []byte {
	data := make([]byte, 2*sector)
	copy(data, "DVDVIDEO-VMG")
	putBE32(data[0xC4:], 1)
	table := data[sector:]
	putBE16(table, len(dvd.Titles))
	for index, title := range dvd.Titles {
		entry := table[8+12*index:]
		entry[1] = 1
		putBE16(entry[2:], max(len(title.ChapterCells), 1))
		entry[6], entry[7] = octet(index+1), 1
	}
	return data
}

// vobs are a title's VOBs as one stream, and each cell's first sector there.
func (title DVDTitle) vobs() ([]byte, []int) {
	order := make([]int, len(title.Cells))
	for index := range order {
		order[index] = index
		if title.StoredBackwards {
			order[index] = len(order) - 1 - index
		}
	}
	sectors := make([]int, len(title.Cells))
	var vobs []byte
	for _, cell := range order {
		sectors[cell] = len(vobs) / sector
		vobs = append(vobs, title.Cells[cell].Data...)
	}
	return vobs, sectors
}

// titleSet is a title's VTS_nn_0.IFO: its part table, its one program chain and the streams'
// attributes.
func (dvd DVD) titleSet(title DVDTitle, sectors []int) []byte {
	data := make([]byte, 4*sector)
	copy(data, "DVDVIDEO-VTS")
	putBE32(data[0xC8:], 1)
	putBE32(data[0xCC:], 2)
	putBE16(data[0x202:], len(dvd.Audio))
	for index, audio := range dvd.Audio {
		attributes := data[0x204+8*index:]
		attributes[0] = audio.Coding << 5
		if audio.Language != "" {
			attributes[0] |= 1 << 2
			copy(attributes[2:], audio.Language)
		}
	}
	putBE16(data[0x254:], len(dvd.Subpictures))
	for index, subpicture := range dvd.Subpictures {
		attributes := data[0x256+6*index:]
		if subpicture.Language != "" {
			attributes[0] = 1
			copy(attributes[2:], subpicture.Language)
		}
	}
	chapters := title.ChapterCells
	if len(chapters) == 0 {
		chapters = []int{1}
	}
	parts := data[sector:]
	putBE16(parts, 1)
	putBE32(parts[4:], 12+4*len(chapters)-1)
	putBE32(parts[8:], 12)
	for index := range chapters {
		putBE16(parts[12+4*index:], 1)
		putBE16(parts[14+4*index:], index+1)
	}
	chains := data[2*sector:]
	putBE16(chains, 1)
	chains[8] = 0x81
	putBE32(chains[12:], 16)
	dvd.programChain(chains[16:], title, chapters, sectors)
	return data
}

// programChain writes a title's program chain: its time, streams, programs and cells.
func (dvd DVD) programChain(chain []byte, title DVDTitle, chapters, sectors []int) {
	var total int64
	for _, cell := range title.Cells {
		total += cell.DurationMs
	}
	chain[2], chain[3] = octet(len(chapters)), octet(len(title.Cells))
	copy(chain[4:], dvdTime(total))
	for index, audio := range dvd.Audio {
		putBE16(chain[0x0C+2*index:], 0x8000|audio.Stream<<8)
	}
	for index, subpicture := range dvd.Subpictures {
		putBE32(chain[0x1C+4*index:], 0x80000000|subpicture.Stream<<24)
	}
	programMap := 0xEC
	cellTable := programMap + len(chapters) + 1
	putBE16(chain[0xE6:], programMap)
	putBE16(chain[0xE8:], cellTable)
	programs := make([]byte, 0, len(chapters))
	for _, cell := range chapters {
		programs = append(programs, octet(cell))
	}
	copy(chain[programMap:], programs)
	for index, cell := range title.Cells {
		entry := chain[cellTable+24*index:]
		entry[0] = cell.Flags
		copy(entry[4:], dvdTime(cell.DurationMs))
		putBE32(entry[8:], sectors[index])
		putBE32(entry[16:], sectors[index]+cell.LastVOBU)
		putBE32(entry[20:], sectors[index]+len(cell.Data)/sector-1)
	}
}

// dvdTime is a DVD's time of ms, at 25 frames a second, in BCD.
func dvdTime(ms int64) []byte {
	bcd := func(value int64) byte { return octet(int(value/10<<4 | value%10)) }
	seconds := ms / 1000
	return []byte{bcd(seconds / 3600), bcd(seconds / 60 % 60), bcd(seconds % 60), 1<<6 | bcd(ms%1000/40)}
}

// Sectors is n sectors of filler holding text, which a test reads back to see which cells it was
// served; none of it is a program stream pack, so none of it is scrambled.
func Sectors(text string, n int) []byte {
	data := make([]byte, n*sector)
	for index := range n {
		copy(data[index*sector:], fmt.Sprintf("%s:%d|", text, index))
	}
	return data
}

// VOBU is a DVD's VOBU of two packs: its navigation pack, saying it plays from startTicks to
// endTicks (90 kHz), then a video pack whose SCR and DTS are startTicks and PTS startTicks+3003.
func VOBU(startTicks, endTicks int) []byte {
	data := make([]byte, 2*sector)
	nav := data[:sector]
	packHeader(nav, startTicks)
	copy(nav[14:], []byte{0, 0, 1, 0xBB, 0, 18})
	copy(nav[38:], []byte{0, 0, 1, 0xBF, 0x03, 0xD4, 0x00})
	putBE32(nav[0x39:], startTicks)
	putBE32(nav[0x3D:], endTicks)
	copy(nav[1024:], []byte{0, 0, 1, 0xBF, 0x03, 0xFA, 0x01})
	putBE32(nav[0x40F:], 1) // VOBU_EA: its last pack, the video pack, is the sector after
	video := data[sector:]
	packHeader(video, startTicks)
	copy(video[14:], []byte{0, 0, 1, 0xE0})
	putBE16(video[18:], sector-14-6)
	video[20], video[21], video[22] = 0x81, 0xC0, 10
	putTimestamp(video[23:], 3, startTicks+3003)
	putTimestamp(video[28:], 1, startTicks)
	return data
}

// packHeader writes an MPEG-2 pack header whose SCR is ticks, with no stuffing.
func packHeader(pack []byte, ticks int) {
	copy(pack, []byte{0, 0, 1, 0xBA})
	pack[4] = octet(0x44 | ticks>>27&0x38 | ticks>>28&3)
	pack[5] = octet(ticks >> 20 & 0xFF)
	pack[6] = octet(ticks>>15&0x1F<<3 | 0x04 | ticks>>13&3)
	pack[7] = octet(ticks >> 5 & 0xFF)
	pack[8] = octet(ticks&0x1F<<3 | 0x04)
	pack[9] = 0x01
	pack[10], pack[11], pack[12], pack[13] = 0x01, 0x89, 0xC3, 0xF8
}

// putTimestamp writes a PES PTS or DTS of ticks after its four-bit prefix.
func putTimestamp(field []byte, prefix, ticks int) {
	field[0] = octet(prefix<<4 | ticks>>30&7<<1 | 1)
	field[1] = octet(ticks >> 22 & 0xFF)
	field[2] = octet(ticks>>15&0x7F<<1 | 1)
	field[3] = octet(ticks >> 7 & 0xFF)
	field[4] = octet(ticks&0x7F<<1 | 1)
}

// VOBUTicks reads back the SCR, PTS and DTS of a VOBU's video pack, the second sector of vobu.
func VOBUTicks(vobu []byte) (scr, pts, dts int64) {
	video := vobu[sector:]
	scr = int64(video[4]>>3&7)<<30 | int64(video[4]&3)<<28 | int64(video[5])<<20 |
		int64(video[6]>>3)<<15 | int64(video[6]&3)<<13 | int64(video[7])<<5 | int64(video[8]>>3)
	timestamp := func(field []byte) int64 {
		return int64(field[0]>>1&7)<<30 | int64(field[1])<<22 | int64(field[2]>>1)<<15 |
			int64(field[3])<<7 | int64(field[4]>>1)
	}
	return scr, timestamp(video[23:]), timestamp(video[28:])
}

// AC3FrameBytes is the size of AC3Frames' frames, and AC3FrameTicks how long each plays: 192 kb/s
// at 48 kHz, 1536 samples.
const (
	AC3FrameBytes = 768
	AC3FrameTicks = 2880
)

// AC3Frames is n AC-3 frames, each its sync word and a header saying 48 kHz at 192 kb/s, the rest
// zeros.
func AC3Frames(n int) []byte {
	frames := make([]byte, n*AC3FrameBytes)
	for start := 0; start < len(frames); start += AC3FrameBytes {
		frames[start], frames[start+1], frames[start+4] = 0x0B, 0x77, 0x14 // 48 kHz, frmsizecod 20: 192 kb/s
	}
	return frames
}

// SoundVOBU is VOBU(startTicks, endTicks) with sound after its picture: the bytes of AC-3
// substream 0x80 in private stream 1 packs, the first stamped soundTicks for the frame starting at
// firstFrame of sound, or for none when firstFrame is negative.
func SoundVOBU(startTicks, endTicks int, sound []byte, firstFrame, soundTicks int) []byte {
	const room = sector - 14 - 18 // a pack's header, then the packet's header, PTS and AC-3 header
	data := VOBU(startTicks, endTicks)
	for first := true; first || len(sound) > 0; first = false {
		size := min(len(sound), room)
		if gap := room - size; gap > 0 && gap < 6 {
			size = room - 6 // too little room for a padding packet: the rest goes in the next pack
		}
		pack := make([]byte, sector)
		packHeader(pack, startTicks)
		copy(pack[14:], []byte{0, 0, 1, 0xBD})
		putBE16(pack[18:], 12+size)
		pack[20], pack[22] = 0x81, 5
		for at := 23; at < 28; at++ {
			pack[at] = 0xFF // stuffing, where a PTS is not
		}
		pack[28] = 0x80
		if first {
			pack[21] = 0x80
			putTimestamp(pack[23:], 2, soundTicks)
			if firstFrame >= 0 {
				pack[29] = 1
				putBE16(pack[30:], firstFrame+1)
			}
		}
		copy(pack[32:], sound[:size])
		sound = sound[size:]
		if end := 32 + size; end < sector {
			copy(pack[end:], []byte{0, 0, 1, 0xBE})
			putBE16(pack[end+4:], sector-end-6)
		}
		data = append(data, pack...)
	}
	putBE32(data[0x40F:], len(data)/sector-1) // VOBU_EA: its last pack
	return data
}
