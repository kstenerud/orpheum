// SPDX-License-Identifier: GPL-3.0-or-later

package textindex

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFold_OneNormaliserForEveryScript(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"Sølvi":      "solvi",
		"GØGGS":      "goggs",
		"Fièvre":     "fievre",
		"Straße":     "strasse",
		"Æon Flux":   "aeon flux",
		"Łódź":       "lodz",
		"十字路":   "十字路",
		"狂った一頁":      "狂った一頁",
		"Ｆｕｌｌ ｗｉｄｔｈ": "full width",
	} {
		if got := Fold(input); got != want {
			t.Errorf("Fold(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSameWork_NamesThatDifferOnlyInWhatFilesAndProvidersWriteTheirOwnWayAreOne(t *testing.T) {
	t.Parallel()
	for _, same := range [][]string{
		{"Lantern's Agents of L.A.N.T.E.R.N.", "Lanterns Agents of SHIELD", "MARVEL’S AGENTS OF LANTERN"},
		{"Moth-Lantern: Homecoming", "Moth Lantern Homecoming", "moth.lantern.homecoming"},
		{"Fièvre", "FIEVRE", "Ame\u0301lie"},
		{"Sølvi", "Solvi"},
		{"Ｆｕｌｌ ｗｉｄｔｈ", "Full Width"},
		{"十字路", "千と千尋の 神隠し"},
		{"नमस्ते", "नमस्ते!"},
	} {
		for _, name := range same[1:] {
			if SameWork(name) != SameWork(same[0]) {
				t.Errorf("SameWork(%q) = %q, SameWork(%q) = %q; want one work", name, SameWork(name), same[0],
					SameWork(same[0]))
			}
		}
		if SameWork(same[0]) == "" {
			t.Errorf("SameWork(%q) is empty; every script's letters are kept", same[0])
		}
	}
	for _, different := range [][2]string{{"Metropolis", "Metropolis 2"}, {"Wing", "Wings"}, {"十字", "千と千"}} {
		if SameWork(different[0]) == SameWork(different[1]) {
			t.Errorf("%q and %q fold alike; want two works", different[0], different[1])
		}
	}
}

func TestSorter_FollowsTheLocaleAndNumbers(t *testing.T) {
	t.Parallel()
	ordered := func(locale string, names ...string) []string {
		t.Helper()
		sorter, err := NewSorter(locale)
		if err != nil {
			t.Fatal(err)
		}
		sorted := slices.Clone(names)
		slices.SortFunc(sorted, func(a, b string) int { return bytes.Compare(sorter.Key(a), sorter.Key(b)) })
		return sorted
	}
	if got := ordered("en", "Episode 10", "Episode 2", "episode 1"); !slices.Equal(got,
		[]string{"episode 1", "Episode 2", "Episode 10"}) {
		t.Errorf("en: %v; numbers sort as numbers, case aside", got)
	}
	// Swedish puts Å, Ä and Ö after Z; English among the A and O.
	if got := ordered("sv", "Öga", "Zebra", "Apa"); !slices.Equal(got, []string{"Apa", "Zebra", "Öga"}) {
		t.Errorf("sv: %v", got)
	}
	if got := ordered("en", "Öga", "Zebra", "Apa"); !slices.Equal(got, []string{"Apa", "Öga", "Zebra"}) {
		t.Errorf("en: %v", got)
	}
	if _, err := NewSorter("not a locale!"); err == nil {
		t.Error("NewSorter accepted a malformed locale")
	}
}

func TestPlanQuery_RoutesByLengthAndScript(t *testing.T) {
	t.Parallel()
	for query, want := range map[string]Plan{
		"":      {},
		"  ":    {},
		"up":    {Route: RouteSubstring, IndexQuery: "up"},
		"5%":    {Route: RouteSubstring, IndexQuery: "5%"},
		"Sølvi": {Route: RouteWords, IndexQuery: `("solvi" OR "solvi"*)`},
		"heat NEAR 95": {
			Route: RouteWords, IndexQuery: `("heat" OR "heat"*) AND ("near" OR "near"*) AND ("95" OR "95"*)`,
		},
		`the "quote"`: {Route: RouteWords, IndexQuery: `("the" OR "the"*) AND ("quote" OR "quote"*)`},
		"十字":        {Route: RouteTrigram, IndexQuery: `"十字"`},
		"千尋":          {Route: RouteSubstring, IndexQuery: "千尋"},
	} {
		if got := PlanQuery(query); got != want {
			t.Errorf("PlanQuery(%q) = %+v, want %+v", query, got, want)
		}
	}
}

func TestClean_NothingShownCanLieOrSpill(t *testing.T) {
	t.Parallel()
	zalgo := "Z" + strings.Repeat("\u0301", 40) + "algo"
	for input, want := range map[string]string{
		"evil\u202Evkm.exe":           "evilvkm.exe", // a right-to-left override
		"a\u2066b\u2069c":             "abc",         // isolates
		"in\u200Bvisible\uFEFF":       "invisible",   // zero width space, BOM
		"tab\tand\nnewline\r\x07bell": "tab and newline bell",
		"  spaced   out  ":            "spaced out",
		"c1\u0085control":             "c1 control",
		"bad \xff byte":               "bad � byte",
		zalgo:                         "Z\u0301\u0301\u0301\u0301algo",
		"Việt Nam":                    "Việt Nam", // marks a real script needs
		"👩\u200D💻 مرحبا\u200Fعالم": "👩\u200D💻 مرحبا\u200Fعالم", // ZWJ and RLM kept
	} {
		if got := Line(input); got != want {
			t.Errorf("Line(%q) = %q, want %q", input, got, want)
		}
	}
	if got := Prose("One.\n\n\n\nTwo.\r\nThree."); got != "One.\n\nTwo.\nThree." {
		t.Errorf("Prose = %q; paragraph breaks kept, at most two, and CRLF one break", got)
	}
	if got := Line(strings.Repeat("x", 600)); utf8.RuneCountInString(got) != MaxNameRunes {
		t.Errorf("a 600-rune name kept %d runes, want %d", utf8.RuneCountInString(got), MaxNameRunes)
	}
}

func TestPath_MarksWhatWasHiddenAndChangesNothingElse(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"/media/Films/Metropolis (1995)/Metropolis.mkv": "/media/Films/Metropolis (1995)/Metropolis.mkv",
		"/media/two  spaces.mkv":            "/media/two  spaces.mkv",
		"/media/evil\u202Evkm.exe":          "/media/evil�vkm.exe",
		"/media/line\nbreak.mkv":            "/media/line�break.mkv",
		"/media/bad\xffbyte.mkv":            "/media/bad�byte.mkv",
	} {
		if got := Path(input); got != want {
			t.Errorf("Path(%q) = %q, want %q", input, got, want)
		}
	}
}
