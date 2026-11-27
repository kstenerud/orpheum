// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// sheetArgumentsByRecipe is a hash of the FFmpeg arguments the node builds for the sheets of a fixed
// set of commands at each nodeproto.TrickplayRecipe (D536). A version is added here with each raise
// of it, and never changed after.
var sheetArgumentsByRecipe = map[int]string{
	// 1 is the first recipe (trickplay.md §3).
	1: "df3c59ab3a97ca176d3e30827869fa24df25334d0ab76bb51f2570cef2059e80",
}

// TestTrickplayRecipe_IsRaisedWithAnyChangeToItsArguments holds that the FFmpeg arguments of a run of
// sheets are the ones nodeproto.TrickplayRecipe was raised for (D536): a change to how the pictures
// are chosen, scaled, laid or encoded without raising it, which would serve sheets made before as
// this recipe's, or a raise without its arguments recorded, fails.
func TestTrickplayRecipe_IsRaisedWithAnyChangeToItsArguments(t *testing.T) {
	t.Parallel()
	var lines []string
	for _, run := range []struct {
		command nodeproto.MakeTrickplay
		first   int
	}{
		{nodeproto.MakeTrickplay{
			IntervalMs: 10000, PictureWidth: 320, SheetColumns: 10, SheetRows: 10, WebPQuality: 60,
		}, 0},
		{nodeproto.MakeTrickplay{
			VideoStreamIndex: 2, IntervalMs: 2500, PictureWidth: 240, SheetColumns: 5, SheetRows: 4, WebPQuality: 75,
		}, 3},
	} {
		arguments := makeArguments(execx.Film{Path: "/films/a.mkv"}.Input(), run.command, run.first)
		lines = append(lines, strings.Join(arguments, "\x00"))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	got := hex.EncodeToString(sum[:])
	if want, recorded := sheetArgumentsByRecipe[nodeproto.TrickplayRecipe]; !recorded || want != got {
		t.Errorf("a trickplay run's arguments are %s; TrickplayRecipe %d records %q. Changing how sheets are "+
			"made raises nodeproto.TrickplayRecipe, and the new version's arguments are recorded in "+
			"sheetArgumentsByRecipe", got, nodeproto.TrickplayRecipe, want)
	}
}
