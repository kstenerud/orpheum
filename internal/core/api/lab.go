// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
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
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportLabInputEvents", Method: http.MethodPost, Path: "/lab-input-events",
		Summary:       "What the button test heard, as it heard it, for the server's log (tv-device-tests.md H2)",
		Tags:          []string{tagRunningIt},
		DefaultStatus: http.StatusNoContent,
		MaxBodyBytes:  labResultBytesAtMost,
	}, server.reportLabInputEvents)
}

// requireDeviceTests signs the caller in and refuses them while the admin has the device tests
// off (should_offer_device_tests): every lab endpoint's one gate.
func (server *Server) requireDeviceTests(ctx context.Context, authorization string) (auth.Principal, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return auth.Principal{}, err
	}
	isOffered, err := dependencies.Settings.Bool(ctx, settings.ShouldOfferDeviceTests)
	if err != nil {
		return auth.Principal{}, server.settingsError(ctx, "read "+string(settings.ShouldOfferDeviceTests), err)
	}
	if !isOffered {
		// As a server that serves no client or takes no WebRTC connection says so: there is no
		// such thing here while it is off.
		return auth.Principal{}, newError(http.StatusNotFound, CodeNotFound,
			"device tests are turned off on this server")
	}
	return principal, nil
}

func (server *Server) reportLabResult(ctx context.Context, input *LabResultInput) (*LabResultOutput, error) {
	principal, err := server.requireDeviceTests(ctx, input.Authorization)
	if err != nil {
		return nil, err
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

// LabInputRunStart is the button test's page as it opened, sent with its run's first events.
type LabInputRunStart struct {
	ViewportWidth    int64   `doc:"The page's width in CSS pixels"            json:"viewportWidth"    maximum:"100000" minimum:"0"` //nolint:lll // one struct tag
	ViewportHeight   int64   `doc:"The page's height in CSS pixels"           json:"viewportHeight"   maximum:"100000" minimum:"0"` //nolint:lll // one struct tag
	DevicePixelRatio float64 `doc:"Device pixels to a CSS pixel"              json:"devicePixelRatio" maximum:"100"    minimum:"0"` //nolint:lll // one struct tag
	Mode             string  `doc:"Who moves focus between the tiles"         enum:"browser,geometric" json:"mode"`                 //nolint:lll // one struct tag
	HasMediaSession  bool    `doc:"Whether the browser has Media Session"     json:"hasMediaSession"`
}

// LabInputEvent is one thing the button test heard or measured: a key going down, a step back in
// the page's history, a controller's button, a Media Session action, a move of focus with its
// latency, or a change of who moves focus. Its fields are the log line's attributes, so each is
// named here and bounded: a page cannot add an attribute or write a long value into the log.
type LabInputEvent struct {
	Seq       int64  `doc:"Its place in the run, from 0"                           json:"seq"                 maximum:"1000000000" minimum:"0"`                                                               //nolint:lll // one struct tag
	AtMs      int64  `doc:"Milliseconds since the page loaded"                     json:"atMs"                maximum:"1000000000" minimum:"0"`                                                               //nolint:lll // one struct tag
	Kind      string `doc:"What it is"                                             enum:"key,back,gamepad,media,move,mode" json:"kind"`                                                                       //nolint:lll // one struct tag
	Button    string `doc:"The button the test named it, where it is a press"      enum:"up,down,left,right,select,back,playPause,rewind,fastForward,volumeUp,volumeDown,mute,other" json:"button,omitempty"` //nolint:lll // one struct tag
	Mode      string `doc:"Who moved focus when it came"                           enum:"browser,geometric" json:"mode,omitempty"`                                                                            //nolint:lll // one struct tag
	Key       string `doc:"A key's key"                                            json:"key,omitempty"       maxLength:"40"`                                                                                 //nolint:lll // one struct tag
	Code      string `doc:"A key's code"                                           json:"code,omitempty"      maxLength:"40"`                                                                                 //nolint:lll // one struct tag
	KeyCode   *int64 `doc:"A key's keyCode"                                        json:"keyCode,omitempty"   maximum:"100000"     minimum:"0"`                                                               //nolint:lll // one struct tag
	IsRepeat  *bool  `doc:"Whether a key is its own repeat"                        json:"isRepeat,omitempty"`
	PadIndex  *int64 `doc:"A controller's index"                                   json:"padIndex,omitempty"  maximum:"63"         minimum:"0"`                                //nolint:lll // one struct tag
	PadButton *int64 `doc:"A controller's button, in its mapping"                  json:"padButton,omitempty" maximum:"63"         minimum:"0"`                                //nolint:lll // one struct tag
	PadID     string `doc:"A controller's own name for itself"                     json:"padId,omitempty"     maxLength:"100"`                                                 //nolint:lll // one struct tag
	Action    string `doc:"A Media Session action"                                 enum:"play,pause,seekbackward,seekforward,previoustrack,nexttrack" json:"action,omitempty"` //nolint:lll // one struct tag
	LatencyMs *int64 `doc:"A move's time from its key to the frame after focus moved" json:"latencyMs,omitempty" maximum:"60000" minimum:"0"`                                  //nolint:lll // one struct tag
}

// LabInputEventsInput is the button test's events since its last request, under its run's name,
// the first request's with the page as it opened. Signed in only, taken only while the admin
// offers the device tests, bounded, and only logged, each event as a line of its own, for the run
// to be read from the log by its name (tv-device-tests.md H2, the owner, 2026-10-25: "With the
// right logging it should be trivial to figure out the events").
type LabInputEventsInput struct {
	AuthorizationHeader
	Body struct {
		Run   string            `doc:"The run's name, made by the page as it loaded and shown on it" json:"run" pattern:"^[A-Z0-9]{8}$"` //nolint:lll // one struct tag
		Start *LabInputRunStart `doc:"The page as it opened, with the run's first events"           json:"start,omitempty"`              //nolint:lll // one struct tag
		// At most 200 events a request: the page sends what it heard while its last request was on
		// its way, a few presses, and splits a longer wait's into requests of at most that many;
		// more is refused whole, as a body that is not the page's. A justified constant, as
		// labResultBytesAtMost is.
		Events []LabInputEvent `doc:"What it heard since its last request, in order"               json:"events" maxItems:"200" minItems:"1"` //nolint:lll // one struct tag
	}
	ClientFacts
}

func (server *Server) reportLabInputEvents(ctx context.Context, input *LabInputEventsInput) (*struct{}, error) {
	principal, err := server.requireDeviceTests(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	run := input.Body.Run
	if start := input.Body.Start; start != nil {
		server.logger.InfoContext(ctx, "a button test started", "labRun", run,
			"viewport", fmt.Sprintf("%dx%d", start.ViewportWidth, start.ViewportHeight),
			"devicePixelRatio", start.DevicePixelRatio, "mode", start.Mode,
			"hasMediaSession", start.HasMediaSession, "userId", principal.UserID,
			"deviceId", principal.DeviceID, "userAgent", input.userAgent)
	}
	for _, event := range input.Body.Events {
		attributes := labInputEventAttributes(run, event, principal)
		switch event.Kind {
		case "move":
			server.logger.InfoContext(ctx, "a button test measured a move", attributes...)
		case "mode":
			server.logger.InfoContext(ctx, "a button test changed who moves focus", attributes...)
		default:
			server.logger.InfoContext(ctx, "a button test heard a press", attributes...)
		}
	}
	return &struct{}{}, nil
}

// keyCodeAttribute names a key's code in the log, as the event names it.
const keyCodeAttribute = "code"

// labInputEventAttributes is an event's log line, flat: the run, its place and kind first, then
// only the fields it carries, then who sent it.
func labInputEventAttributes(run string, event LabInputEvent, principal auth.Principal) []any {
	attributes := []any{"labRun", run, "seq", event.Seq, "atMs", event.AtMs, "kind", event.Kind}
	for _, text := range []struct{ name, value string }{
		{"button", event.Button},
		{"mode", event.Mode},
		{"key", event.Key},
		{keyCodeAttribute, event.Code},
		{"padId", event.PadID},
		{"action", event.Action},
	} {
		if text.value != "" {
			attributes = append(attributes, text.name, text.value)
		}
	}
	for _, number := range []struct {
		name  string
		value *int64
	}{
		{"keyCode", event.KeyCode},
		{"padIndex", event.PadIndex},
		{"padButton", event.PadButton},
		{"latencyMs", event.LatencyMs},
	} {
		if number.value != nil {
			attributes = append(attributes, number.name, *number.value)
		}
	}
	if event.IsRepeat != nil {
		attributes = append(attributes, "isRepeat", *event.IsRepeat)
	}
	return append(attributes, "userId", principal.UserID, "deviceId", principal.DeviceID)
}
