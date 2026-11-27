// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"path/filepath"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/speedtrial"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// speedTrialClipsDir is where a node instance keeps its speed trials' clips: in its cache, since they
// are made again when gone.
func (instance Instance) speedTrialClipsDir() string {
	return filepath.Join(instance.CacheDir, "speed-trial-clips")
}

// speedTrialsDir is where a node instance keeps its speed trials' results: persistent, so a restart
// with the same FFmpeg, encoder and recipe runs none again.
func (instance Instance) speedTrialsDir() string {
	return filepath.Join(instance.PersistentDir, "speed-trials")
}

// speedTrialsOf are the node's start-up speed trials (internal/node/speedtrial) on a node that makes
// chunks, keyed by its FFmpeg, as its hello names it, its chosen encoder and the quality recipe; none
// on a node whose prepared directory did not open, which makes no chunk to choose a quality of.
func speedTrialsOf(
	config Config, runner *execx.Runner, hello nodeproto.Hello, encoder encode.Encoder,
	directory preparedDirectory, logger *slog.Logger,
) *speedtrial.Trials {
	if directory.root == nil {
		return nil
	}
	return speedtrial.New(speedtrial.Config{
		Runner: runner, Encoder: encoder,
		Key: speedtrial.Key{
			FFmpegVersion: hello.FFmpegVersion, FFmpegSha256: hello.FFmpegSha256, Encoder: encoder,
			QualityRecipe: nodeproto.QualityRecipe,
		},
		ClipsDir: config.Instance.speedTrialClipsDir(), KeptDir: config.Instance.speedTrialsDir(),
		Clock: clock.System{}, Logger: logger,
	})
}
