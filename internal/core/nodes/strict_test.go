// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A node's message is read strictly (D456): a status with a key said twice is refused, not read as
// the last of them.
func TestStatus_AKeySaidTwiceIsRefused(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	response := harness.post(t, nodeproto.PathStatus, reply.NodeID, strings.NewReader(`{"sentAt":7,"sentAt":8}`))
	if response.status != http.StatusBadRequest || !strings.Contains(string(response.body), "duplicate key") {
		t.Errorf("a status with a duplicate key: %d %q, want a 400 naming it", response.status, response.body)
	}
}
