// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// WellKnownPath is where a discovery serves the signed release record, at its issuer and at each of
// its address document's locations, and where a record is published beside each release (D391 §6).
const WellKnownPath = "/.well-known/orpheum-release-record"

// Record is the release record, the payload a root key signs (trust.KindReleaseRecord).
type Record struct {
	// IssuedAt is when the record was written, Unix milliseconds: a newer one replaces an older.
	IssuedAt int64 `json:"issuedAt"`
	// Latest is the newest release's tag, "v0.3.2".
	Latest string `json:"latest"`
	// Locations are the image repositories a release may be pulled from, "registry.example/orpheum",
	// each a mirror: the digest, not the place, says it is the release (D391 §6).
	Locations []string `json:"locations"`
	// Releases are every release the record has seen, by its tag: its image's digest, and for an
	// older one whether it has a security fix pending.
	Releases map[string]Note `json:"releases"`
}

// Note is what the record says of one release.
type Note struct {
	// Digest is the release image's, "sha256:…": the multi-platform image's own, as a registry names
	// it after a push.
	Digest string `json:"digest"`
	// Security is whether the release has a security fix pending; Summary says what it fixes, for the
	// admin, and FixedIn is the first release that has it.
	Security bool   `json:"security,omitempty"`
	Summary  string `json:"summary,omitempty"`
	FixedIn  string `json:"fixedIn,omitempty"`
}

// Told is what the record tells a core of its own version.
type Told struct {
	// Latest is the newest release, only when it is newer than the core's version, Digest its image's
	// and Location where it may be pulled from.
	Latest, Digest, Location string
	// Security is the security fix the version lacks, nil when none is pending.
	Security *Fix
}

// Fix is a security fix a version lacks: what it fixes, in a sentence for the admin, and the first
// release that has it.
type Fix struct {
	Summary, FixedIn string
}

// For is what the record tells the core running version (security-updates.md Part A): the latest and
// its digest when it is newer, and the version's pending fix. Nothing for a version that is no
// release's tag, a development build's.
func (record Record) For(version string) Told {
	var told Told
	if order, isRelease := Compare(record.Latest, version); isRelease && order > 0 {
		told.Latest, told.Digest = record.Latest, record.Releases[record.Latest].Digest
		if len(record.Locations) > 0 {
			told.Location = record.Locations[0]
		}
	}
	if note, isNamed := record.Releases[version]; isNamed && note.Security {
		told.Security = &Fix{Summary: note.Summary, FixedIn: note.FixedIn}
	}
	return told
}

// VersionOf is the release whose image digest is, and whether the record names one.
func (record Record) VersionOf(digest string) (string, bool) {
	for _, tag := range slices.Sorted(maps.Keys(record.Releases)) {
		if record.Releases[tag].Digest == digest {
			return tag, true
		}
	}
	return "", false
}

// digestPattern is an image's digest, as a registry names it.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Errors Released and Open return.
var (
	ErrNotARelease = errors.New("not a release's tag: v<major>.<minor>.<patch>, perhaps with -<pre-release>")
	ErrOlder       = errors.New("older than the record's latest release")
	ErrNotADigest  = errors.New("not an image's digest: sha256: and 64 hexadecimal digits")
	ErrNotARecord  = errors.New("not a release record")
)

// Released is the record once tag, its image at digest, is released at now: the latest, named with
// its digest among the releases, and with securitySummary, what the release fixes, every older
// release that has no fix pending marked as needing this one. A release already pending keeps its
// first fix, which the latest has too. The latest released again takes its digest anew, as a re-run
// of the workflow does. Locations, when given, are where the image may be pulled from henceforth.
func (record Record) Released(tag, digest, securitySummary string, locations []string, now time.Time) (Record, error) {
	if _, isRelease := parse(tag); !isRelease {
		return record, fmt.Errorf("%q: %w", tag, ErrNotARelease)
	}
	if !digestPattern.MatchString(digest) {
		return record, fmt.Errorf("%q: %w", digest, ErrNotADigest)
	}
	if order, isRelease := Compare(tag, record.Latest); isRelease && order < 0 {
		return record, fmt.Errorf("%s is %w, %s", tag, ErrOlder, record.Latest)
	}
	releases := make(map[string]Note, len(record.Releases)+1)
	maps.Copy(releases, record.Releases)
	if securitySummary != "" {
		for older, note := range releases {
			if older != tag && !note.Security {
				releases[older] = Note{Digest: note.Digest, Security: true, Summary: securitySummary, FixedIn: tag}
			}
		}
	}
	releases[tag] = Note{Digest: digest}
	if len(locations) == 0 {
		locations = record.Locations
	}
	return Record{
		IssuedAt: now.UnixMilli(), Latest: tag, Locations: slices.Clone(locations), Releases: releases,
	}, nil
}

// Open is the record signed holds once its signature is a root key's of pinned (trust.Pinned.OpenRecord).
func Open(signed trust.Signed, pinned trust.Pinned) (Record, error) {
	payload, err := pinned.OpenRecord(signed)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err = json.Unmarshal(payload, &record); err != nil {
		return Record{}, fmt.Errorf("%w: %w", ErrNotARecord, err)
	}
	if record.Latest == "" || record.IssuedAt == 0 {
		return Record{}, fmt.Errorf("%w: it names no latest release or when it was written", ErrNotARecord)
	}
	return record, nil
}

// Read is the unsigned record path holds, the release workflow's own copy; an absent file is an
// empty record, nothing known.
func Read(path string) (Record, error) {
	contents, err := os.ReadFile(path) //nolint:gosec // the path is the workflow's configured one
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

// Write writes record to path, readable by all, as the exact bytes a root key then signs: it holds
// no secret.
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
