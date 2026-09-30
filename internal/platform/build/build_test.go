// SPDX-License-Identifier: GPL-3.0-or-later

package build

import (
	"runtime/debug"
	"testing"
)

func recorded(version string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/kstenerud/orpheum", Version: version}, Settings: settings,
	}
}

func TestFromBuildInfo_IsTheTagOrDevAndTheCommitAndWhetherTheTreeWasModified(t *testing.T) {
	t.Parallel()
	commit := debug.BuildSetting{Key: "vcs.revision", Value: "0f6c5cc1a2b3c4d5e6f708192a3b4c5d6e7f8091"}
	modified := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	when := debug.BuildSetting{Key: "vcs.time", Value: "2026-09-27T12:00:00Z"}
	for _, step := range []struct {
		name  string
		built *debug.BuildInfo
		want  string
	}{
		{"a tagged commit", recorded("v1.2.0", commit, when), "v1.2.0"},
		{"a tagged commit, modified", recorded("v1.2.0+dirty", commit, modified), "v1.2.0-modified"},
		{"an untagged commit", recorded("v0.0.0-20260927120000-0f6c5cc1a2b3", commit), "dev-0f6c5cc1a2b3"},
		{
			"after a tag", recorded("v1.2.1-0.20260927120000-0f6c5cc1a2b3+dirty", commit, modified),
			"dev-0f6c5cc1a2b3-modified",
		},
		{"no record of the tree", recorded("(devel)"), "dev"},
	} {
		if got := FromBuildInfo(step.built).String(); got != step.want {
			t.Errorf("%s: %q, want %q", step.name, got, step.want)
		}
	}
	if got := FromBuildInfo(recorded("v1.2.0", commit, when)).CommittedAt; got != 1_790_510_400_000 {
		t.Errorf("the commit's time = %d, want 2026-09-27 12:00 UTC in milliseconds", got)
	}
}

func TestIsSame_TrustsTheBinarysHashElseAnUnmodifiedCommit(t *testing.T) {
	t.Parallel()
	release := Provenance{Version: "v1.2.0", Commit: "abc"}
	if !release.IsSame(release) || release.IsSame(Provenance{Version: "v1.1.0", Commit: "def"}) {
		t.Error("an unmodified build is not told apart from another by its version and commit")
	}
	modified := Provenance{Version: devVersion, Commit: "abc", HasUncommittedChanges: true}
	if modified.IsSame(modified) {
		t.Error("two modified builds of one commit are called the same, though they may differ")
	}
	if !modified.WithBinarySha256("h1").IsSame(modified.WithBinarySha256("h1")) ||
		release.WithBinarySha256("h1").IsSame(release.WithBinarySha256("h2")) {
		t.Error("the binaries' hashes do not decide when both are known")
	}
}
