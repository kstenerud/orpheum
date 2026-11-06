// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// D437 §4: the refused copy's and the serving copy's warnings reach the System page's discovery
// section and the needs-attention menu, in Unix milliseconds, and nothing while there is none.
func TestOneCopy_BothWarningsReachTheDiscoveryStatusAndNeedsAttention(t *testing.T) {
	stoodAside := time.UnixMilli(1_790_000_000_000)
	refusedAt := time.UnixMilli(1_790_000_600_000)
	status := discovery.Status{StoodAsideAt: stoodAside, CopyRefusedFrom: "203.0.113.9", CopyRefusedAt: refusedAt}

	body := discoveryStatusBody(status)
	if body.StoodAsideAt != stoodAside.UnixMilli() || body.CopyRefusedFrom != "203.0.113.9" ||
		body.CopyRefusedAt != refusedAt.UnixMilli() {
		t.Errorf("/system/discovery says stoodAsideAt %d, copyRefusedFrom %q, copyRefusedAt %d; want %d, %q, %d",
			body.StoodAsideAt, body.CopyRefusedFrom, body.CopyRefusedAt,
			stoodAside.UnixMilli(), "203.0.113.9", refusedAt.UnixMilli())
	}
	output := &NeedsAttentionOutput{}
	addOneCopyWarnings(output, status)
	if output.Body.StoodAsideAt != stoodAside.UnixMilli() || output.Body.CopyRefusedFrom != "203.0.113.9" ||
		output.Body.CopyRefusedAt != refusedAt.UnixMilli() {
		t.Errorf("/needs-attention says stoodAsideAt %d, copyRefusedFrom %q, copyRefusedAt %d; want %d, %q, %d",
			output.Body.StoodAsideAt, output.Body.CopyRefusedFrom, output.Body.CopyRefusedAt,
			stoodAside.UnixMilli(), "203.0.113.9", refusedAt.UnixMilli())
	}

	none := discoveryStatusBody(discovery.Status{})
	if none.StoodAsideAt != 0 || none.CopyRefusedFrom != "" || none.CopyRefusedAt != 0 {
		t.Errorf("with no warning, /system/discovery says %d, %q, %d; want 0, empty, 0",
			none.StoodAsideAt, none.CopyRefusedFrom, none.CopyRefusedAt)
	}
}
