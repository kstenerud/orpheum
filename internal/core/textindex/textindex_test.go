// SPDX-License-Identifier: GPL-3.0-or-later

package textindex

import (
	"bytes"
	"slices"
	"testing"
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
		"":             {},
		"  ":           {},
		"up":           {Route: RouteShort, Match: "up"},
		"5%":           {Route: RouteShort, Match: "5%"},
		"Sølvi":        {Route: RouteWords, Match: `("solvi" OR "solvi"*)`},
		"heat NEAR 95": {Route: RouteWords, Match: `("heat" OR "heat"*) AND ("near" OR "near"*) AND ("95" OR "95"*)`},
		`the "quote"`:  {Route: RouteWords, Match: `("the" OR "the"*) AND ("quote" OR "quote"*)`},
		"十字":         {Route: RouteTrigram, Match: `"十字"`},
		"千尋":           {Route: RouteShort, Match: "千尋"},
	} {
		if got := PlanQuery(query); got != want {
			t.Errorf("PlanQuery(%q) = %+v, want %+v", query, got, want)
		}
	}
}
