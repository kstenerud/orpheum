// SPDX-License-Identifier: AGPL-3.0-or-later

package ptr_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

func TestValueOrZero_ReadsNilAsZero(t *testing.T) {
	t.Parallel()
	text, number := "text", int64(7)
	if got := ptr.ValueOrZero(&text); got != "text" {
		t.Errorf("a string: %q", got)
	}
	if got := ptr.ValueOrZero[string](nil); got != "" {
		t.Errorf("no string: %q", got)
	}
	if got := ptr.ValueOrZero(&number); got != 7 {
		t.Errorf("a number: %d", got)
	}
	if got := ptr.ValueOrZero[int64](nil); got != 0 {
		t.Errorf("no number: %d", got)
	}
}

func TestNilIfZero_WritesZeroAsNil(t *testing.T) {
	t.Parallel()
	if got := ptr.NilIfZero("text"); got == nil || *got != "text" {
		t.Errorf("a string: %v", got)
	}
	if got := ptr.NilIfZero(""); got != nil {
		t.Errorf("no string: %q", *got)
	}
	if got := ptr.NilIfZero(int64(7)); got == nil || *got != 7 {
		t.Errorf("a number: %v", got)
	}
	if got := ptr.NilIfZero(int64(0)); got != nil {
		t.Errorf("no number: %d", *got)
	}
}

func TestEqual_ComparesWhatThePointersHold(t *testing.T) {
	t.Parallel()
	first, second, other, empty := "text", "text", "other", ""
	cases := []struct {
		name        string
		left, right *string
		isEqual     bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", &first, nil, false},
		{"the other nil", nil, &first, false},
		{"nil and an empty string", nil, &empty, false},
		{"equal values at two addresses", &first, &second, true},
		{"different values", &first, &other, false},
	}
	for _, each := range cases {
		if got := ptr.Equal(each.left, each.right); got != each.isEqual {
			t.Errorf("%s: Equal = %v, want %v", each.name, got, each.isEqual)
		}
	}
}
