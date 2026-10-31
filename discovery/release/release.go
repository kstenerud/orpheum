// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// The paths the signed release record and the publisher's root chain are served at, under each
// location, each beside its signature at the same path and ".sig" (the root key audit's §3.1).
const (
	RecordPath              = "/.well-known/orpheum/release"
	PublisherRootPathPrefix = "/.well-known/orpheum/publisher-root/"
	PublisherRootLatestPath = PublisherRootPathPrefix + "latest"
)

// PublisherRootPath is where the publisher's root statement of version is served.
func PublisherRootPath(version int64) string {
	return PublisherRootPathPrefix + strconv.FormatInt(version, 10)
}

// recordLasts is how long a record is current after it is issued: the owner signs it again at each
// release, or monthly (the audit's F9).
const recordLasts = 30 * 24 * time.Hour

// Record is the release record (the audit's §3.1; D393 §2), signed by a key of the publisher's root:
// the publisher's id, the root version it was signed under and its sequence, which only rise; when it
// was issued and until when it is current; the latest release; every release's image digest, which
// never changes once signed; the fixes each older release lacks; where an image may be pulled; and
// where the record is served.
type Record struct {
	ID          string
	RootVersion int64
	Sequence    int64
	Issued      time.Time
	Expires     time.Time
	Latest      string
	// Images are each release's image digest, the multi-architecture index's, by its tag.
	Images map[string]string
	// Pending are the security fixes each older release lacks, by its tag.
	Pending   map[string]Fix
	Pulls     []string
	Locations []string
}

// Fix is a security fix a version lacks: the first release that has it, and what it fixes, in a
// sentence for the admin.
type Fix struct {
	FixedIn, Summary string
}

// Told is what the record tells a core of its own version.
type Told struct {
	// Latest is the newest release, only when it is newer than the core's version, Digest its image's
	// and Location where it may be pulled from.
	Latest, Digest, Location string
	// Security is the security fix the version lacks, nil when none is pending.
	Security *Fix
}

// recordSchema is the release record's lines, format 1.
func recordSchema() trust.Schema {
	return trust.Schema{Type: "release", Format: 1, Fields: []trust.Field{
		{Name: "id", IsRequired: true},
		{Name: "root-version", IsRequired: true},
		{Name: "sequence", IsRequired: true},
		{Name: "issued", IsRequired: true},
		{Name: "expires", IsRequired: true},
		{Name: "latest", IsRequired: true},
		{Name: "image", IsRepeated: true, IsRequired: true},
		{Name: "pending", IsRepeated: true},
		{Name: "pull", IsRepeated: true},
		{Name: "location", IsRepeated: true},
	}}
}

// digestPattern is an image's digest, as a registry names it.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Errors Released, Parse and Open return.
var (
	ErrNotARelease     = errors.New("not a release's tag: v<major>.<minor>.<patch>, perhaps with -<pre-release>")
	ErrOlder           = errors.New("older than the record's latest release")
	ErrNotADigest      = errors.New("not an image's digest: sha256: and 64 hexadecimal digits")
	ErrDigestChanged   = errors.New("a released version's digest never changes: a rebuilt release takes a new version")
	ErrNotARecord      = errors.New("not a release record")
	ErrNotThePublisher = errors.New("the record is not the publisher's")
	ErrAheadOfChain    = errors.New("the record names a root version the publisher's chain held has not reached")
)

// Parse reads a record's payload, checking its form and that its latest has an image.
func Parse(payload []byte) (Record, error) {
	lines, err := recordSchema().Parse(payload)
	if err != nil {
		return Record{}, err
	}
	record := Record{
		ID: lines.One("id"), Latest: lines.One("latest"), Images: map[string]string{}, Pending: map[string]Fix{},
		Pulls: lines["pull"], Locations: lines["location"],
	}
	if record.RootVersion, err = trust.ParseCount(lines.One("root-version")); err != nil {
		return Record{}, err
	}
	if record.Sequence, err = trust.ParseCount(lines.One("sequence")); err != nil {
		return Record{}, err
	}
	if record.Issued, err = trust.ParseTime(lines.One("issued")); err != nil {
		return Record{}, err
	}
	if record.Expires, err = trust.ParseTime(lines.One("expires")); err != nil {
		return Record{}, err
	}
	for _, line := range lines["image"] {
		tag, digest, _ := strings.Cut(line, " ")
		if _, isRelease := parse(tag); !isRelease || !digestPattern.MatchString(digest) || record.Images[tag] != "" {
			return Record{}, fmt.Errorf("%w: image %q", ErrNotARecord, line)
		}
		record.Images[tag] = digest
	}
	for _, line := range lines["pending"] {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 {
			return Record{}, fmt.Errorf("%w: pending %q", ErrNotARecord, line)
		}
		record.Pending[fields[0]] = Fix{FixedIn: fields[1], Summary: fields[2]}
	}
	if record.Images[record.Latest] == "" {
		return Record{}, fmt.Errorf("%w: the latest, %s, has no image", ErrNotARecord, record.Latest)
	}
	return record, nil
}

// Payload is the record written as its lines, which a key then signs: images and fixes newest first.
func (record Record) Payload() ([]byte, error) {
	lines := trust.Lines{
		"id": {record.ID}, "root-version": {strconv.FormatInt(record.RootVersion, 10)},
		"sequence": {strconv.FormatInt(record.Sequence, 10)}, "issued": {trust.TimeText(record.Issued)},
		"expires": {trust.TimeText(record.Expires)}, "latest": {record.Latest}, "pull": record.Pulls,
		"location": record.Locations,
	}
	for _, tag := range newestFirst(slices.Collect(maps.Keys(record.Images))) {
		lines["image"] = append(lines["image"], tag+" "+record.Images[tag])
	}
	for _, tag := range newestFirst(slices.Collect(maps.Keys(record.Pending))) {
		fix := record.Pending[tag]
		lines["pending"] = append(lines["pending"], tag+" "+fix.FixedIn+" "+fix.Summary)
	}
	return recordSchema().Write(lines)
}

// newestFirst are tags in order, the newest release first.
func newestFirst(tags []string) []string {
	slices.SortFunc(tags, func(one, other string) int {
		order, _ := Compare(other, one)
		return order
	})
	return tags
}

// Open is the record signed holds once it is checked (§3.5): a signature of the publisher's chain's
// current set's in the release namespace, its id the chain's and its root version one the chain has
// reached. The core keeps the floors and the digests it has seen.
func Open(signed trust.Signed, publisher trust.Chain) (Record, error) {
	if publisher.Kind() != trust.ChainPublisher {
		return Record{}, ErrNotThePublisher
	}
	if err := publisher.Verify(trust.NamespaceRelease, signed); err != nil {
		return Record{}, err
	}
	record, err := Parse([]byte(signed.Payload))
	if err != nil {
		return Record{}, err
	}
	switch {
	case record.ID != publisher.ID():
		return Record{}, fmt.Errorf("%w: %s, not %s", ErrNotThePublisher, record.ID, publisher.ID())
	case record.RootVersion > publisher.Version():
		return Record{}, fmt.Errorf("%w: %d, held %d", ErrAheadOfChain, record.RootVersion, publisher.Version())
	}
	return record, nil
}

// IsNewerThan is whether the record comes after other: a higher root version, or the same and a
// higher sequence.
func (record Record) IsNewerThan(other Record) bool {
	if record.RootVersion != other.RootVersion {
		return record.RootVersion > other.RootVersion
	}
	return record.Sequence > other.Sequence
}

// ChangedDigest is the first version for which the record names another digest than seen did, and
// whether there is one: equivocation, which a core refuses and tells its admins of (the audit's F11).
func (record Record) ChangedDigest(seen map[string]string) (string, bool) {
	for _, tag := range slices.Sorted(maps.Keys(record.Images)) {
		if known, isKnown := seen[tag]; isKnown && known != record.Images[tag] {
			return tag, true
		}
	}
	return "", false
}

// For is what the record tells the core running version (security-updates.md Part A): the latest, its
// digest and where to pull it when it is newer, and the version's pending fix. Nothing for a version
// that is no release's tag, a development build's.
func (record Record) For(version string) Told {
	var told Told
	if order, isRelease := Compare(record.Latest, version); isRelease && order > 0 {
		told.Latest, told.Digest = record.Latest, record.Images[record.Latest]
		if len(record.Pulls) > 0 {
			told.Location = record.Pulls[0]
		}
	}
	if fix, isPending := record.Pending[version]; isPending {
		told.Security = &fix
	}
	return told
}

// VersionOf is the release whose image digest is, and whether the record names one.
func (record Record) VersionOf(digest string) (string, bool) {
	for _, tag := range slices.Sorted(maps.Keys(record.Images)) {
		if record.Images[tag] == digest {
			return tag, true
		}
	}
	return "", false
}

// Released is the record once tag, its image at digest, is released at now, its sequence one more,
// current for a month: the latest, its digest named, and with securitySummary, what the release
// fixes, every older release that has no fix pending marked as needing this one. A version already
// named keeps its digest: naming another is refused (the audit's F11). Pulls, when given, are where
// an image may be pulled, from this record on.
func (record Record) Released(tag, digest, securitySummary string, pulls []string, now time.Time) (Record, error) {
	if _, isRelease := parse(tag); !isRelease {
		return record, fmt.Errorf("%q: %w", tag, ErrNotARelease)
	}
	if !digestPattern.MatchString(digest) {
		return record, fmt.Errorf("%q: %w", digest, ErrNotADigest)
	}
	if known, isKnown := record.Images[tag]; isKnown && known != digest {
		return record, fmt.Errorf("%s: %w", tag, ErrDigestChanged)
	}
	if order, isRelease := Compare(tag, record.Latest); isRelease && order < 0 {
		return record, fmt.Errorf("%s is %w, %s", tag, ErrOlder, record.Latest)
	}
	next := record
	next.Images, next.Pending = maps.Clone(record.Images), maps.Clone(record.Pending)
	if next.Images == nil {
		next.Images, next.Pending = map[string]string{}, map[string]Fix{}
	}
	if securitySummary != "" {
		for older := range next.Images {
			if _, isPending := next.Pending[older]; !isPending && older != tag {
				next.Pending[older] = Fix{FixedIn: tag, Summary: securitySummary}
			}
		}
	}
	next.Images[tag], next.Latest = digest, tag
	if len(pulls) > 0 {
		next.Pulls = slices.Clone(pulls)
	}
	next.Sequence++
	next.Issued, next.Expires = now.UTC().Truncate(time.Second), now.UTC().Truncate(time.Second).Add(recordLasts)
	return next, nil
}

// Read is the unsigned record path holds, the release workflow's own copy; an absent file is an
// empty record.
func Read(path string) (Record, error) {
	contents, err := os.ReadFile(path) //nolint:gosec // the path is the workflow's configured one
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("read the release record: %w", err)
	}
	record, err := Parse(contents)
	if err != nil {
		return Record{}, fmt.Errorf("read the release record %s: %w", path, err)
	}
	return record, nil
}

// Write writes record to path, readable by all, as the exact bytes a root key then signs: it holds
// no secret.
func Write(path string, record Record) error {
	payload, err := record.Payload()
	if err != nil {
		return fmt.Errorf("write the release record: %w", err)
	}
	if err = os.WriteFile(path, payload, 0o644); err != nil { //nolint:gosec // readable by all, as said
		return fmt.Errorf("write the release record: %w", err)
	}
	return nil
}
