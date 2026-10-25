// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A connection is named in the log by whose it is once a request over it is signed in, so a network
// turned away past its share says whose connections filled it (the owner's four browsers, staging,
// 2026-10-25, when nothing said whose the 16 were).
func TestRTCOffer_NamesAConnectionByTheSignInOfItsRequests(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	rtcLogs := &testutil.Log{}
	rtc := rtcapi.New(t.Context(), harness.server.Handler(), slog.New(slog.NewJSONHandler(rtcLogs, nil)))
	t.Cleanup(func() { _ = rtc.Close() }) //nolint:errcheck // the test is over
	harness.server.SetRTC(rtc)
	token := harness.createAdminAndSignIn(t)
	//nolint:contextcheck // the harness's requests carry the test's context
	answer := func(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answered := harness.do(t, http.MethodPost, "/rtc/offer", token, map[string]any{
			"type": offer.Type, "sdp": offer.SDP,
		})
		var body SessionDescriptionBody
		if answered.status != http.StatusOK || json.Unmarshal([]byte(answered.body), &body) != nil {
			return rtcapi.Description{}, fmt.Errorf("POST /rtc/offer = %d %s", answered.status, answered.body)
		}
		return rtcapi.Description{Type: body.Type, SDP: body.SDP}, nil
	}
	first := rtcapitest.Connect(t, answer)
	signedIn := first.Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: Prefix + "/libraries",
		Headers: http.Header{"Authorization": {"Bearer " + token}},
	}, nil)
	if signedIn.Head.Status != http.StatusOK {
		t.Fatalf("GET /libraries over a channel = %d %s", signedIn.Head.Status, signedIn.Body)
	}
	// The next connection from the network logs the ones it already holds, the first by its sign-in.
	rtcapitest.Connect(t, answer)
	rtcLogs.AwaitLine(t, `"msg":"rtcapi: a connection is held"`, `"networkHeld":2`, `(user `, ` device `)
}
