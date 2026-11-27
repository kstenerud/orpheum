// SPDX-License-Identifier: AGPL-3.0-or-later

package speedtrial

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/pkg/atomics"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// trialRecipe is the trials' own recipe: the kinds, their clips and the qualities tried. Raising it
// runs every trial again on every node, as a changed Key does.
const trialRecipe = 1

// keptName is the file the results are kept in, in the node's persistent directory of its own.
const keptName = "speed-trials.json"

const keptMode = 0o600

// Key is what a node's results hold for: the same FFmpeg, encoder and quality recipe make a film at
// the same speed, so a restart with the same key runs no trial, and a changed one runs them all.
type Key struct {
	FFmpegVersion string         `json:"ffmpegVersion"`
	FFmpegSha256  string         `json:"ffmpegSha256"`
	Encoder       encode.Encoder `json:"encoder"`
	QualityRecipe int            `json:"qualityRecipe"`
	TrialRecipe   int            `json:"trialRecipe"`
}

// kept is the results file: the trials ended so far under Key, and whether all have.
type kept struct {
	Key        Key                    `json:"key"`
	Trials     []nodeproto.SpeedTrial `json:"trials"`
	IsComplete bool                   `json:"isComplete"`
}

// readKept reads the results kept in directory; none, or ones of another key or unreadable, are an
// empty start under key.
func readKept(directory string, key Key) (kept, error) {
	contents, err := os.ReadFile(filepath.Join(directory, keptName)) //nolint:gosec // G304: the node's own file
	if errors.Is(err, fs.ErrNotExist) {
		return kept{Key: key}, nil
	}
	if err != nil {
		return kept{}, fmt.Errorf("read the kept speed trials: %w", err)
	}
	var found kept
	if err := strictjson.Unmarshal(contents, &found); err != nil || found.Key != key {
		return kept{Key: key}, nil //nolint:nilerr // A file of another shape is made again.
	}
	return found, nil
}

// writeKept keeps results in directory, written whole (atomics.Replace), so a node stopped mid-write
// keeps the results it had.
func writeKept(directory string, results kept) (err error) {
	contents, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("encode the speed trials: %w", err)
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("make the speed trials' directory: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("keep the speed trials: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err = atomics.Replace(root, keptName, contents, keptMode); err != nil {
		return fmt.Errorf("keep the speed trials: %w", err)
	}
	return nil
}
