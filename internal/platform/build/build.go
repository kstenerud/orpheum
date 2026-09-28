// SPDX-License-Identifier: GPL-3.0-or-later

package build

import (
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

// Info is what a binary was built from.
type Info struct {
	// Version is the release tag the build's commit carries, or "dev" when it carries none.
	Version string `json:"version"`
	// Commit is the full commit hash, empty when the build recorded none (a test binary).
	Commit string `json:"commit,omitempty"`
	// IsModified is whether the tree had uncommitted changes when it was built.
	IsModified bool `json:"isModified"`
	// CommittedAt is the commit's time, in Unix milliseconds; zero when unrecorded.
	CommittedAt int64 `json:"committedAt,omitempty"`
	// BinarySHA256 is the running binary's hash, which tells two builds apart when nothing else
	// can; empty when it could not be read. The caller fills it in (Hashed).
	BinarySHA256 string `json:"binarySha256,omitempty"`
}

// dev is the version of a build whose commit carries no release tag.
const dev = "dev"

// pseudoVersion is the version Go gives a build whose commit has no tag: v0.0.0-<time>-<commit>.
var pseudoVersion = regexp.MustCompile(`-(?:0\.)?\d{14}-[0-9a-f]{12}$`)

// This is what the running binary was built from.
func This() Info {
	recorded, isRecorded := debug.ReadBuildInfo()
	if !isRecorded {
		return Info{Version: dev}
	}
	return FromBuildInfo(recorded)
}

// FromBuildInfo reads a build's record: Go gives the main module the tag its commit carries,
// with +dirty on a modified tree, and records the commit, its time and the modification.
func FromBuildInfo(recorded *debug.BuildInfo) Info {
	info := Info{Version: dev}
	tag := strings.TrimSuffix(recorded.Main.Version, "+dirty")
	if tag != "" && tag != "(devel)" && !pseudoVersion.MatchString(tag) {
		info.Version = tag
	}
	for _, setting := range recorded.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.modified":
			info.IsModified = setting.Value == "true"
		case "vcs.time":
			if committed, err := time.Parse(time.RFC3339, setting.Value); err == nil {
				info.CommittedAt = committed.UnixMilli()
			}
		}
	}
	return info
}

// Hashed is info with the binary's hash.
func (info Info) Hashed(binarySHA256 string) Info {
	info.BinarySHA256 = binarySHA256
	return info
}

// String is the build in a word: the tag, or "dev-" and the short commit, and "-modified" for a
// tree with uncommitted changes: "v1.2.0", "dev-0f6c5cc1a2b3-modified".
func (info Info) String() string {
	text := info.Version
	if info.Version == dev && info.Commit != "" {
		text += "-" + info.Commit[:min(12, len(info.Commit))]
	}
	if info.IsModified {
		text += "-modified"
	}
	return text
}

// IsSame is whether two binaries are the same build: the same bytes when both know their hash,
// else the same version and commit, both unmodified. Two modified trees from one commit may
// differ, so they are not called the same.
func (info Info) IsSame(other Info) bool {
	if info.BinarySHA256 != "" && other.BinarySHA256 != "" {
		return info.BinarySHA256 == other.BinarySHA256
	}
	return info.Commit != "" && info.Commit == other.Commit && info.Version == other.Version &&
		!info.IsModified && !other.IsModified
}
