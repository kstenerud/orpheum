// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// maxOutputBytes bounds what ffprobe may write for one file. Real output is tens of kilobytes; a
// file that makes ffprobe say more is hostile or broken, and is refused.
const maxOutputBytes = 16 << 20

// Prober runs ffprobe on one file at a time, through execx.
type Prober struct {
	runner  *execx.Runner
	timeout time.Duration
}

// NewProber returns a prober that gives each file at most timeout.
func NewProber(runner *execx.Runner, timeout time.Duration) *Prober {
	return &Prober{runner: runner, timeout: timeout}
}

// packetsRead is how many of the file's first packets, of every stream together, ffprobe lists, by
// which the node judges whether a picture's display times need rebuilding (needsRetiming). They are
// read already, as ffprobe finds the file's streams, so listing them costs no further read.
const packetsRead = 200

// arguments are ffprobe's, for film, read as its Input names it: the container, its streams and
// chapters, and the times of its first packetsRead packets.
func arguments(film execx.Film) []string {
	return append([]string{
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters",
		"-show_entries", "packet=stream_index,pts,dts", "-read_intervals", "%+#" + strconv.Itoa(packetsRead),
	}, film.Input()...)
}

// Probe returns what is inside film, a file at an absolute path: read by that path, or streamed
// through the node (D330).
func (prober *Prober) Probe(ctx context.Context, film execx.Film) (nodeproto.Probe, error) {
	if !filepath.IsAbs(film.Path) {
		return nodeproto.Probe{}, fmt.Errorf("probe needs an absolute path, not %q", film.Path)
	}
	ctx, cancel := context.WithTimeout(ctx, prober.timeout)
	defer cancel()
	output, err := prober.runner.RunOnFilm(ctx, execx.FFprobe, film, arguments(film), maxOutputBytes)
	if err != nil {
		return nodeproto.Probe{}, fmt.Errorf("probe %s: %w", film.Path, err)
	}
	if film.Disc != nil {
		return ParseDisc(output, *film.Disc)
	}
	return Parse(output)
}

// Parse turns ffprobe's JSON into the bounded probe record.
func Parse(output []byte) (nodeproto.Probe, error) {
	var raw rawOutput
	if err := json.Unmarshal(output, &raw); err != nil {
		return nodeproto.Probe{}, fmt.Errorf("ffprobe's output is not the JSON it promised: %w", err)
	}
	return raw.toProbe()
}
