// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import "testing"

// groupAnswering is a provider whose every episode group is parts; nothing else of it is asked.
type groupAnswering struct {
	Provider
	parts []EpisodeGroupPart
}

func (provider groupAnswering) ParseEpisodeGroup([]byte) ([]EpisodeGroupPart, error) {
	return provider.parts, nil
}

// Principles §22, D211: the episode a group's entry carries at its aired place is clean where it
// enters, as the entry's own id is: it names a file and is shown with no season listing (§3.3 of
// episode-orderings.md).
func TestCleaningProvider_AGroupEntrysAiredEpisodeIsClean(t *testing.T) {
	t.Parallel()
	// A zero-width space, a right-to-left override and a right-to-left isolate, as UTF-8 bytes.
	const zeroWidth, override, isolate = "\xe2\x80\x8b", "\xe2\x80\xae", "\xe2\x81\xa7"
	hostile := groupAnswering{parts: []EpisodeGroupPart{{Name: "Part\x00 One", Episodes: []EpisodeGroupEntry{{
		EpisodeID: "41" + zeroWidth,
		Aired: Episode{
			ID: "41" + zeroWidth, Season: 1, Number: 2, Name: "The" + override + "End\t\t  Begins",
			Overview: "One." + zeroWidth + "\n\n\n\nTwo.", StillRef: "/still" + isolate + ".jpg",
		},
	}}}}}
	parts, err := cleaningProvider{Provider: hostile}.ParseEpisodeGroup(nil)
	if err != nil || len(parts) != 1 || len(parts[0].Episodes) != 1 {
		t.Fatalf("parsed %+v, %v; want the one part's one entry", parts, err)
	}
	want := Episode{
		ID: "41", Season: 1, Number: 2, Name: "TheEnd Begins", Overview: "One.\n\nTwo.", StillRef: "/still.jpg",
	}
	if aired := parts[0].Episodes[0].Aired; aired != want {
		t.Errorf("the entry's aired episode %+v; want it clean, %+v", aired, want)
	}
}
