// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// startSpeedPercent is the speed, in percent of real time, a quality a playback starts on must be
// expected to be made at on its node: three times real time, a six-second chunk in two seconds
// (design/plans/start-quality-by-node-speed.md §2). It is the lead's default, which the owner has
// not yet confirmed (the plan's question 1).
const startSpeedPercent = 300

// realTimePercent is real time in percent: a speed's cost, the seconds spent on each second of
// film, is it over the speed.
const realTimePercent = 100

// trialPictureWidth and trialPictureHeight are the shape of the trials' generated picture: 16:9, as
// every clip is (internal/node/speedtrial's Kind.source).
const trialPictureWidth, trialPictureHeight = 16, 9

// trialLine is how fast a node makes the qualities of one kind of film (plan §2, the lead's
// reading): its two trials, the top quality of the kind's size and the lowest, taken as the seconds
// it spends on each second of film growing in a straight line with the pixels made, from what the
// decode alone costs, so a quality between them is judged by the time its pixels put on the line.
// The speeds it gives lie under the straight line between the two speeds: the more cautious reading.
type trialLine struct {
	lowestPixels, topPixels float64
	// lowestCost and topCost are the seconds spent on each second of film: realTimePercent over the
	// speed.
	lowestCost, topCost float64
}

// speedPercentAt is the speed the line expects a quality of pixels to be made at: at or below the
// lowest's pixels, the lowest's own, never faster than measured; above the top's, further along the
// line, slower.
func (line trialLine) speedPercentAt(pixels float64) float64 {
	cost := line.lowestCost
	if pixels > line.lowestPixels && line.topPixels > line.lowestPixels {
		slope := max(0, line.topCost-line.lowestCost) / (line.topPixels - line.lowestPixels)
		cost += slope * (pixels - line.lowestPixels)
	}
	return realTimePercent / cost
}

// trialKind is one kind of film a node was tried on, with its line.
type trialKind struct {
	codec            string
	bitDepth, height int
	line             trialLine
}

// trialKindsOf are the kinds the trials measured both qualities of; a kind with either not measured,
// not tried or failed, is not one (plan §2, the lead's reading), and borrows another's.
func trialKindsOf(trials []nodeproto.SpeedTrial) []trialKind {
	type kindKey struct {
		codec            string
		bitDepth, height int
	}
	measured := map[kindKey][]nodeproto.SpeedTrial{}
	var order []kindKey
	for _, trial := range trials {
		if trial.SpeedPercent <= 0 || trial.MadeHeight <= 0 {
			continue
		}
		key := kindKey{codec: trial.Codec, bitDepth: trial.BitDepth, height: trial.Height}
		if _, isKnown := measured[key]; !isKnown {
			order = append(order, key)
		}
		measured[key] = append(measured[key], trial)
	}
	var kinds []trialKind
	for _, key := range order {
		rows := measured[key]
		lowest := slices.MinFunc(rows, func(a, b nodeproto.SpeedTrial) int { return a.MadeHeight - b.MadeHeight })
		top := slices.MaxFunc(rows, func(a, b nodeproto.SpeedTrial) int { return a.MadeHeight - b.MadeHeight })
		if lowest.MadeHeight == top.MadeHeight {
			continue
		}
		line := trialLine{
			lowestPixels: trialPixels(lowest.MadeHeight), topPixels: trialPixels(top.MadeHeight),
			lowestCost: realTimePercent / float64(lowest.SpeedPercent),
			topCost:    realTimePercent / float64(top.SpeedPercent),
		}
		kinds = append(kinds, trialKind{codec: key.codec, bitDepth: key.bitDepth, height: key.height, line: line})
	}
	return kinds
}

// trialPixels are the pixels of a trial's quality of height, in its 16:9 picture.
func trialPixels(height int) float64 {
	return float64(height) * float64(height) * trialPictureWidth / trialPictureHeight
}

// trialLineFor is the line a film whose picture is picture is judged by (plan §2): its own kind's,
// the same codec at the same or a greater bit depth and size, the nearest such, smallest first; else,
// the kind the node made slowest of all, by its top quality. False with no kind measured.
func trialLineFor(trials []nodeproto.SpeedTrial, picture nodeproto.ProbeStream) (trialLine, bool) {
	kinds := trialKindsOf(trials)
	if len(kinds) == 0 {
		return trialLine{}, false
	}
	bitDepth, width, height := picture.Video.BitDepth(), picture.Video.Width, picture.Video.Height
	var nearest *trialKind
	for index := range kinds {
		kind := &kinds[index]
		holds := kind.codec == picture.Codec && kind.bitDepth >= bitDepth && kind.height >= height &&
			kind.height*trialPictureWidth/trialPictureHeight >= width
		if holds && (nearest == nil || kind.height < nearest.height ||
			(kind.height == nearest.height && kind.bitDepth < nearest.bitDepth)) {
			nearest = kind
		}
	}
	if nearest != nil {
		return nearest.line, true
	}
	slowest := slices.MaxFunc(kinds, func(a, b trialKind) int {
		switch {
		case a.line.topCost > b.line.topCost:
			return 1
		case a.line.topCost < b.line.topCost:
			return -1
		}
		return 0
	})
	return slowest.line, true
}

// heldByNodeSpeed is the place among the offered pictures, highest first, a start at from takes so
// its node is expected to make it at startSpeedPercent or faster (plan §2), and the speed expected of
// it: from or lower, the film's own picture copied passing, as nothing is made of it; the lowest when
// none passes; from, at speed 0, where the node's trials measured nothing, as before they did.
func (service *Service) heldByNodeSpeed(film cutFilm, offered offer, from int) (place, speedPercent int) {
	if service.encoders == nil {
		return from, 0
	}
	line, isMeasured := trialLineFor(service.encoders.SpeedTrials(film.file.NodeID), film.picture)
	if !isMeasured {
		return from, 0
	}
	width, height := film.picture.Video.Width, film.picture.Video.Height
	speedOf := func(picture Stream) float64 {
		quality := float64(picture.Quality.Height)
		pixels := quality * quality * trialPictureWidth / trialPictureHeight
		if width > 0 && height > 0 {
			pixels = quality * quality * float64(width) / float64(height)
		}
		return line.speedPercentAt(pixels)
	}
	last := len(offered.pictures) - 1
	for index := from; index <= last; index++ {
		picture := offered.pictures[index]
		if picture.Kind != nodeproto.StreamQuality {
			return index, 0
		}
		if speed := speedOf(picture); speed >= startSpeedPercent || index == last {
			return index, int(speed)
		}
	}
	return from, 0
}
