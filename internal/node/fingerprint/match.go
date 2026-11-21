// SPDX-License-Identifier: AGPL-3.0-or-later

package fingerprint

import (
	"math/bits"
	"slices"
	"time"
)

// pointDuration is how much sound each point of a fingerprint stands for: chromaprint's default
// algorithm reads its audio at 11,025 samples a second in frames of 4,096 samples, each a third of a
// frame after the last, so a point every 4096/3/11025 seconds, about 124 ms.
const pointDuration = time.Duration(4096 * int64(time.Second) / 3 / 11025)

// The thresholds a repeat is found by are intro-skipper's for Jellyfin, which seasons of real
// television have tested (plugins-and-workarounds.md §3).

// maxPointBitDifference is how many of a point's 32 bits may differ for two points to be the same
// sound. Unrelated sound differs in about half, 16; the same sound read at another phase of the
// frame, as it is when it starts at another time in another file, in a few.
const maxPointBitDifference = 6

// maxGap is how long a run of the same sound may go without a point the same and still be one run: a
// word spoken over a theme in one episode and not another breaks no intro.
const maxGap = 3500 * time.Millisecond

// minRepeat is the shortest repeat taken for an intro or credits: a jingle, a logo's sting or a moment
// of silence shared by two episodes is shorter.
const minRepeat = 15 * time.Second

// Span is a stretch of a fingerprint, in its points: Start the first, End one past the last.
type Span struct {
	Start int
	End   int
}

// Times is the span as times from the fingerprint's start: where the sound it shares starts and
// stops being heard, each point's time moved by how far into its sound it is the same (runStartLead,
// runEndLead). A span from the first point starts where the fingerprint does: the sound was shared
// from before it.
func (span Span) Times() (start, end time.Duration) {
	end = time.Duration(span.End-1)*pointDuration + runEndLead
	if span.Start == 0 {
		return 0, end
	}
	return time.Duration(span.Start)*pointDuration + runStartLead, end
}

// Each point is made from the sound of about 2.5 s from its start: its frame, three points long, and
// chromaprint's filters across the frames after it. So a point is the same in two files once most of
// what it hears is shared, which a run's first point is a little before the shared sound starts and
// its last a little before the shared sound ends. Measured on the generated season
// (TestMeasure_FindsASeasonsIntroAndCreditsToWithinASecond): a run's first point starts 0.5 to 1.4 s
// before the shared sound, and its last 1.1 to 1.3 s before the shared sound ends, which these
// leads take back.
const (
	runStartLead = 900 * time.Millisecond
	runEndLead   = 1240 * time.Millisecond
)

// Shared is the longest stretch of sound two fingerprints share, as a span of each, found at every
// offset of one against the other: at each, the points whose bits differ by no more than
// maxPointBitDifference are the same sound, and the same points with gaps of no more than maxGap are
// one stretch. One shorter than minRepeat is no stretch: false.
func Shared(one, other []uint32) (inOne, inOther Span, isShared bool) {
	maxGapPoints := int(maxGap / pointDuration)
	minPoints := int((minRepeat + runStartLead - runEndLead + pointDuration - 1) / pointDuration)
	best := 0
	for shift := -(len(other) - 1); shift < len(one); shift++ {
		// At this shift, point i of one is heard against point i-shift of other.
		first, last := max(0, shift), min(len(one), len(other)+shift)
		runStart, lastSame := -1, -1
		for index := first; index < last; index++ {
			if bits.OnesCount32(one[index]^other[index-shift]) > maxPointBitDifference {
				continue
			}
			if lastSame < 0 || index-lastSame > maxGapPoints {
				runStart = index
			}
			lastSame = index
			if length := lastSame - runStart + 1; length > best {
				best = length
				inOne = Span{Start: runStart, End: lastSame + 1}
				inOther = Span{Start: runStart - shift, End: lastSame + 1 - shift}
			}
		}
	}
	if best < minPoints {
		return Span{}, Span{}, false
	}
	return inOne, inOther, true
}

// Repeat is the stretch of one fingerprint that others of its set share, and how many share it.
type Repeat struct {
	Span
	// HeardIn is how many of the set's other fingerprints share it; 0 is none, and no span.
	HeardIn int
}

// Repeats are, for each of a set's fingerprints in order, the stretch the others share with it: each
// pair is compared (Shared), and of the stretches a fingerprint shares with the others, the one most
// of them overlap is taken, starting and ending where most of those that overlap it do (their
// medians), so one episode that shares a longer stretch with another, a two-parter's recap, moves
// neither end. A stretch is a repeat once one other shares it: two episodes.
func Repeats(prints [][]uint32) []Repeat {
	shared := make([][]Span, len(prints))
	for one := range prints {
		for other := one + 1; other < len(prints); other++ {
			inOne, inOther, isShared := Shared(prints[one], prints[other])
			if isShared {
				shared[one] = append(shared[one], inOne)
				shared[other] = append(shared[other], inOther)
			}
		}
	}
	repeats := make([]Repeat, len(prints))
	for index, spans := range shared {
		var agreeing []Span
		for _, candidate := range spans {
			overlapping := slices.DeleteFunc(slices.Clone(spans),
				func(span Span) bool { return !overlaps(candidate, span) })
			if len(overlapping) > len(agreeing) {
				agreeing = overlapping
			}
		}
		if len(agreeing) == 0 {
			continue
		}
		starts, ends := make([]int, len(agreeing)), make([]int, len(agreeing))
		for at, span := range agreeing {
			starts[at], ends[at] = span.Start, span.End
		}
		repeats[index] = Repeat{Span: Span{Start: median(starts), End: median(ends)}, HeardIn: len(agreeing)}
	}
	return repeats
}

// overlaps is whether two spans share at least half the shorter of them.
func overlaps(one, other Span) bool {
	shared := min(one.End, other.End) - max(one.Start, other.Start)
	return shared*2 >= min(one.End-one.Start, other.End-other.Start)
}

// median is the middle of values, the lower middle of an even count.
func median(values []int) int {
	slices.Sort(values)
	return values[(len(values)-1)/2]
}
