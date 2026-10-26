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
