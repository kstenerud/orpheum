// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueuetest

import "io"

// Random is a random source that gives the same eight bytes over and over, so a test knows the
// jitter every retry gets: the queue reads eight bytes per retry.
type Random [8]byte

// Read fills p with the eight bytes, repeated.
func (random Random) Read(p []byte) (int, error) {
	for index := range p {
		p[index] = random[index%len(random)]
	}
	return len(p), nil
}

// Unjittered is a source whose jitter moves no wait: the middle of the range.
func Unjittered() io.Reader { return Random{0x80} }

// EarliestJitter is a source whose jitter moves every wait as early as it goes: a tenth sooner.
func EarliestJitter() io.Reader { return Random{} }

// LatestJitter is a source whose jitter moves every wait as late as it goes: just short of a tenth later.
func LatestJitter() io.Reader { return Random{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff} }
