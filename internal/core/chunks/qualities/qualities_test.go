// SPDX-License-Identifier: GPL-3.0-or-later

package qualities

import (
	"errors"
	"slices"
	"testing"
)

func TestParse_ReadsEachQualityHighestFirst(t *testing.T) {
	t.Parallel()
	for text, want := range map[string][]Quality{
		"3000k@720p,2000k@540p,1200k@480p,700k@432p": {
			{Kbps: 3000, Height: 720}, {Kbps: 2000, Height: 540}, {Kbps: 1200, Height: 480}, {Kbps: 700, Height: 432},
		},
		"1200k@480p": {{Kbps: 1200, Height: 480}},
		// Two bit rates at one height are two qualities: the lower is no taller.
		"2000k@720p,1500k@720p": {{Kbps: 2000, Height: 720}, {Kbps: 1500, Height: 720}},
		// The bounds themselves.
		"50000k@2160p,100k@144p": {{Kbps: 50_000, Height: 2160}, {Kbps: 100, Height: 144}},
	} {
		got, err := Parse(text)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("Parse(%q) = %v, %v; want %v", text, got, err, want)
		}
		if !IsValid(text) {
			t.Errorf("IsValid(%q) = false; want the setting to take what Parse reads", text)
		}
	}
}

func TestParse_RefusesAListNotOfThatShape(t *testing.T) {
	t.Parallel()
	for what, text := range map[string]string{
		"nothing":                    "",
		"a trailing comma":           "3000k@720p,",
		"spaces":                     "3000k@720p, 2000k@540p",
		"no unit":                    "3000@720",
		"megabits":                   "3M@720p",
		"a bit rate under the floor": "99k@480p",
		"a bit rate over the top":    "50001k@720p",
		"a height under the floor":   "700k@142p",
		"a height over the top":      "9000k@2162p",
		"an odd height":              "700k@431p",
		"a leading zero":             "0700k@432p",
		"the same quality twice":     "1200k@480p,1200k@480p",
		"lowest first":               "700k@432p,3000k@720p",
		"a lower bit rate taller":    "3000k@540p,2000k@720p",
		"the same bit rate shorter":  "3000k@720p,3000k@540p",
	} {
		got, err := Parse(text)
		if !errors.Is(err, ErrNotQualities) || got != nil {
			t.Errorf("%s: Parse(%q) = %v, %v; want ErrNotQualities", what, text, got, err)
		}
		if IsValid(text) {
			t.Errorf("%s: IsValid(%q) = true; want the setting to refuse what Parse refuses", what, text)
		}
	}
}
