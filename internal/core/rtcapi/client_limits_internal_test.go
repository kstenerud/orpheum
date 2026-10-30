// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// webClientConstant is the integer the web client's SDK exports as name in file, under
// web/packages/sdk/src: a number, or a product of numbers ("64 * 1024"), as the SDK writes them.
func webClientConstant(t *testing.T, file, name string) int64 {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "packages", "sdk", "src", file))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`export const ` + name + ` = ([\d_ *]+);`).FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s has no %s this test can read; say where the client's went", file, name)
	}
	product := int64(1)
	for factor := range strings.SplitSeq(string(found[1]), "*") {
		value, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(factor), "_", ""), 10, 64)
		if err != nil {
			t.Fatalf("%s's %s: %v", file, name, err)
		}
		product *= value
	}
	return product
}

// The web client's transport keeps to the core's bounds, which nothing else holds it to: a lowered
// channelLimit or MaxMessageBytes on one side alone refused requests or broke large answers, and
// only over WebRTC, where it is hardest to see (the audit of 2026-10-25, R3). Each pair is the
// same, or the client's within the core's: requests in flight under the channels a connection
// serves, a Watch Together socket holding one more; a grant asked for within the credit the core
// starts each answer with, or the answer would wait for a grant that never comes.
func TestWebClient_KeepsToTheCoresBounds(t *testing.T) {
	t.Parallel()
	if got := webClientConstant(t, "rtc-request.ts", "maxMessageBytes"); got != MaxMessageBytes {
		t.Errorf("the client's messages hold %d bytes, the core's %d; want the same", got, MaxMessageBytes)
	}
	if got := webClientConstant(t, "rtc-transport-link.ts", "clientConnections"); got != ClientConnections {
		t.Errorf("the client opens %d connections, the core expects %d; want the same", got, ClientConnections)
	}
	if got := webClientConstant(t, "rtc-transport-link.ts", "requestsPerConnection"); got >= channelLimit {
		t.Errorf("the client sends %d requests a connection, the core serves %d; want fewer, a Watch "+
			"Together socket holding one", got, channelLimit)
	}
	if got := webClientConstant(t, "rtc-request.ts", "grantEvery"); got > InitialCreditMessages {
		t.Errorf("the client grants every %d messages, the core starts with %d; want a grant within them",
			got, InitialCreditMessages)
	}
	gather := time.Duration(webClientConstant(t, "rtc-connect.ts", "gatherAfterReflexiveMs")) * time.Millisecond
	if gather != gatherAfterReflexive {
		t.Errorf("the client gathers %v after its reflexive candidate, the core %v; want the same", gather,
			gatherAfterReflexive)
	}
}
