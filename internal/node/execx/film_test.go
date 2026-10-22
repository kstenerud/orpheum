// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// protocolsListing is the start of ffmpeg -protocols from the patched build, then its output
// protocols.
const protocolsListing = "Supported file protocols:\nInput:\n  async\n  bluray\n  fd\n  ffstreamer\n  file\n" +
	"Output:\n  file\n  pipe\n"

// A node streams its films only to an FFmpeg that lists ffstreamer among the protocols it reads: the
// stock build, which does not, reads each film by its path (D330), and so does one that names it only
// as an output.
func TestHasFFstreamer_IsTheProtocolAmongTheInputsFFmpegLists(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name, listing string
		want          bool
	}{
		{"the patched build", protocolsListing, true},
		{"the stock build", "Supported file protocols:\nInput:\n  async\n  file\nOutput:\n  file\n", false},
		{"an output only", "Supported file protocols:\nInput:\n  file\nOutput:\n  ffstreamer\n", false},
	} {
		runner := newRunner(t, testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegStdout: example.listing}))
		got, err := runner.HasFFstreamer(t.Context())
		if err != nil || got != example.want {
			t.Errorf("%s: %v, %v; want %v", example.name, got, err, example.want)
		}
	}
}
