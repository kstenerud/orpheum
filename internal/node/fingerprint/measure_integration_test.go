// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package fingerprint

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// tune is a melody of two voices, a note every quarter second chosen by a hash of its time and seed,
// so it never repeats within itself and two seeds share nothing: chromaprint hears a melody's chroma,
// and noise, whose chroma is flat, would match itself everywhere.
func tune(seed int, length time.Duration) string {
	note := func(scale string) string {
		hash := fmt.Sprintf("sin(floor(t*4)*%s+%d)*43758.5453", scale, seed)
		return "floor(24*(" + hash + "-floor(" + hash + ")))"
	}
	return fmt.Sprintf("aevalsrc=0.2*sin(2*PI*220*pow(2\\,%s/12)*t)+0.2*sin(2*PI*110*pow(2\\,%s/12)*t):d=%.3f:s=48000",
		note("12.9898"), note("78.233"), length.Seconds())
}

// episode is a generated episode of a season: its own sound for lead, the season's intro, its own
// story, the season's credits, and its own sound for tail, in 5.1, as a film's track often is.
type episode struct {
	lead, story, tail time.Duration
}

const (
	introLength   = 20 * time.Second
	creditsLength = 20 * time.Second
	openingLength = 40 * time.Second
	closingLength = 40 * time.Second
	// Tolerance is how near the found intro and credits must be to where they are (B1 Phase D).
	tolerance = time.Second
)

func (shape episode) duration() time.Duration {
	return shape.lead + introLength + shape.story + creditsLength + shape.tail
}

// D101, D115: a season's episodes, the same intro and credits at a different offset in each amid sound
// of their own, in 5.1, are fingerprinted from their openings and closings, and the stretch the others
// share is found in each to within a second of where it is.
func TestMeasure_FindsASeasonsIntroAndCreditsToWithinASecond(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	fingerprinter := NewFingerprinter(runner)
	season := []episode{
		{lead: 3 * time.Second, story: 50 * time.Second, tail: 5 * time.Second},
		{lead: 7300 * time.Millisecond, story: 45 * time.Second, tail: 11500 * time.Millisecond},
		{lead: 11650 * time.Millisecond, story: 55 * time.Second, tail: 2 * time.Second},
		{lead: 0, story: 48 * time.Second, tail: 8200 * time.Millisecond},
	}
	directory := t.TempDir()
	var openings, closings [][]uint32
	for index, shape := range season {
		path := filepath.Join(directory, fmt.Sprintf("episode-%d.mkv", index))
		var inputs []string
		var labels string
		for _, part := range []struct {
			seed   int
			length time.Duration
		}{
			{index + 1, shape.lead},
			{1000, introLength},
			{index + 101, shape.story},
			{2000, creditsLength},
			{index + 201, shape.tail},
		} {
			if part.length == 0 {
				continue // an episode that opens on the intro
			}
			labels += fmt.Sprintf("[%d]", len(inputs)/4)
			inputs = append(inputs, "-f", "lavfi", "-i", tune(part.seed, part.length))
		}
		testutil.GenerateMedia(t, path, append(inputs, "-filter_complex",
			labels+fmt.Sprintf("concat=n=%d:v=0:a=1,aformat=channel_layouts=5.1", len(inputs)/4), "-c:a", "flac")...)
		film := execx.Film{Path: path}
		opening, err := fingerprinter.Fingerprint(t.Context(), film, 0, 0, openingLength)
		if err != nil {
			t.Fatal(err)
		}
		closing, err := fingerprinter.Fingerprint(t.Context(), film, 0, shape.duration()-closingLength, closingLength)
		if err != nil {
			t.Fatal(err)
		}
		openings, closings = append(openings, opening), append(closings, closing)
	}
	introRepeats, creditsRepeats := Repeats(openings), Repeats(closings)
	for index, shape := range season {
		closingStart := shape.duration() - closingLength
		creditsStart := shape.lead + introLength + shape.story
		for _, check := range []struct {
			what       string
			repeat     Repeat
			from       time.Duration
			start, end time.Duration
		}{
			{"intro", introRepeats[index], 0, shape.lead, shape.lead + introLength},
			{"credits", creditsRepeats[index], closingStart, creditsStart, creditsStart + creditsLength},
		} {
			start, end := check.repeat.Times()
			start, end = check.from+start, check.from+end
			t.Logf("episode %d's %s: found %v to %v, is %v to %v", index, check.what, start, end, check.start,
				check.end)
			if check.repeat.HeardIn != len(season)-1 || (start-check.start).Abs() > tolerance ||
				(end-check.end).Abs() > tolerance {
				t.Errorf("episode %d's %s: %v to %v heard in %d; want %v to %v, to within %v, heard in %d",
					index, check.what, start, end, check.repeat.HeardIn, check.start, check.end, tolerance,
					len(season)-1)
			}
		}
	}
}
