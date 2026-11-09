// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck_RefusesADocumentPastItsBound(t *testing.T) {
	t.Parallel()
	bounds := defaultLimits()
	bounds.documentBytes = 10
	if err := check([]byte(`[1,2,3,45]`), bounds, nil); err != nil {
		t.Errorf("at the bound: %v", err)
	}
	err := check([]byte(`[1,2,3,456]`), bounds, nil)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "past the limit of 10") {
		t.Errorf("past the bound: %v; want ErrRefused", err)
	}
	bounds.elements = 3
	bounds.documentBytes = MaxDocumentBytes
	err = check([]byte(`{"a":1,"b":2,"c":3,"d":4}`), bounds, nil)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "more than 3 elements") {
		t.Errorf("an object of more pairs than the bound: %v; want ErrRefused", err)
	}
}
