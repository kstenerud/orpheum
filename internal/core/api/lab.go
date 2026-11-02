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
// device's capabilities, a phone's endurance, extending, casting or the cast receiver's
// own report. Signed in only, taken only while the admin offers the device tests
// (should_offer_device_tests, the owner, 2026-10-25), bounded, and only logged, as a page's failure
// is (ClientProblemInput): a TV's or a Chromecast's console is read by no one.
type LabResultInput struct {
	AuthorizationHeader
	Body struct {
		Probe  string         `doc:"Which lab page measured it" enum:"device,remote,extend,cast,receiver,throughput" json:"probe"` //nolint:lll // one struct tag
		Result map[string]any `doc:"What it measured, as the page assembled it"                                 json:"result"`     //nolint:lll // one struct tag
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
	server.registerLabThroughput()
}

// labThroughputProbe is the throughput test's name among the lab pages.
const labThroughputProbe = "throughput"

// requireDeviceTests signs the caller in and refuses them while the admin has the device tests
// off (should_offer_device_tests): every lab endpoint's one gate.
func (server *Server) requireDeviceTests(ctx context.Context, authorization string) (auth.Principal, error) {
	_, principal, err := server.deviceTestsCaller(ctx, authorization)
	return principal, err
}

// deviceTestsCaller is requireDeviceTests, answering the server's dependencies too.
func (server *Server) deviceTestsCaller(
	ctx context.Context, authorization string,
) (*Dependencies, auth.Principal, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, auth.Principal{}, err
	}
	isOffered, err := dependencies.Settings.Bool(ctx, settings.ShouldOfferDeviceTests)
	if err != nil {
		return nil, auth.Principal{}, server.settingsError(ctx, "read "+string(settings.ShouldOfferDeviceTests), err)
	}
	if !isOffered {
		// As a server that serves no client or takes no WebRTC connection says so: there is no
		// such thing here while it is off.
		return nil, auth.Principal{}, newError(http.StatusNotFound, CodeNotFound,
			"device tests are turned off on this server")
	}
	return dependencies, principal, nil
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
	attributes := []any{
		"labResultId", output.Body.ID,
		"probe", input.Body.Probe, logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID,
		"userAgent", input.userAgent, "result", json.RawMessage(result),
	}
	if input.Body.Probe == labThroughputProbe {
		// Its own words, so the lead finds a device's figures by them (webrtc-throughput-2026-10-28.md).
		server.logger.InfoContext(ctx, "a throughput test ran", attributes...)
		return output, nil
	}
	server.logger.InfoContext(ctx, "a lab page sent its results", attributes...)
	return output, nil
}

// LabInputRunStart is the button test's page as it opened, sent with its run's first events.
type LabInputRunStart struct {
	ViewportWidth    int64   `doc:"The page's width in CSS pixels"            json:"viewportWidth"    maximum:"100000" minimum:"0"` //nolint:lll // one struct tag
	ViewportHeight   int64   `doc:"The page's height in CSS pixels"           json:"viewportHeight"   maximum:"100000" minimum:"0"` //nolint:lll // one struct tag
	DevicePixelRatio float64 `doc:"Device pixels to a CSS pixel"              json:"devicePixelRatio" maximum:"100"    minimum:"0"` //nolint:lll // one struct tag
	Mode             string  `doc:"Who moves focus between the tiles"         enum:"browser,geometric" json:"mode"`                 //nolint:lll // one struct tag
	HasMediaSession  bool    `doc:"Whether the browser has Media Session"     json:"hasMediaSession"`
	HasFullscreen    bool    `doc:"Whether elements have requestFullscreen"   json:"hasFullscreen"`
	HasCaptureStream bool    `doc:"Whether a canvas has captureStream, for the full-screen test's video" json:"hasCaptureStream"` //nolint:lll // one struct tag
}

// LabInputEvent is one thing the button test heard or measured: a key going down, a step back in
// the page's history, a controller's button, stick or axis, a Media Session action, a move of focus
// with its latency, a change of who moves focus, its text field's focus, input and composition, the
// visual viewport's resizing, full screen's entering and leaving, a controller seen and the person's
// choice of its OK button (tv-device-tests.md H2, H6–H8), a context menu asked for, and a key's
// modifiers, place and composing, for the volume keys' layouts and a TV's Menu (D407 §10). Its
// fields are the log line's attributes, so each is named here and bounded: a page cannot add an
// attribute or write a long value into the log. The field's text is never sent, only its length.
type LabInputEvent struct {
	Seq              int64    `doc:"Its place in the run, from 0"                                json:"seq"                        maximum:"1000000000" minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	AtMs             int64    `doc:"Milliseconds since the page loaded"                          json:"atMs"                       maximum:"1000000000" minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	Kind             string   `doc:"What it is"                                                  enum:"key,back,gamepad,stick,axis,media,move,mode,fieldIn,fieldOut,fieldInput,compositionStart,compositionEnd,viewport,fullscreenEnter,fullscreenExit,fullscreenRefused,fullscreenExitAsked,pad,okChoice,contextmenu" json:"kind"` //nolint:lll // one struct tag
	Button           string   `doc:"The button the test named it, where it is a press"           enum:"up,down,left,right,select,back,playPause,rewind,fastForward,volumeUp,volumeDown,mute,plus,minus,previousTrack,nextTrack,other" json:"button,omitempty"`                                                                      //nolint:lll // one struct tag
	Mode             string   `doc:"Who moved focus when it came"                                enum:"browser,geometric" json:"mode,omitempty"`                                                                                                                                                                                    //nolint:lll // one struct tag
	Key              string   `doc:"A key's key; a character typed in the field is sent as Character" json:"key,omitempty"       maxLength:"40"`                                                                                                                                                                                    //nolint:lll // one struct tag
	Code             string   `doc:"A key's code"                                                json:"code,omitempty"             maxLength:"40"`                                                                                                                                                                                  //nolint:lll // one struct tag
	KeyCode          *int64   `doc:"A key's keyCode"                                             json:"keyCode,omitempty"          maximum:"100000"     minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	IsRepeat         *bool    `doc:"Whether a key is its own repeat"                             json:"isRepeat,omitempty"`                                                                                                                                                                                                         //nolint:lll // one struct tag
	InField          *bool    `doc:"Whether the test's text field had focus"                     json:"inField,omitempty"`                                                                                                                                                                                                          //nolint:lll // one struct tag
	InFullscreen     *bool    `doc:"Whether the page was full screen"                            json:"inFullscreen,omitempty"`                                                                                                                                                                                                     //nolint:lll // one struct tag
	PadIndex         *int64   `doc:"A controller's index"                                        json:"padIndex,omitempty"         maximum:"63"         minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	PadButton        *int64   `doc:"A controller's button, in its mapping"                       json:"padButton,omitempty"        maximum:"63"         minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	PadButtonName    string   `doc:"A standard controller's button by its place on the pad"      enum:"faceBottom,faceRight,faceLeft,faceTop,shoulderLeft,shoulderRight,triggerLeft,triggerRight,select,start,stickLeft,stickRight,dpadUp,dpadDown,dpadLeft,dpadRight,home" json:"padButtonName,omitempty"`                         //nolint:lll // one struct tag
	PadID            string   `doc:"A controller's own name for itself"                          json:"padId,omitempty"            maxLength:"100"`                                                                                                                                                                                 //nolint:lll // one struct tag
	PadMapping       string   `doc:"A controller's mapping: the standard one, none, or another"  enum:"standard,none,other" json:"padMapping,omitempty"`                                                                                                                                                                            //nolint:lll // one struct tag
	PadLayout        string   `doc:"The layout the page inferred from the controller's name"     enum:"xbox,playstation,nintendo,unknown" json:"padLayout,omitempty"`                                                                                                                                                               //nolint:lll // one struct tag
	OKButton         string   `doc:"Which face button confirms, as inferred or chosen"           enum:"bottom,right" json:"okButton,omitempty"`                                                                                                                                                                                     //nolint:lll // one struct tag
	AxisIndex        *int64   `doc:"A controller's axis"                                         json:"axisIndex,omitempty"        maximum:"63"         minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	AxisValue        *float64 `doc:"The axis's value as it crossed half way"                     json:"axisValue,omitempty"        maximum:"1"          minimum:"-1"`                                                                                                                                                               //nolint:lll // one struct tag
	Action           string   `doc:"A Media Session action"                                      enum:"play,pause,seekbackward,seekforward,previoustrack,nexttrack" json:"action,omitempty"`                                                                                                                                        //nolint:lll // one struct tag
	LatencyMs        *int64   `doc:"A move's time from its key to the frame after focus moved"   json:"latencyMs,omitempty"        maximum:"60000"      minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	ValueLength      *int64   `doc:"How many characters the field holds, never what they are"   json:"valueLength,omitempty"      maximum:"100000"     minimum:"0"`                                                                                                                                                                 //nolint:lll // one struct tag
	InputType        string   `doc:"An input event's inputType"                                  json:"inputType,omitempty"        maxLength:"40"       pattern:"^[A-Za-z]*$"`                                                                                                                                                      //nolint:lll // one struct tag
	Width            *int64   `doc:"The visual viewport's width in CSS pixels"                   json:"width,omitempty"            maximum:"100000"     minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	Height           *int64   `doc:"The visual viewport's height in CSS pixels"                  json:"height,omitempty"           maximum:"100000"     minimum:"0"`                                                                                                                                                                //nolint:lll // one struct tag
	FullscreenTarget string   `doc:"What was asked to go full screen: the video's box or the bare video" enum:"container,video" json:"fullscreenTarget,omitempty"`                                                                                                                                                                  //nolint:lll // one struct tag
	ErrorName        string   `doc:"The name of the error a refused request rejected with"       json:"errorName,omitempty"        maxLength:"40"       pattern:"^[A-Za-z]*$"`                                                                                                                                                      //nolint:lll // one struct tag
	ShiftKey         *bool    `doc:"Whether Shift was held with a key"                          json:"shiftKey,omitempty"`                                                                                                                                                                                                          //nolint:lll // one struct tag
	CtrlKey          *bool    `doc:"Whether Control was held with a key"                        json:"ctrlKey,omitempty"`                                                                                                                                                                                                           //nolint:lll // one struct tag
	AltKey           *bool    `doc:"Whether Alt (Option) was held with a key"                   json:"altKey,omitempty"`                                                                                                                                                                                                            //nolint:lll // one struct tag
	MetaKey          *bool    `doc:"Whether Meta (Command, Windows) was held with a key"         json:"metaKey,omitempty"`                                                                                                                                                                                                          //nolint:lll // one struct tag
	AltGraph         *bool    `doc:"Whether AltGr was held with a key, as getModifierState says" json:"altGraph,omitempty"`                                                                                                                                                                                                         //nolint:lll // one struct tag
	Location         *int64   `doc:"Where a key sits: 0 standard, 1 left, 2 right, 3 the number pad" json:"location,omitempty" maximum:"3" minimum:"0"`                                                                                                                                                                             //nolint:lll // one struct tag
	IsComposing      *bool    `doc:"Whether a key came while an input method composed"          json:"isComposing,omitempty"`                                                                                                                                                                                                       //nolint:lll // one struct tag
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
		Events []LabInputEvent `doc:"What it heard since its last request, in order"               json:"events" maxItems:"200" minItems:"1" nullable:"false"` //nolint:lll // one struct tag
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
			"hasMediaSession", start.HasMediaSession, "hasFullscreen", start.HasFullscreen,
			"hasCaptureStream", start.HasCaptureStream, "userId", principal.UserID,
			"deviceId", principal.DeviceID, "userAgent", input.userAgent)
	}
	for _, event := range input.Body.Events {
		server.logLabInputEvent(ctx, run, event, principal)
	}
	return &struct{}{}, nil
}

// logLabInputEvent logs one of the button test's events as a line of its own, under a message
// saying what the test saw, in a few fixed words, so a run's kinds of line are grepped by message
// too.
func (server *Server) logLabInputEvent(ctx context.Context, run string, event LabInputEvent, principal auth.Principal) {
	attributes := labInputEventAttributes(run, event, principal)
	switch event.Kind {
	case "move":
		server.logger.InfoContext(ctx, "a button test measured a move", attributes...)
	case "mode":
		server.logger.InfoContext(ctx, "a button test changed who moves focus", attributes...)
	case "fieldIn", "fieldOut", "fieldInput", "compositionStart", "compositionEnd":
		server.logger.InfoContext(ctx, "a button test's text field changed", attributes...)
	case "viewport":
		server.logger.InfoContext(ctx, "a button test's visual viewport resized", attributes...)
	case "fullscreenEnter", "fullscreenExit", "fullscreenRefused", "fullscreenExitAsked":
		server.logger.InfoContext(ctx, "a button test's full screen changed", attributes...)
	case "pad", "okChoice":
		server.logger.InfoContext(ctx, "a button test read a controller", attributes...)
	case "axis":
		server.logger.InfoContext(ctx, "a button test heard a controller's axis", attributes...)
	case "contextmenu":
		server.logger.InfoContext(ctx, "a button test heard a context menu", attributes...)
	default:
		server.logger.InfoContext(ctx, "a button test heard a press", attributes...)
	}
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
		{"padButtonName", event.PadButtonName},
		{"padId", event.PadID},
		{"padMapping", event.PadMapping},
		{"padLayout", event.PadLayout},
		{"okButton", event.OKButton},
		{"action", event.Action},
		{"inputType", event.InputType},
		{"fullscreenTarget", event.FullscreenTarget},
		{"errorName", event.ErrorName},
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
		{"axisIndex", event.AxisIndex},
		{"valueLength", event.ValueLength},
		{"location", event.Location},
		{"width", event.Width},
		{"height", event.Height},
	} {
		if number.value != nil {
			attributes = append(attributes, number.name, *number.value)
		}
	}
	if event.AxisValue != nil {
		attributes = append(attributes, "axisValue", *event.AxisValue)
	}
	for _, flag := range []struct {
		name  string
		value *bool
	}{
		{"isRepeat", event.IsRepeat},
		{"inField", event.InField},
		{"inFullscreen", event.InFullscreen},
		{"shiftKey", event.ShiftKey},
		{"ctrlKey", event.CtrlKey},
		{"altKey", event.AltKey},
		{"metaKey", event.MetaKey},
		{"altGraph", event.AltGraph},
		{"isComposing", event.IsComposing},
	} {
		if flag.value != nil {
			attributes = append(attributes, flag.name, *flag.value)
		}
	}
	return append(attributes, "userId", principal.UserID, "deviceId", principal.DeviceID)
}
