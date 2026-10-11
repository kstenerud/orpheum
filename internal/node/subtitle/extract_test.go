// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitle

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// readsOnly is whether a run's input is the one named, with only protocol allowed.
func readsOnly(arguments []string, protocol, input string) bool {
	whitelist := slices.Index(arguments, "-protocol_whitelist")
	named := slices.Index(arguments, "-i")
	return whitelist >= 0 && whitelist+1 < len(arguments) && arguments[whitelist+1] == protocol &&
		named >= 0 && named+1 < len(arguments) && arguments[named+1] == input &&
		slices.Index(arguments[named+1:], "-i") < 0
}

func TestExtract_FFmpegReadsOnlyTheFileOrThePipeItIsGiven(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "the text"})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	extractor := NewExtractor(runner, 20<<20)
	if _, err := extractor.ExtractTrack(t.Context(), "/media/film.mkv", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := extractor.ConvertText(t.Context(), []byte("the sidecar")); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls(t)
	if len(calls) != 4 {
		t.Fatalf("%d runs, want a track and its WebVTT, a sidecar and its WebVTT", len(calls))
	}
	wants := []struct {
		protocol, input, stdin string
	}{
		{"file", "file:/media/film.mkv", ""},
		{"pipe", "pipe:0", "the text"},
		{"pipe", "pipe:0", "the sidecar"},
		{"pipe", "pipe:0", "the text"},
	}
	for index, call := range calls {
		want := wants[index]
		if !readsOnly(call.Arguments, want.protocol, want.input) || string(call.Stdin) != want.stdin {
			t.Errorf("run %d read %q with stdin %q; want only %s by %s, fed %q",
				index, call.Arguments, call.Stdin, want.input, want.protocol, want.stdin)
		}
	}
	if !slices.Contains(calls[0].Arguments, "0:3") {
		t.Errorf("the track's run %q does not map stream 3", calls[0].Arguments)
	}
}
