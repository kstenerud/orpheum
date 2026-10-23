// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Slow work leaves a disk's last reader to faster work that came for it within the last five seconds,
// waiting on a destination of its own until then, so the faster work's next piece takes the reader
// without bumping a read-through that had only just started; a person's read is never held, and once
// the faster work has been quiet the slow work takes the reader (D255 §4; staging, 2026-10-23: 2,654
// read-throughs bumped in half an hour of a first scan).
func TestAssign_SlowWorkLeavesADisksLastReaderToFasterWorkThatCameLately(t *testing.T) {
	t.Parallel()
	bench := newBumpBench(t) // tank: one reader and one more
	if answer := bench.assign(1, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("an examining on idle tank was answered %v; want it given", answer)
	}
	examinedAt := bench.now.Now()
	bench.now.Advance(2 * time.Second)

	parking, isParked := jobqueue.ParkingOf(bench.assign(2, jobs.KindReadThrough, jobs.PriorityBackground,
		readThroughOf("/media")))
	if !isParked || parking.Destination != jobs.LastReaderDestination(1, "zfs:tank") || !parking.IsBackPressure ||
		!parking.ProbeAt.Equal(examinedAt.Add(5*time.Second)) {
		t.Fatalf("a read-through for tank's last reader, 2 s after an examining, waits on %+v (parked %v); want "+
			"tank's last reader, back-pressure, until 5 s after the examining", parking, isParked)
	}
	if answer := bench.assign(3, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("the next examining was answered %v; want it given the reader kept for it", answer)
	}
	if len(bench.stops) != 0 {
		t.Errorf("the next examining stopped %+v; want nothing bumped", bench.stops)
	}
	bench.now.Advance(6 * time.Second)
	if answer := bench.assign(4, jobs.KindReadThrough, jobs.PriorityBackground,
		readThroughOf("/media")); jobqueue.IsAssigned(answer) {
		t.Errorf("a read-through on full tank was given; want it to wait for tank's readers, as before")
	}
}

// Faster work that has been quiet five seconds keeps nothing: slow work takes the disk's last reader.
// While a reader stays free after it, slow work takes one whatever came; and a read a person asked for
// is never held for faster work.
func TestAssign_SlowWorkTakesTheLastReaderOnceFasterWorkIsQuiet(t *testing.T) {
	t.Parallel()
	bench := newBumpBench(t) // tank: one reader and one more
	if answer := bench.assign(1, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("an examining on idle tank was answered %v; want it given", answer)
	}
	if answer := bench.assign(2, jobs.KindReadThrough, jobs.PriorityPersonsRequest,
		readThroughOf("/media")); !jobqueue.IsAssigned(answer) {
		t.Errorf("a person's read-through, just after an examining, was answered %v; want it given", answer)
	}

	quiet := newBumpBench(t)
	if answer := quiet.assign(1, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("an examining on idle tank was answered %v; want it given", answer)
	}
	quiet.now.Advance(5 * time.Second)
	if answer := quiet.assign(2, jobs.KindReadThrough, jobs.PriorityBackground,
		readThroughOf("/media")); !jobqueue.IsAssigned(answer) {
		t.Errorf("a read-through 5 s after the last examining was answered %v; want it given the last reader",
			answer)
	}
}
