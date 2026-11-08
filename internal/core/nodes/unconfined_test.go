// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// postLocked posts a locked node's report, in this build's protocol unless it names one.
func (harness *fixture) postLocked(t *testing.T, report nodeproto.LockedReport) (nodeproto.LockedReply, int) {
	t.Helper()
	if report.ProtocolVersion == 0 {
		report.ProtocolVersion = nodeproto.ProtocolVersion
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	response := harness.post(t, nodeproto.PathLocked, 0, bytes.NewReader(encoded))
	var reply nodeproto.LockedReply
	if response.status == http.StatusOK {
		if err := json.Unmarshal(response.body, &reply); err != nil {
			t.Fatal(err)
		}
	}
	return reply, response.status
}

// A locked node is enrolled by its report and shown locked with what it lacks, held until an admin
// lets it run unconfined; the leave, with who gave it and when, is then in its report's reply, and its
// hello makes it connected (D450 §1).
func TestLocked_ANodeIsHeldUntilAnAdminLetsItRunUnconfined(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	adminID := storetest.InsertUser(t, harness.database, "karl", "admin")
	lacking := []nodeproto.Confinement{{Mechanism: "landlock", CauseCode: "disabled_at_boot"}}
	reply, status := harness.postLocked(t, nodeproto.LockedReport{Name: "nas", Confinement: lacking})
	if status != http.StatusOK || reply.MayRunUnconfined {
		t.Fatalf("the first report answered %d %+v; want 200 and no leave", status, reply)
	}
	nodeID, err := Enrol(t.Context(), harness.database, harness.clock, "nas")
	if err != nil {
		t.Fatal(err)
	}
	assertShownLocked(t, harness.service, nodeID)
	harness.clock.Advance(time.Minute)
	if err = harness.service.AcceptUnconfined(t.Context(), "nas", adminID); err != nil {
		t.Fatal(err)
	}
	leave, isGiven, err := harness.service.UnconfinedLeave(t.Context(), nodeID)
	if err != nil || !isGiven || leave.AcceptedBy != "karl" || !leave.AcceptedAt.Equal(harness.clock.Now()) {
		t.Errorf("the leave = %+v, %t, %v; want karl's, at now", leave, isGiven, err)
	}
	if reply, _ = harness.postLocked(t, nodeproto.LockedReport{Name: "nas"}); !reply.MayRunUnconfined {
		t.Error("the report after the leave was given answered without it; want it")
	}
	harness.hello(t, nodeproto.Hello{Name: "nas", IsUnconfinedByLeave: true})
	if connection, _ := harness.service.NodeConnection(t.Context(), nodeID); connection != Connected {
		t.Errorf("the node's connection after its hello = %s; want connected", connection)
	}
	if _, _, isLocked := harness.service.LockedConfinement(nodeID); isLocked {
		t.Error("the node is still locked after its hello; want its locked report forgotten")
	}
	if err = harness.service.AcceptUnconfined(t.Context(), "elsewhere", adminID); !errors.Is(err, ErrNoNode) {
		t.Errorf("a leave for a node there is not = %v; want ErrNoNode", err)
	}
}

// assertShownLocked fails the test unless the node is shown locked, heard, with the one mechanism it
// reported lacking.
func assertShownLocked(t *testing.T, service *Service, nodeID int64) {
	t.Helper()
	if connection, lastHeard := service.NodeConnection(t.Context(), nodeID); connection != Locked ||
		lastHeard.IsZero() {
		t.Errorf("the node's connection = %s heard %v; want locked and heard", connection, lastHeard)
	}
	if got, _, isLocked := service.LockedConfinement(nodeID); !isLocked || len(got) != 1 {
		t.Errorf("the node's locked confinement = %+v, %t; want what it reported", got, isLocked)
	}
}

// A locked node in another version of the protocol is refused, as its hello would be (D233 §9).
func TestLocked_AReportInAnotherProtocolIsRefused(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	if _, status := harness.postLocked(t, nodeproto.LockedReport{Name: "nas", ProtocolVersion: 1}); status !=
		http.StatusConflict {
		t.Errorf("a report in protocol 1 answered %d; want 409", status)
	}
	if _, status := harness.postLocked(t, nodeproto.LockedReport{}); status != http.StatusBadRequest {
		t.Errorf("a report naming no node answered %d; want 400", status)
	}
}

// A retired node's locked report goes with it, so nothing shows it locked afterwards (D450 §1).
func TestLocked_ARetiredNodeIsNoLongerLocked(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	lacking := []nodeproto.Confinement{{Mechanism: "landlock", CauseCode: "disabled_at_boot"}}
	if _, status := harness.postLocked(t, nodeproto.LockedReport{Name: "nas", Confinement: lacking}); status !=
		http.StatusOK {
		t.Fatalf("the report answered %d; want 200", status)
	}
	nodeID, err := Enrol(t.Context(), harness.database, harness.clock, "nas")
	if err != nil {
		t.Fatal(err)
	}
	assertShownLocked(t, harness.service, nodeID)
	if err = harness.service.RemoveNode(t.Context(), nodeID); err != nil {
		t.Fatal(err)
	}
	if _, _, isLocked := harness.service.LockedConfinement(nodeID); isLocked {
		t.Error("the retired node is still locked; want its locked report gone with it")
	}
}
