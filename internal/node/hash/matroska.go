// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"fmt"
	"io"

	"github.com/kstenerud/orpheum/internal/node/container"
)

// Bounds on what a hostile file can make the node read or walk.
const (
	maxSeekHeadBytes = 64 << 10
	maxCuesBytes     = 32 << 20
)

// matroskaPayload returns the span from the first Cluster's start to the last Cluster's end
// (D179 §3). The first is found by stepping over the few elements before it; the last by
// starting at the last cue's cluster, found through the SeekHead, and stepping to the end of
// the segment. Without Cues it steps over every cluster header instead: more reads, the same
// answer.
func matroskaPayload(file io.ReaderAt, sizeBytes int64) (payloadSpan, error) {
	header, err := container.ReadElement(file, sizeBytes, 0)
	if err != nil || header.ID != container.IDEBML {
		return payloadSpan{}, fmt.Errorf("%w: no EBML header", container.ErrNotThisContainer)
	}
	segment, err := container.ReadElement(file, sizeBytes, header.End())
	if err != nil || segment.ID != container.IDSegment {
		return payloadSpan{}, fmt.Errorf("%w: no Segment after the EBML header", container.ErrNotThisContainer)
	}
	segmentEnd := sizeBytes
	if segment.DataSize != container.UnknownSize {
		segmentEnd = min(segment.End(), sizeBytes)
	}

	firstCluster, cuesPosition, err := walkToFirstCluster(file, sizeBytes, segment.DataStart, segmentEnd)
	if err != nil {
		return payloadSpan{}, err
	}
	from := firstCluster
	if cuesPosition >= 0 {
		// A cue that points anywhere but at a cluster is ignored: the walk from the first
		// cluster gives the same answer.
		if lastCued, found := lastCuedCluster(file, sizeBytes, segment.DataStart, cuesPosition); found {
			from = max(lastCued, firstCluster)
		}
	}
	lastEnd, err := walkToLastClusterEnd(file, sizeBytes, from, segmentEnd)
	if err != nil {
		return payloadSpan{}, err
	}
	return payloadSpan{start: firstCluster, end: lastEnd, anchor: AnchorMatroska}, nil
}

// walkToFirstCluster steps over top-level elements from the segment's data to the first
// Cluster, and notes where the SeekHead says the Cues are (-1 when it does not say).
func walkToFirstCluster(
	file io.ReaderAt, sizeBytes, segmentDataStart, segmentEnd int64,
) (firstCluster, cuesPosition int64, err error) {
	cuesPosition = -1
	position := segmentDataStart
	for range container.MaxElementHops {
		if position >= segmentEnd {
			return 0, 0, fmt.Errorf("%w: the segment has no cluster", container.ErrNotThisContainer)
		}
		found, readErr := container.ReadElement(file, sizeBytes, position)
		if readErr != nil {
			return 0, 0, readErr
		}
		switch {
		case found.ID == container.IDCluster:
			return found.Start, cuesPosition, nil
		case found.DataSize == container.UnknownSize:
			return 0, 0, fmt.Errorf("%w: a header element of unknown size", container.ErrNotThisContainer)
		case found.ID == container.IDSeekHead && cuesPosition < 0:
			cuesPosition = seekPositionOfCues(file, sizeBytes, found, segmentDataStart)
		}
		position = found.End()
	}
	return 0, 0, fmt.Errorf("%w: too many elements before the first cluster", container.ErrNotThisContainer)
}

// walkToLastClusterEnd steps over top-level elements from a cluster to the segment's end and
// returns where the last Cluster ends.
func walkToLastClusterEnd(file io.ReaderAt, sizeBytes, from, segmentEnd int64) (int64, error) {
	lastEnd := int64(-1)
	position := from
	for range container.MaxElementHops {
		if position >= segmentEnd {
			if lastEnd < 0 {
				return 0, fmt.Errorf("%w: no cluster at the cued position", container.ErrNotThisContainer)
			}
			return lastEnd, nil
		}
		found, err := container.ReadElement(file, sizeBytes, position)
		if err != nil {
			return 0, err
		}
		if found.DataSize == container.UnknownSize || found.End() > sizeBytes {
			return 0, fmt.Errorf("%w: an element at %d of unknown size or past the end of the file",
				container.ErrNotThisContainer, found.Start)
		}
		if found.ID == container.IDCluster {
			lastEnd = found.End()
		}
		position = found.End()
	}
	return 0, fmt.Errorf("%w: too many elements after the first cluster", container.ErrNotThisContainer)
}

// seekPositionOfCues reads a SeekHead and returns the Cues' absolute position, or -1.
func seekPositionOfCues(file io.ReaderAt, sizeBytes int64, seekHead container.Element, segmentDataStart int64) int64 {
	children, err := container.ReadChildren(file, sizeBytes, seekHead, maxSeekHeadBytes, 3)
	if err != nil {
		return -1
	}
	for _, seek := range children {
		if seek.ID != container.IDSeek {
			continue
		}
		var target, position uint64
		var hasTarget, hasPosition bool
		for _, field := range seek.Children {
			switch field.ID {
			case container.IDSeekID:
				target, hasTarget = field.Integer(), true
			case container.IDSeekPosition:
				position, hasPosition = field.Integer(), true
			}
		}
		isCues := hasTarget && hasPosition && target == container.IDCues
		if isCues && position < uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return segmentDataStart + int64(position)
		}
	}
	return -1
}

// lastCuedCluster reads the Cues and returns the absolute position of the latest cluster any
// cue points at, when that position holds a Cluster.
func lastCuedCluster(file io.ReaderAt, sizeBytes, segmentDataStart, cuesPosition int64) (int64, bool) {
	cues, err := container.ReadElement(file, sizeBytes, cuesPosition)
	if err != nil || cues.ID != container.IDCues || cues.DataSize == container.UnknownSize {
		return 0, false
	}
	points, err := container.ReadChildren(file, sizeBytes, cues, maxCuesBytes, 3)
	if err != nil {
		return 0, false
	}
	latest := latestClusterPosition(points)
	if latest >= uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
		return 0, false
	}
	at := segmentDataStart + int64(latest) //nolint:gosec // bounded by sizeBytes above
	cluster, err := container.ReadElement(file, sizeBytes, at)
	if err != nil || cluster.ID != container.IDCluster {
		return 0, false
	}
	return at, true
}

// latestClusterPosition is the largest CueClusterPosition among the cue points.
func latestClusterPosition(points []container.Node) uint64 {
	var latest uint64
	for _, point := range points {
		if point.ID != container.IDCuePoint {
			continue
		}
		for _, positions := range point.Children {
			if positions.ID != container.IDCueTrackPositions {
				continue
			}
			for _, field := range positions.Children {
				if field.ID == container.IDCueClusterPosition {
					latest = max(latest, field.Integer())
				}
			}
		}
	}
	return latest
}
