// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The System page shows a locked node as locked, with what its kernel lacks; an admin's leave to run it
// unconfined is given in the admin's name, and shown with who gave it and when (D450 §1).
func TestUnconfined_AnAdminLetsALockedNodeRun(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	harness.nodes.lockedConfinement = []nodeproto.Confinement{
		{Mechanism: "landlock", CauseCode: "disabledAtBoot", Remedy: "add lsm=landlock"},
	}
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	if !strings.Contains(got.body, `"connection":"locked"`) ||
		!strings.Contains(got.body, `"mechanism":"landlock","isInForce":false,"causeCode":"disabledAtBoot"`) ||
		strings.Contains(got.body, `"unconfinedLeave"`) {
		t.Errorf("GET /system = %d %s; want local locked, lacking Landlock, with no leave", got.status, got.body)
	}
	if answer := harness.do(t, http.MethodPut, "/nodes/local/unconfined", admin, nil); answer.status !=
		http.StatusNoContent {
		t.Fatalf("PUT /nodes/local/unconfined = %d %s; want 204", answer.status, answer.body)
	}
	got = harness.do(t, http.MethodGet, "/system", admin, nil)
	if !strings.Contains(got.body, `"unconfinedLeave":{"acceptedBy":"admin","acceptedAt":`) {
		t.Errorf("GET /system after the leave = %s; want who gave it and when", got.body)
	}
	if by := harness.nodes.leaveGivenBy; len(by) != 1 || by[0] == 0 {
		t.Errorf("the leave was given by users %v; want the signed-in admin's", by)
	}
	wantError(t, harness.do(t, http.MethodPut, "/nodes/attic/unconfined", admin, nil),
		http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodPut, "/nodes/local/unconfined", "", nil),
		http.StatusUnauthorized, api.CodeAuthRequired)
}
