// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// probeShapeByVersion is the shape of the probe record at each ProbeVersion: a hash of every field
// of it, at any depth, with its type and wire tag. A version is added here with each raise of
// ProbeVersion, and never changed after.
var probeShapeByVersion = map[int]string{
	// 1 is the probe with a sound's first packet's stretch (D534 §1) and the version that read it
	// (D535 §1).
	1: "52a5ec9c3347197268de6650ab4f316d747e7228df39e44acce67d28513f60fd",
}

// TestProbeVersion_IsRaisedWithAnyChangeToTheProbe holds that the probe's shape is the one this
// ProbeVersion was raised for (D535 §1): a fact added, removed, retyped or renamed without raising
// it, which would leave the files an older probe read unread again, or a raise without its shape
// recorded, fails. A change to how a fact is read is the review's.
func TestProbeVersion_IsRaisedWithAnyChangeToTheProbe(t *testing.T) {
	t.Parallel()
	got := probeShape()
	if want, recorded := probeShapeByVersion[ProbeVersion]; !recorded || want != got {
		t.Errorf("the probe's shape is %s; probe version %d records %q. Changing the probe raises ProbeVersion, "+
			"and the new version's shape is recorded in probeShapeByVersion", got, ProbeVersion, want)
	}
}

// probeShape is a hash of every field of the probe record, at any depth.
func probeShape() string {
	fields := fieldsOf(reflect.TypeFor[Probe](), map[reflect.Type]bool{})
	slices.Sort(fields)
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return hex.EncodeToString(sum[:])
}
