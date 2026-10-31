// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"regexp"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Record is the release record, as its file holds it.
type Record struct {
	// Latest is the newest release's tag, "v0.3.2", and Digest its image's, "sha256:…".
	Latest string `json:"latest"`
	Digest string `json:"digest"`
	// Versions are the older releases, by their tags, each with whether it has a security fix pending.
	Versions map[string]Note `json:"versions"`
}

// Note is what the record says of one older release.
type Note struct {
	// Security is whether the release has a security fix pending; Summary says what it fixes, for the
	// admin, and FixedIn is the first release that has it.
	Security bool   `json:"security"`
	Summary  string `json:"summary,omitempty"`
	FixedIn  string `json:"fixedIn,omitempty"`
}

// For is what the record tells the core running version (security-updates.md Part A): the latest
// and its digest when it is newer, and the version's pending fix. Nothing for a version that is no
// release's tag, a development build's.
func (record Record) For(version string) discoveryproto.ReleaseResponse {
	var told discoveryproto.ReleaseResponse
	if order, isRelease := Compare(record.Latest, version); isRelease && order > 0 {
		told.Latest, told.Digest = record.Latest, record.Digest
	}
	if note, isNamed := record.Versions[version]; isNamed && note.Security {
		told.Security = &discoveryproto.SecurityFix{Summary: note.Summary, FixedIn: note.FixedIn}
	}
	return told
}

// digestPattern is an image's digest, as a registry names it.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Errors Released returns.
var (
	ErrNotARelease = errors.New("not a release's tag: v<major>.<minor>.<patch>, perhaps with -<pre-release>")
	ErrOlder       = errors.New("older than the record's latest release")
	ErrNotADigest  = errors.New("not an image's digest: sha256: and 64 hexadecimal digits")
)

// Released is the record once tag, its image at digest, is released: the latest, the release before
// it named among the older ones, and with securitySummary, what the release fixes, every older
// release that has no fix pending marked as needing this one. A release already pending keeps its
// first fix, which the latest has too. The latest released again takes its digest anew, as a re-run
// of the workflow does.
func (record Record) Released(tag, digest, securitySummary string) (Record, error) {
	if _, isRelease := parse(tag); !isRelease {
		return record, fmt.Errorf("%q: %w", tag, ErrNotARelease)
	}
	if !digestPattern.MatchString(digest) {
		return record, fmt.Errorf("%q: %w", digest, ErrNotADigest)
	}
	if order, isRelease := Compare(tag, record.Latest); isRelease && order < 0 {
		return record, fmt.Errorf("%s is %w, %s", tag, ErrOlder, record.Latest)
	}
	versions := make(map[string]Note, len(record.Versions)+1)
	maps.Copy(versions, record.Versions)
	if _, isNamed := versions[record.Latest]; !isNamed && record.Latest != "" && record.Latest != tag {
		versions[record.Latest] = Note{}
	}
	if securitySummary != "" {
		for older, note := range versions {
			if !note.Security {
				versions[older] = Note{Security: true, Summary: securitySummary, FixedIn: tag}
			}
		}
	}
	return Record{Latest: tag, Digest: digest, Versions: versions}, nil
}

// Read is the record path holds; an absent file is an empty record, nothing known.
func Read(path string) (Record, error) {
	contents, err := os.ReadFile(path) //nolint:gosec // the path is the deployment's configured one
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("read the release record: %w", err)
	}
	var record Record
	if err = json.Unmarshal(contents, &record); err != nil {
		return Record{}, fmt.Errorf("read the release record %s: %w", path, err)
	}
	return record, nil
}

// Write writes record to path, readable by all: it holds no secret. The release workflow writes it
// where it runs, and the file is then copied into the discovery's deployment, whose copy puts it in
// place whole; a discovery that reads one half there tells its cores nothing that once (File.Answer).
func Write(path string, record Record) error {
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the release record: %w", err)
	}
	if err = os.WriteFile(path, append(contents, '\n'), 0o644); err != nil { //nolint:gosec // readable by all, as said
		return fmt.Errorf("write the release record: %w", err)
	}
	return nil
}

// File is the release record a discovery answers its cores from, read afresh at each asking: a core
// asks once a day, and a new release is answered from the moment its record is written.
type File struct {
	// Path is the record's file in the discovery's deployment; empty knows nothing.
	Path   string
	Logger *slog.Logger
}

// Answer is what the record tells the core running version (Record.For). A file absent, unreadable
// or not a record is nothing known: the core is told nothing, and the discovery's log why.
func (file File) Answer(ctx context.Context, version string) discoveryproto.ReleaseResponse {
	if file.Path == "" {
		return discoveryproto.ReleaseResponse{}
	}
	record, err := Read(file.Path)
	if err != nil {
		if file.Logger != nil {
			file.Logger.WarnContext(ctx, "discovery: the release record could not be read; telling the core nothing",
				"err", err)
		}
		return discoveryproto.ReleaseResponse{}
	}
	return record.For(version)
}
