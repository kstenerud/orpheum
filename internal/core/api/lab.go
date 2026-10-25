// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// labResultBytesAtMost bounds a lab page's results: a device report or a probe's log of a few
// hundred events is a few KiB, and the remote page's two-hour log, sent whole every 30 s, stays
// well under it (tv-device-tests.md). A justified constant, not a setting: it bounds what one
// signed-in request may make the core parse and log, as every endpoint's body is bounded.
const labResultBytesAtMost = 64 << 10

// labResultIDLength is how many characters of a random base32 text name a lab result: enough
// that two results a person sends in a week never share one, short enough to read off a TV and
// find in the log. A justified constant.
const labResultIDLength = 8

// LabResultInput is what one of the lab pages measured on a device (plans/tv-device-tests.md): a
// device's capabilities, its input, a phone's endurance, extending, casting or the cast receiver's
// own report. Signed in only, taken only while the admin offers the device tests
// (should_offer_device_tests, the owner, 2026-10-25), bounded, and only logged, as a page's failure
// is (ClientProblemInput): a TV's or a Chromecast's console is read by no one.
type LabResultInput struct {
	AuthorizationHeader
	Body struct {
		Probe  string         `doc:"Which lab page measured it" enum:"device,input,remote,extend,cast,receiver" json:"probe"`  //nolint:lll // one struct tag
		Result map[string]any `doc:"What it measured, as the page assembled it"                                 json:"result"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// LabResultOutput names the logged result, for the page to show beside it.
type LabResultOutput struct {
	Body struct {
		ID string `doc:"The result's name in the server's log" json:"id"`
	}
}

func (server *Server) registerLab() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportLabResult", Method: http.MethodPost, Path: "/lab-results",
		Summary:       "What a lab page measured on a device, for the server's log (tv-device-tests.md)",
		Tags:          []string{tagRunningIt},
		DefaultStatus: http.StatusOK,
		MaxBodyBytes:  labResultBytesAtMost,
	}, server.reportLabResult)
}

func (server *Server) reportLabResult(ctx context.Context, input *LabResultInput) (*LabResultOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isOffered, err := dependencies.Settings.Bool(ctx, settings.ShouldOfferDeviceTests)
	if err != nil {
		return nil, server.settingsError(ctx, "read "+string(settings.ShouldOfferDeviceTests), err)
	}
	if !isOffered {
		// As a server that serves no client or takes no WebRTC connection says so: there is no
		// such thing here while it is off.
		return nil, newError(http.StatusNotFound, CodeNotFound, "device tests are turned off on this server")
	}
	result, err := json.Marshal(input.Body.Result)
	if err != nil {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.result", "the result is not JSON")
	}
	output := &LabResultOutput{}
	output.Body.ID = rand.Text()[:labResultIDLength]
	server.logger.InfoContext(ctx, "a lab page sent its results", "labResultId", output.Body.ID,
		"probe", input.Body.Probe, "userId", principal.UserID, "deviceId", principal.DeviceID,
		"userAgent", input.userAgent, "result", json.RawMessage(result))
	return output, nil
}
