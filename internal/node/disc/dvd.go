// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// Where a DVD's IFOs keep what the node reads (DVD-Video's VMGI_MAT and VTSI_MAT, as libdvdread
// names them): the sectors, from the IFO's start, of its tables, and its streams' attributes.
const (
	ifoTitleTable        = 0xC4 // VIDEO_TS.IFO: TT_SRPT
	ifoPartTable         = 0xC8 // VTS_nn_0.IFO: VTS_PTT_SRPT
	ifoProgramChainTable = 0xCC // VTS_nn_0.IFO: VTS_PGCIT
	ifoAudioCount        = 0x202
	ifoAudioAttributes   = 0x204
	ifoSubpictureCount   = 0x254
	ifoSubpictureAttrs   = 0x256
)

// A DVD's bounds: a disc has at most 99 titles, a title set 99 parts of a title, and a program
// chain 255 cells.
const (
	maxDVDTitles = 99
	maxDVDParts  = 999
)

// A cell's playback flags (DVD-Video's C_PBI): its place in an angle block, and whether it is
// interleaved with other cells.
const (
	cellAngleBlock   = 1 // block type
	cellFirstInBlock = 1 // block mode
	cellInterleaved  = 0x04
)

// dvdTitle is a title of VIDEO_TS.IFO's table: its number, its title set, and its number there.
type dvdTitle struct {
	number, titleSet, setTitle int
}

// part is a chapter of a title: the program chain it starts in, and the program there.
type part struct {
	programChain, program int
}

// titleSet is a title set's IFO and the runs of the image its title VOBs make, read as one stream.
type titleSet struct {
	ifo  *field
	vobs []Extent
}

// programChain is a PGC: its playing time, its programs' entry cells, its cells, and which streams
// play in it.
type programChain struct {
	durationMs        int64
	programEntryCells []int
	cells             []dvdCell
	audioControl      []int
	subpictureControl []int64
}

// dvdCell is a cell of a program chain: its flags, playing time and sectors of the title VOBs.
type dvdCell struct {
	flags                   int
	durationMs              int64
	firstSector, lastSector int64
	lastVOBU                int64 // the sector its last VOBU starts at
}

// dvdMainTitle is a DVD's longest title, by the playing time of its program chains.
func dvdMainTitle(disc *volume) (Title, error) {
	entries, err := disc.children("VIDEO_TS")
	if err != nil {
		return Title{}, err
	}
	manager, err := disc.readEntry(entries, "VIDEO_TS.IFO")
	if err != nil {
		return Title{}, err
	}
	titles, err := dvdTitles(fieldsOf(manager))
	if err != nil {
		return Title{}, err
	}
	sets := map[int]*titleSet{}
	var best dvdTitle
	var bestChains []programChain
	var bestParts []part
	var bestMs int64 = -1
	for _, candidate := range titles {
		chains, parts, err := disc.titleChains(entries, sets, candidate)
		if errors.Is(err, ErrMalformed) {
			continue // a broken title is not the film; the others may be
		}
		if err != nil {
			return Title{}, err
		}
		if ms := chainsDurationMs(chains); ms > bestMs {
			best, bestChains, bestParts, bestMs = candidate, chains, parts, ms
		}
	}
	if bestMs < 0 {
		return Title{}, fmt.Errorf("%w: VIDEO_TS.IFO lists no title", ErrNoTitle)
	}
	set := sets[best.titleSet]
	title, cells, assembleErr := set.assemble(best.number, bestChains, bestParts)
	if assembleErr != nil {
		return Title{}, assembleErr
	}
	shifts, shiftsErr := disc.dvdClockShifts(set, cells)
	title.ClockShifts = shifts
	return title, shiftsErr
}

// titleChains is the program chains and parts of a title of the manager's table, from its set.
func (disc *volume) titleChains(
	entries map[string]longAD, sets map[int]*titleSet, candidate dvdTitle,
) ([]programChain, []part, error) {
	set, err := disc.loadTitleSet(entries, sets, candidate.titleSet)
	if err != nil {
		return nil, nil, err
	}
	return set.title(candidate.setTitle)
}

// dvdTitles reads VIDEO_TS.IFO's title table (TT_SRPT).
func dvdTitles(manager *field) ([]dvdTitle, error) {
	if string(manager.bytes(0, 12)) != "DVDVIDEO-VMG" {
		return nil, fmt.Errorf("%w: VIDEO_TS.IFO is not a video manager's", ErrMalformed)
	}
	table := int(manager.be32(ifoTitleTable)) * sectorBytes
	count := min(manager.be16(table), maxDVDTitles)
	titles := make([]dvdTitle, 0, count)
	for index := range count {
		entry := table + 8 + 12*index
		titles = append(titles, dvdTitle{
			number: index + 1, titleSet: manager.u8(entry + 6), setTitle: manager.u8(entry + 7),
		})
	}
	return titles, manager.err("VIDEO_TS.IFO's title table")
}

// loadTitleSet reads title set number's IFO and finds its title VOBs, once.
func (disc *volume) loadTitleSet(entries map[string]longAD, sets map[int]*titleSet, number int) (*titleSet, error) {
	if set, found := sets[number]; found {
		return set, nil
	}
	ifo, err := disc.readEntry(entries, fmt.Sprintf("VTS_%02d_0.IFO", number))
	if err != nil {
		return nil, err
	}
	if string(ifo[:min(12, len(ifo))]) != "DVDVIDEO-VTS" {
		return nil, fmt.Errorf("%w: VTS_%02d_0.IFO is not a title set's", ErrMalformed, number)
	}
	set := &titleSet{ifo: fieldsOf(ifo)}
	for part := 1; part <= 9; part++ {
		icb, found := entries[fmt.Sprintf("VTS_%02d_%d.VOB", number, part)]
		if !found {
			break
		}
		vob, err := disc.readFileEntry(icb)
		if err != nil {
			return nil, err
		}
		set.vobs = appendJoined(set.vobs, vob.extents...)
	}
	sets[number] = set
	return set, nil
}

// title is the program chains a title of the set plays, in order, and its parts (chapters).
func (set *titleSet) title(setTitle int) ([]programChain, []part, error) {
	ifo := set.ifo
	table := int(ifo.be32(ifoPartTable)) * sectorBytes
	count := ifo.be16(table)
	if setTitle < 1 || setTitle > count {
		return nil, nil, fmt.Errorf("%w: title %d of a set of %d", ErrMalformed, setTitle, count)
	}
	start := table + int(ifo.be32(table+4+4*setTitle))
	end := table + int(ifo.be32(table+4)) + 1
	if setTitle < count {
		end = table + int(ifo.be32(table+8+4*setTitle))
	}
	var parts []part
	var chains []programChain
	seen := map[int]int{}
	for offset := start; offset+4 <= end && len(parts) < maxDVDParts; offset += 4 {
		number, program := ifo.be16(offset), ifo.be16(offset+2)
		if _, found := seen[number]; !found {
			chain, err := set.programChain(number)
			if err != nil {
				return nil, nil, err
			}
			seen[number] = len(chains)
			chains = append(chains, chain)
		}
		parts = append(parts, part{programChain: seen[number], program: program})
	}
	return chains, parts, ifo.err("a title set's part table")
}

// programChain reads the set's program chain number (1-based).
func (set *titleSet) programChain(number int) (programChain, error) {
	ifo := set.ifo
	table := int(ifo.be32(ifoProgramChainTable)) * sectorBytes
	if number < 1 || number > ifo.be16(table) {
		return programChain{}, fmt.Errorf("%w: program chain %d of %d", ErrMalformed, number, ifo.be16(table))
	}
	start := table + int(ifo.be32(table+8+8*(number-1)+4))
	chain := programChain{durationMs: dvdTimeMs(ifo.bytes(start+4, 4))}
	for index := range 8 {
		chain.audioControl = append(chain.audioControl, ifo.be16(start+0x0C+2*index))
	}
	for index := range 32 {
		chain.subpictureControl = append(chain.subpictureControl, ifo.be32(start+0x1C+4*index))
	}
	programMap, cellTable := start+ifo.be16(start+0xE6), start+ifo.be16(start+0xE8)
	for index := range ifo.u8(start + 2) {
		chain.programEntryCells = append(chain.programEntryCells, ifo.u8(programMap+index))
	}
	for index := range ifo.u8(start + 3) {
		cell := cellTable + 24*index
		chain.cells = append(chain.cells, dvdCell{
			flags: ifo.u8(cell), durationMs: dvdTimeMs(ifo.bytes(cell+4, 4)),
			firstSector: ifo.be32(cell + 8), lastVOBU: ifo.be32(cell + 16), lastSector: ifo.be32(cell + 20),
		})
	}
	return chain, ifo.err("a program chain")
}

// chainsDurationMs is how long a title's program chains play, by their own playing times.
func chainsDurationMs(chains []programChain) int64 {
	var total int64
	for _, chain := range chains {
		total += chain.durationMs
	}
	return total
}

// dvdTimeMs reads a DVD's time (dvd_time_t): hours, minutes, seconds and frames in BCD, the frames'
// rate in the last byte's top two bits.
func dvdTimeMs(time []byte) int64 {
	bcd := func(value byte) int64 { return int64(value>>4)*10 + int64(value&0x0F) }
	durationMs := ((bcd(time[0])*60+bcd(time[1]))*60 + bcd(time[2])) * 1000
	switch time[3] >> 6 {
	case 1:
		durationMs += bcd(time[3]&0x3F) * 1000 / 25
	case 3:
		durationMs += bcd(time[3]&0x3F) * 1001 / 30
	}
	return durationMs
}

// assemble makes title number of the set from its program chains and parts: its cells' sectors as
// runs of the image, angle 1 of each angle block, and its chapters at its parts' first cells; and
// the cells it plays, for its clock.
func (set *titleSet) assemble(number int, chains []programChain, parts []part) (Title, []clockCell, error) {
	title := Title{Kind: DVD, Number: number}
	count := 0
	for _, chain := range chains {
		count += len(chain.cells)
	}
	if count > maxTitleCells {
		return Title{}, nil, fmt.Errorf("%w: a title of %d cells", ErrMalformed, count)
	}
	var cells []clockCell
	var titleBytes int64
	cellStartMs := make([][]int64, len(chains))
	for chainIndex, chain := range chains {
		for _, cell := range chain.cells {
			cellStartMs[chainIndex] = append(cellStartMs[chainIndex], title.DurationMs)
			if (cell.flags>>4)&3 == cellAngleBlock && cell.flags>>6 != cellFirstInBlock {
				continue // another angle of the block
			}
			if cell.flags&cellInterleaved != 0 {
				return Title{}, nil, fmt.Errorf("%w: title %d", ErrInterleaved, number)
			}
			if cell.lastSector < cell.firstSector {
				return Title{}, nil, fmt.Errorf("%w: a cell that ends before it starts", ErrMalformed)
			}
			firstByte, lengthBytes := cell.firstSector*sectorBytes, (cell.lastSector-cell.firstSector+1)*sectorBytes
			pieces, err := slice(set.vobs, firstByte, lengthBytes)
			if err != nil {
				return Title{}, nil, err
			}
			cells = append(cells, newClockCell(cell, titleBytes))
			title.Extents = appendJoined(title.Extents, pieces...)
			titleBytes += lengthBytes
			title.DurationMs += cell.durationMs
		}
	}
	title.ChaptersMs = dvdChapters(chains, parts, cellStartMs)
	if len(chains) > 0 {
		title.Tracks = set.tracks(chains[0])
	}
	return title, cells, nil
}

// dvdChapters are where a title's parts start, from the start of each part's program's entry cell.
func dvdChapters(chains []programChain, parts []part, cellStartMs [][]int64) []int64 {
	chapters := []int64{0}
	for _, part := range parts {
		entries := chains[part.programChain].programEntryCells
		if part.program < 1 || part.program > len(entries) {
			continue
		}
		cell := entries[part.program-1] - 1
		if cell >= 0 && cell < len(cellStartMs[part.programChain]) {
			chapters = append(chapters, cellStartMs[part.programChain][cell])
		}
	}
	slices.Sort(chapters)
	return slices.Compact(chapters)
}

// tracks are the languages of the audio and subpicture streams chain plays, by the stream
// numbers FFmpeg's program stream demuxer gives them.
func (set *titleSet) tracks(chain programChain) []Track {
	var tracks []Track
	ifo := set.ifo
	for index := range min(ifo.be16(ifoAudioCount), len(chain.audioControl)) {
		control, attributes := chain.audioControl[index], ifoAudioAttributes+8*index
		base, isKnown := audioStreamBase(ifo.u8(attributes) >> 5)
		if control&0x8000 == 0 || !isKnown {
			continue
		}
		language := ""
		if (ifo.u8(attributes)>>2)&3 == 1 {
			language = twoLetters(ifo.bytes(attributes+2, 2))
		}
		tracks = append(tracks, Track{ID: base + int64((control>>8)&7), Kind: "audio", Language: language})
	}
	for index := range min(ifo.be16(ifoSubpictureCount), len(chain.subpictureControl)) {
		control, attributes := chain.subpictureControl[index], ifoSubpictureAttrs+6*index
		if control&0x80000000 == 0 {
			continue
		}
		language := ""
		if ifo.u8(attributes)&3 == 1 {
			language = twoLetters(ifo.bytes(attributes+2, 2))
		}
		tracks = append(tracks, Track{ID: 0x20 + (control>>24)&0x1F, Kind: "subtitle", Language: language})
	}
	slices.SortFunc(tracks, func(a, b Track) int { return cmp.Compare(a.ID, b.ID) })
	return tracks
}

// audioStreamBase is the first stream identifier FFmpeg's program stream demuxer gives an audio
// coding of a DVD: AC-3 0x80, DTS 0x88, LPCM 0xA0, MPEG audio its start code 0x1C0.
func audioStreamBase(coding int) (int64, bool) {
	switch coding {
	case 0:
		return 0x80, true
	case 2, 3:
		return 0x1C0, true
	case 4:
		return 0xA0, true
	case 6:
		return 0x88, true
	}
	return 0, false
}

// twoLetters is an ISO 639-1 code as a DVD writes it, or "" for anything else.
func twoLetters(code []byte) string {
	if len(code) != 2 || code[0] < 'a' || code[0] > 'z' || code[1] < 'a' || code[1] > 'z' {
		return ""
	}
	return string(code)
}
