// SPDX-License-Identifier: AGPL-3.0-or-later

package fingerprint

import (
	"math/rand/v2"
	"testing"
	"time"
)

// pointsOf is how many points a fingerprint has for duration of sound.
func pointsOf(duration time.Duration) int { return int(duration / pointDuration) }

// unrelated is count points of sound no other fingerprint shares, seeded so each run is the same.
func unrelated(random *rand.Rand, count int) []uint32 {
	found := make([]uint32, count)
	for index := range found {
		found[index] = random.Uint32()
	}
	return found
}

// heardAgain is shared as another file hears it: a few bits of each point differ, as they do when
// the same sound starts at another phase of chromaprint's frame.
func heardAgain(random *rand.Rand, shared []uint32) []uint32 {
	found := make([]uint32, len(shared))
	for index, point := range shared {
		for range random.IntN(maxPointBitDifference/2 + 1) {
			point ^= 1 << random.IntN(32)
		}
		found[index] = point
	}
	return found
}

// fingerprintOf is a fingerprint of before points of its own sound, then shared heard again, then after.
func fingerprintOf(random *rand.Rand, before int, shared []uint32, after int) []uint32 {
	found := unrelated(random, before)
	found = append(found, heardAgain(random, shared)...)
	return append(found, unrelated(random, after)...)
}

// D101: the stretch two fingerprints share is found where it is in each, whatever its offset in
// either, and a stretch shorter than minRepeat is none.
func TestShared_FindsTheStretchTwoFingerprintsShareAtItsOffsetInEach(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // test data, not a secret
	intro := unrelated(random, pointsOf(40*time.Second))
	one := fingerprintOf(random, 30, intro, 400)
	other := fingerprintOf(random, 170, intro, 90)
	inOne, inOther, isShared := Shared(one, other)
	if !isShared || inOne != (Span{Start: 30, End: 30 + len(intro)}) ||
		inOther != (Span{Start: 170, End: 170 + len(intro)}) {
		t.Errorf("Shared = %+v, %+v, %v; want the intro at points 30 and 170", inOne, inOther, isShared)
	}
	jingle := unrelated(random, pointsOf(minRepeat/2))
	jingleOne, jingleOther := fingerprintOf(random, 30, jingle, 400), fingerprintOf(random, 90, jingle, 300)
	if inOne, inOther, isShared := Shared(jingleOne, jingleOther); isShared {
		t.Errorf("a %v jingle shared = %+v, %+v; want no stretch shorter than %v", minRepeat/2, inOne, inOther,
			minRepeat)
	}
}

// D101: a stretch broken by less than maxGap of sound one file has and the other not, a line spoken
// over the theme, is still one stretch; one broken by more is two, and the longer is taken.
func TestShared_KeepsAStretchWholeAcrossAGapShorterThanMaxGap(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // test data, not a secret
	first, second := unrelated(random, pointsOf(20*time.Second)), unrelated(random, pointsOf(30*time.Second))
	for _, check := range []struct {
		gap  time.Duration
		want Span
	}{
		{maxGap - time.Second, Span{Start: 50, End: 50 + len(first) + pointsOf(maxGap-time.Second) + len(second)}},
		{maxGap + time.Second, Span{Start: 50 + len(first) + pointsOf(maxGap+time.Second), End: 50 + len(first) +
			pointsOf(maxGap+time.Second) + len(second)}},
	} {
		gap := pointsOf(check.gap)
		one := append(fingerprintOf(random, 50, first, 0), fingerprintOf(random, gap, second, 100)...)
		other := append(fingerprintOf(random, 50, first, 0), fingerprintOf(random, gap, second, 100)...)
		if inOne, _, isShared := Shared(one, other); !isShared || inOne != check.want {
			t.Errorf("across a gap of %v: %+v, %v; want %+v", check.gap, inOne, isShared, check.want)
		}
	}
}

// D101: of a season's fingerprints, each that shares its intro with another has it found, with how
// many share it, and the one that shares it with none has none: two episodes that also share a longer
// stretch, a recap, still have the intro the most of the others share, not the recap.
func TestRepeats_FindsEachFingerprintsStretchTheOthersShareAndHowMany(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // test data, not a secret
	intro := unrelated(random, pointsOf(30*time.Second))
	recap := unrelated(random, pointsOf(60*time.Second))
	prints := [][]uint32{
		fingerprintOf(random, 40, intro, 300),
		fingerprintOf(random, 100, intro, 200),
		fingerprintOf(random, 10, intro, 400),
		unrelated(random, 600),
		// Two that open on a recap of the episode before.
		append(fingerprintOf(random, 0, recap, 0), fingerprintOf(random, 20, intro, 100)...),
		append(fingerprintOf(random, 0, recap, 0), fingerprintOf(random, 60, intro, 100)...),
	}
	repeats := Repeats(prints)
	want := []Repeat{
		{Span: Span{Start: 40, End: 40 + len(intro)}, HeardIn: 4},
		{Span: Span{Start: 100, End: 100 + len(intro)}, HeardIn: 4},
		{Span: Span{Start: 10, End: 10 + len(intro)}, HeardIn: 4},
		{},
		{Span: Span{Start: len(recap) + 20, End: len(recap) + 20 + len(intro)}, HeardIn: 3},
		{Span: Span{Start: len(recap) + 60, End: len(recap) + 60 + len(intro)}, HeardIn: 3},
	}
	for index := range want {
		if repeats[index] != want[index] {
			t.Errorf("fingerprint %d: %+v; want %+v", index, repeats[index], want[index])
		}
	}
}

// Times places a span's sound where it is heard, its points' times moved by the leads, and one from
// the first point where the fingerprint starts.
func TestSpanTimes_AreItsPointsTimesMovedByTheLeads(t *testing.T) {
	t.Parallel()
	start, end := Span{Start: 10, End: 21}.Times()
	if start != 10*pointDuration+runStartLead || end != 20*pointDuration+runEndLead {
		t.Errorf("Times = %v, %v; want %v, %v", start, end, 10*pointDuration+runStartLead, 20*pointDuration+runEndLead)
	}
	if start, _ := (Span{Start: 0, End: 21}).Times(); start != 0 {
		t.Errorf("a span from the first point starts at %v; want where the fingerprint does, 0", start)
	}
}

// A raw fingerprint is little-endian 32-bit points; bytes that are not whole points are an error.
func TestPoints_ReadsARawFingerprintsLittleEndianPoints(t *testing.T) {
	t.Parallel()
	found, err := points([]byte{1, 0, 0, 0, 0, 0, 0, 0x80})
	if err != nil || len(found) != 2 || found[0] != 1 || found[1] != 0x80000000 {
		t.Errorf("points = %v, %v; want [1 0x80000000]", found, err)
	}
	if _, err := points([]byte{1, 2, 3}); err == nil {
		t.Error("three bytes read as points; want an error")
	}
}
