// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

// migrationFiles are the schema steps, embedded so the binary carries every step it knows: a
// baseline and the step after it until migrations are live, and from then on every step, forever
// (D228; migrations.md).
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// raisesCompatibleDirective, on a line of its own, declares that a step drops, renames or
// reshapes, so an older binary can no longer use the file (migrations.md, Expand then contract).
const raisesCompatibleDirective = "-- orpheum:raises-compatible-version"

// baselineDirective, on a line of its own and followed by a checksum, declares that a step is the
// baseline (D228): it builds a new database at its version in one step, folding every step up to
// that version, the last of which had the checksum given.
const baselineDirective = "-- orpheum:baseline folds"

var (
	stepFileNamePattern = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)
	baselineLinePattern = regexp.MustCompile(`^` + baselineDirective + ` ([0-9a-f]{64})$`)
)

// Step is one migration: a SQL file. The first is 0001, or a baseline at the version of the last
// step it folds; the rest follow with no gaps.
type Step struct {
	Version                 int
	Name                    string
	SQL                     string
	RaisesCompatibleVersion bool
	// Checksum is the hex SHA-256 of the file's bytes, as schema_step records it.
	Checksum string
	// IsBaseline is a step that builds a new database at Version, and runs on no other.
	IsBaseline bool
	// FoldedChecksum is a baseline's record of the last step it folds: a database that reached
	// Version through that step recorded this checksum for it, and is as current as one the
	// baseline built.
	FoldedChecksum string
}

// Steps returns the embedded migration steps in order.
func Steps() ([]Step, error) {
	return readSteps(migrationFiles, "migrations")
}

// SchemaVersion is the schema this binary builds: the version of its last step.
func SchemaVersion() (int, error) {
	steps, err := Steps()
	if err != nil {
		return 0, err
	}
	return lastVersion(steps), nil
}

// lastVersion is the version steps bring a database to; 0 when there are none.
func lastVersion(steps []Step) int {
	if len(steps) == 0 {
		return 0
	}
	return steps[len(steps)-1].Version
}

// readSteps reads every step file in directory, and refuses a misnamed file, a gap, or a
// baseline anywhere but first.
func readSteps(files fs.FS, directory string) ([]Step, error) {
	entries, err := fs.ReadDir(files, directory)
	if err != nil {
		return nil, fmt.Errorf("read migration steps: %w", err)
	}
	var steps []Step
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		step, err := readStep(files, directory, entry.Name())
		if err != nil {
			return nil, err
		}
		switch {
		case step.IsBaseline && len(steps) > 0:
			return nil, fmt.Errorf("migration step %q: only the first step may be a baseline", step.Name)
		case len(steps) == 0 && !step.IsBaseline && step.Version != 1:
			return nil, fmt.Errorf("migration step %q: the first step is 0001 or a baseline", step.Name)
		case len(steps) > 0 && step.Version != lastVersion(steps)+1:
			return nil, fmt.Errorf("migration step %q: want step %04d next; steps are numbered with no gaps",
				step.Name, lastVersion(steps)+1)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// readStep reads one step file and its directives.
func readStep(files fs.FS, directory, name string) (Step, error) {
	match := stepFileNamePattern.FindStringSubmatch(name)
	if match == nil {
		return Step{}, fmt.Errorf("migration step %q: want NNNN_what_it_does.sql", name)
	}
	version, err := strconv.Atoi(match[1])
	if err != nil {
		return Step{}, fmt.Errorf("migration step %q: %w", name, err)
	}
	contents, err := fs.ReadFile(files, directory+"/"+name)
	if err != nil {
		return Step{}, fmt.Errorf("read migration step %q: %w", name, err)
	}
	text := string(contents)
	sum := sha256.Sum256(contents)
	step := Step{
		Checksum:                hex.EncodeToString(sum[:]),
		Version:                 version,
		Name:                    name,
		SQL:                     text,
		RaisesCompatibleVersion: hasDirectiveLine(text, raisesCompatibleDirective),
	}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, baselineDirective) {
			continue
		}
		folds := baselineLinePattern.FindStringSubmatch(line)
		if folds == nil {
			return Step{}, fmt.Errorf("migration step %q: want %q and the folded step's SHA-256",
				name, baselineDirective)
		}
		step.IsBaseline, step.FoldedChecksum = true, folds[1]
	}
	return step, nil
}

func hasDirectiveLine(text, directive string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == directive {
			return true
		}
	}
	return false
}
