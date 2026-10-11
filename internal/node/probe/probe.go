// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
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

// arguments are ffprobe's, for the file at an absolute path, read as execx.FileInput reads it.
func arguments(absolutePath string) []string {
	return append([]string{
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters",
	}, execx.FileInput(absolutePath)...)
}

// Probe returns what is inside the file at absolutePath.
func (prober *Prober) Probe(ctx context.Context, absolutePath string) (nodeproto.Probe, error) {
	if !filepath.IsAbs(absolutePath) {
		return nodeproto.Probe{}, fmt.Errorf("probe needs an absolute path, not %q", absolutePath)
	}
	ctx, cancel := context.WithTimeout(ctx, prober.timeout)
	defer cancel()
	output, err := prober.runner.Run(ctx, execx.FFprobe, confine.Access{Read: []string{absolutePath}},
		arguments(absolutePath), maxOutputBytes)
	if err != nil {
		return nodeproto.Probe{}, fmt.Errorf("probe %s: %w", absolutePath, err)
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
