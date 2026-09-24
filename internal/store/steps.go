// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

// migrationFiles are the schema steps, embedded so the binary carries every step it knows
// (migrations.md: every step is kept, forever).
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// raisesCompatibleDirective, on a line of its own, declares that a step drops, renames or
// reshapes, so an older binary can no longer use the file (migrations.md, Expand then contract).
const raisesCompatibleDirective = "-- orpheum:raises-compatible-version"

var stepFileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// Step is one migration: a SQL file, numbered from 1 with no gaps.
type Step struct {
	Version          int
	Name             string
	SQL              string
	RaisesCompatible bool
}

// Steps returns the embedded migration steps in order.
func Steps() ([]Step, error) {
	return readSteps(migrationFiles, "migrations")
}

// SchemaVersion is the schema this binary builds: the number of its last step.
func SchemaVersion() (int, error) {
	steps, err := Steps()
	if err != nil {
		return 0, err
	}
	return len(steps), nil
}

// readSteps reads every step file in directory, and refuses a misnamed file or a gap.
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
		match := stepFileName.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("migration step %q: want NNNN_what_it_does.sql", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("migration step %q: %w", entry.Name(), err)
		}
		if version != len(steps)+1 {
			return nil, fmt.Errorf("migration step %q: want step %04d next; steps are numbered with no gaps",
				entry.Name(), len(steps)+1)
		}
		contents, err := fs.ReadFile(files, directory+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration step %q: %w", entry.Name(), err)
		}
		text := string(contents)
		steps = append(steps, Step{
			Version:          version,
			Name:             entry.Name(),
			SQL:              text,
			RaisesCompatible: hasDirectiveLine(text, raisesCompatibleDirective),
		})
	}
	return steps, nil
}

func hasDirectiveLine(text, directive string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == directive {
			return true
		}
	}
	return false
}
