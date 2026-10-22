// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A node streams its films only when its FFmpeg lists the ffstreamer protocol among its inputs, and
// says at start which way its runs read their films; the stock build reads them by path (D330).
func TestStreamsFilms_OnlyWhenTheNodesFFmpegHasTheProtocolAndSaysWhich(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name, listing, wantSaid string
		want                    bool
	}{
		{
			"the patched build", "Supported file protocols:\nInput:\n  ffstreamer\n  file\nOutput:\n  file\n",
			"through the node", true,
		},
		{
			"the stock build", "Supported file protocols:\nInput:\n  file\nOutput:\n  file\n",
			"by its path", false,
		},
	} {
		fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegStdout: example.listing})
		runner, err := execx.NewRunner(fake.Directory)
		if err != nil {
			t.Fatal(err)
		}
		var logged bytes.Buffer
		got := streamsFilms(t.Context(), runner, slog.New(slog.NewTextHandler(&logged, nil)))
		if got != example.want || !strings.Contains(logged.String(), example.wantSaid) {
			t.Errorf("%s: streams films %v, said %q; want %v, saying it reads films %s", example.name, got,
				logged.String(), example.want, example.wantSaid)
		}
	}
}
