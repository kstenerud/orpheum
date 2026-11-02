// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// What a lab page measured on a device is logged as one line, whose name the page is given to show,
// with the result whole inside it, and only from a signed-in page, bounded: a TV's console is read by
// no one, and the device tests are read from the log (tv-device-tests.md).
func TestLab_AResultIsLoggedAsOneLineNamedByItsIDAndIsBounded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	sent := map[string]any{"probe": "device", "result": map[string]any{
		"screen": map[string]any{"width": 1920, "height": 1080}, "hasMediaSource": true,
	}}
	answer := harness.do(t, http.MethodPost, "/lab-results", user, sent)
	if answer.status != http.StatusOK {
		t.Fatalf("POST /lab-results = %d %s", answer.status, answer.body)
	}
	var named struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(answer.body), &named); err != nil || len(named.ID) != api.LabResultIDLength {
		t.Fatalf("the answer %s names no result (%v)", answer.body, err)
	}
	harness.logs.AwaitLine(t, `"msg":"a lab page sent its results"`, `"labResultId":"`+named.ID+`"`,
		`"probe":"device"`, `"userId":1`,
		`"result":{"hasMediaSource":true,"screen":{"height":1080,"width":1920}}`)

	answer = harness.do(t, http.MethodPost, "/lab-results", "", sent)
	if answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing sent a result: %d", answer.status)
	}
	tooLarge := map[string]any{"probe": "remote", "result": map[string]any{
		"log": strings.Repeat("x", api.LabResultBytesAtMost),
	}}
	answer = harness.do(t, http.MethodPost, "/lab-results", user, tooLarge)
	if answer.status != http.StatusRequestEntityTooLarge {
		t.Errorf("a result over %d bytes was taken: %d %s", api.LabResultBytesAtMost, answer.status, answer.body)
	}
	unknown := map[string]any{"probe": "elsewhere", "result": map[string]any{}}
	answer = harness.do(t, http.MethodPost, "/lab-results", user, unknown)
	if answer.status != http.StatusUnprocessableEntity {
		t.Errorf("a result from no lab page was taken: %d", answer.status)
	}
	// The button test sends its events as they come (/lab-input-events), never a whole result.
	whole := map[string]any{"probe": "input", "result": map[string]any{}}
	answer = harness.do(t, http.MethodPost, "/lab-results", user, whole)
	if answer.status != http.StatusUnprocessableEntity {
		t.Errorf("a whole result from the button test was taken: %d", answer.status)
	}
}

// The device tests start off (the owner, 2026-10-25: for beta participants, once an admin turns
// them on): until then no client is told to offer them and the server takes no result, refused as
// a thing this server does not have; once on, both.
func TestLab_ResultsAreRefusedAndTheTestsNotOfferedUntilTheAdminTurnsThemOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	isOffered := func() bool {
		served := harness.do(t, http.MethodGet, "/client-settings", user, nil)
		var got api.ClientSettingsBody
		if err := json.Unmarshal([]byte(served.body), &got); err != nil || served.status != http.StatusOK {
			t.Fatalf("GET /client-settings = %d %s (%v)", served.status, served.body, err)
		}
		return got.ShouldOfferDeviceTests
	}
	sent := map[string]any{"probe": "device", "result": map[string]any{"hasMediaSource": true}}
	if isOffered() {
		t.Error("the device tests are offered before the admin turned them on")
	}
	wantError(t, harness.do(t, http.MethodPost, "/lab-results", user, sent), http.StatusNotFound, api.CodeNotFound)

	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	if !isOffered() {
		t.Error("the device tests are not offered once the admin turned them on")
	}
	if answer := harness.do(t, http.MethodPost, "/lab-results", user, sent); answer.status != http.StatusOK {
		t.Errorf("POST /lab-results with the tests on = %d %s", answer.status, answer.body)
	}
}

// The button test's events, from a page whose run is named K3J9QW2A.
func labInputEvents(events ...map[string]any) map[string]any {
	return map[string]any{"run": "K3J9QW2A", "events": events}
}

// A key going down, as the button test sends it.
func labKeyEvent(seq int) map[string]any {
	return map[string]any{
		"seq": seq, "atMs": 5310, "kind": "key", "button": "select", "mode": "geometric",
		"key": "Enter", "code": "NumpadEnter", "keyCode": 13, "isRepeat": false,
	}
}

// The button test's events are each logged as a line of their own, flat, by its run's name, with
// a line for the run's start, so a run is read from the log by grepping its name (the owner,
// 2026-10-25: "With the right logging it should be trivial to figure out the events"); a request
// is bounded in its events and in each value, and names no attribute of its own.
func TestLab_TheButtonTestsEventsAreLoggedALineEachAndBounded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	sent := labInputEvents(labKeyEvent(0),
		map[string]any{"seq": 1, "atMs": 5376, "kind": "move", "mode": "geometric", "latencyMs": 66},
		map[string]any{
			"seq": 2, "atMs": 6000, "kind": "gamepad", "button": "back", "padIndex": 0,
			"padButton": 1, "padId": "Xbox Wireless Controller",
		},
		map[string]any{"seq": 3, "atMs": 7000, "kind": "media", "button": "playPause", "action": "play"},
		map[string]any{"seq": 4, "atMs": 8000, "kind": "back", "button": "back"},
		map[string]any{"seq": 5, "atMs": 9000, "kind": "mode", "mode": "browser"},
	)
	sent["start"] = map[string]any{
		"viewportWidth": 960, "viewportHeight": 540, "devicePixelRatio": 2, "mode": "geometric",
		"hasMediaSession": true, "hasFullscreen": true, "hasCaptureStream": true,
	}
	answer := harness.do(t, http.MethodPost, "/lab-input-events", user, sent)
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /lab-input-events = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a button test started"`, `"labRun":"K3J9QW2A"`,
		`"viewport":"960x540"`, `"devicePixelRatio":2`, `"mode":"geometric"`, `"hasMediaSession":true`,
		`"userId":1`, `"userAgent":`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"labRun":"K3J9QW2A"`,
		`"seq":0,"atMs":5310,"kind":"key","button":"select","mode":"geometric","key":"Enter",`+
			`"code":"NumpadEnter","keyCode":13`,
		`"isRepeat":false`, `"userId":1`, `"deviceId":`)
	harness.logs.AwaitLine(t, `"msg":"a button test measured a move"`, `"seq":1`, `"kind":"move"`,
		`"latencyMs":66`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":2`, `"kind":"gamepad"`,
		`"padId":"Xbox Wireless Controller"`, `"padIndex":0`, `"padButton":1`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":3`, `"kind":"media"`,
		`"action":"play"`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":4`, `"kind":"back"`)
	harness.logs.AwaitLine(t, `"msg":"a button test changed who moves focus"`, `"seq":5`, `"kind":"mode"`,
		`"mode":"browser"`)

	answer = harness.do(t, http.MethodPost, "/lab-input-events", "", sent)
	if answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing sent events: %d", answer.status)
	}
	tooMany := make([]map[string]any, 201)
	for index := range tooMany {
		tooMany[index] = labKeyEvent(index)
	}
	longKey := labKeyEvent(0)
	longKey["key"] = strings.Repeat("x", 41)
	ownAttribute := labKeyEvent(0)
	ownAttribute["level"] = "ERROR"
	for name, refused := range map[string]map[string]any{
		"201 events":           labInputEvents(tooMany...),
		"no events":            {"run": "K3J9QW2A", "events": []map[string]any{}},
		"events of null":       {"run": "K3J9QW2A", "events": nil},
		"a kind of its own":    labInputEvents(map[string]any{"seq": 0, "atMs": 0, "kind": "anything"}),
		"a key of 41":          labInputEvents(longKey),
		"an attribute its own": labInputEvents(ownAttribute),
		"a run of its own":     {"run": "../etc", "events": []map[string]any{labKeyEvent(0)}},
	} {
		answer := harness.do(t, http.MethodPost, "/lab-input-events", user, refused)
		if answer.status != http.StatusUnprocessableEntity {
			t.Errorf("%s were taken: %d %s", name, answer.status, answer.body)
		}
	}
}

// The button test's events are refused while the admin has the device tests off, as a result is.
func TestLab_TheButtonTestsEventsAreRefusedUntilTheAdminTurnsTheTestsOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	sent := labInputEvents(labKeyEvent(0))
	wantError(t, harness.do(t, http.MethodPost, "/lab-input-events", user, sent), http.StatusNotFound, api.CodeNotFound)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	answer := harness.do(t, http.MethodPost, "/lab-input-events", user, sent)
	if answer.status != http.StatusNoContent {
		t.Errorf("POST /lab-input-events with the tests on = %d %s", answer.status, answer.body)
	}
}

// The button test's text field, full-screen and controller events are logged flat as the rest are,
// each kind under its own message (tv-device-tests.md H6–H8); the field's length is sent, never its
// text, and each new value is bounded by a list, a range or a pattern.
func TestLab_TheButtonTestsFieldFullScreenAndControllerEventsAreLoggedFlat(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	inField := labKeyEvent(0)
	inField["inField"] = true
	inFullscreen := labKeyEvent(9)
	inFullscreen["inFullscreen"] = true
	sent := labInputEvents(inField,
		map[string]any{"seq": 1, "atMs": 100, "kind": "fieldIn"},
		map[string]any{"seq": 2, "atMs": 110, "kind": "fieldInput", "valueLength": 3, "inputType": "insertText"},
		map[string]any{"seq": 3, "atMs": 120, "kind": "compositionEnd", "valueLength": 4},
		map[string]any{"seq": 4, "atMs": 130, "kind": "viewport", "width": 960, "height": 270},
		map[string]any{"seq": 5, "atMs": 140, "kind": "fullscreenEnter", "fullscreenTarget": "container"},
		map[string]any{
			"seq": 6, "atMs": 150, "kind": "fullscreenRefused", "fullscreenTarget": "video",
			"errorName": "NotAllowedError",
		},
		map[string]any{
			"seq": 7, "atMs": 160, "kind": "pad", "padIndex": 0, "padId": "057e-2009-Pro Controller",
			"padMapping": "standard", "padLayout": "nintendo", "okButton": "right",
		},
		map[string]any{
			"seq": 8, "atMs": 170, "kind": "stick", "button": "up", "padIndex": 0, "axisIndex": 1,
			"axisValue": -0.75, "padLayout": "nintendo",
		},
		inFullscreen,
		map[string]any{
			"seq": 10, "atMs": 180, "kind": "gamepad", "button": "select", "padIndex": 0, "padButton": 1,
			"padButtonName": "faceRight", "padLayout": "nintendo",
		},
		map[string]any{"seq": 11, "atMs": 190, "kind": "okChoice", "okButton": "right"},
		map[string]any{"seq": 12, "atMs": 200, "kind": "axis", "padIndex": 1, "axisIndex": 9, "axisValue": 1},
	)
	sent["start"] = map[string]any{
		"viewportWidth": 960, "viewportHeight": 540, "devicePixelRatio": 2, "mode": "geometric",
		"hasMediaSession": true, "hasFullscreen": true, "hasCaptureStream": false,
	}
	answer := harness.do(t, http.MethodPost, "/lab-input-events", user, sent)
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /lab-input-events = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a button test started"`, `"hasFullscreen":true`,
		`"hasCaptureStream":false`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":0`, `"inField":true`)
	harness.logs.AwaitLine(t, `"msg":"a button test's text field changed"`, `"seq":1`, `"kind":"fieldIn"`)
	harness.logs.AwaitLine(t, `"msg":"a button test's text field changed"`, `"seq":2`,
		`"inputType":"insertText"`, `"valueLength":3`)
	harness.logs.AwaitLine(t, `"msg":"a button test's text field changed"`, `"seq":3`,
		`"kind":"compositionEnd"`, `"valueLength":4`)
	harness.logs.AwaitLine(t, `"msg":"a button test's visual viewport resized"`, `"seq":4`,
		`"width":960,"height":270`)
	harness.logs.AwaitLine(t, `"msg":"a button test's full screen changed"`, `"seq":5`,
		`"kind":"fullscreenEnter"`, `"fullscreenTarget":"container"`)
	harness.logs.AwaitLine(t, `"msg":"a button test's full screen changed"`, `"seq":6`,
		`"fullscreenTarget":"video"`, `"errorName":"NotAllowedError"`)
	harness.logs.AwaitLine(t, `"msg":"a button test read a controller"`, `"seq":7`,
		`"padId":"057e-2009-Pro Controller","padMapping":"standard","padLayout":"nintendo","okButton":"right"`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":8`, `"kind":"stick"`,
		`"button":"up"`, `"axisIndex":1`, `"axisValue":-0.75`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":9`, `"inFullscreen":true`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":10`,
		`"padButtonName":"faceRight"`, `"padLayout":"nintendo"`)
	harness.logs.AwaitLine(t, `"msg":"a button test read a controller"`, `"seq":11`, `"kind":"okChoice"`,
		`"okButton":"right"`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a controller's axis"`, `"seq":12`,
		`"axisIndex":9`, `"axisValue":1`)

	// One event of kind carrying field's value, at the run's start.
	event := func(kind, field string, value any) map[string]any {
		return labInputEvents(map[string]any{"seq": 0, "atMs": 0, "kind": kind, field: value})
	}
	for name, refused := range map[string]map[string]any{
		"a field's text":           event("fieldInput", "value", "secret"),
		"an input type of its own": event("fieldInput", "inputType", "insert text"),
		"a length below 0":         event("fieldInput", "valueLength", -1),
		"a viewport too wide":      event("viewport", "width", 100001),
		"a target of its own":      event("fullscreenEnter", "fullscreenTarget", "page"),
		"an error of its own":      event("fullscreenRefused", "errorName", "<b>"),
		"a layout of its own":      event("pad", "padLayout", "sega"),
		"a mapping of its own":     event("pad", "padMapping", "xr-standard"),
		"an OK of its own":         event("okChoice", "okButton", "left"),
		"a button name its own":    event("gamepad", "padButtonName", "turbo"),
		"an axis past 1":           event("axis", "axisValue", 1.5),
		"an axis of 64":            event("axis", "axisIndex", 64),
		"in a field as text":       event("key", "inField", "yes"),
	} {
		answer := harness.do(t, http.MethodPost, "/lab-input-events", user, refused)
		if answer.status != http.StatusUnprocessableEntity {
			t.Errorf("%s was taken: %d %s", name, answer.status, answer.body)
		}
	}
}

// A key's modifiers, its place on the keyboard and whether it came while composing are logged with
// it, and a context menu asked for under its own message (D407 §10): the volume keys' rule is read
// off a layout's + and −, and Silk's Menu key off what the page hears of it. Each is bounded.
func TestLab_TheButtonTestsKeyModifiersAndContextMenuAreLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	plus := map[string]any{
		"seq": 0, "atMs": 100, "kind": "key", "button": "other", "key": "+", "code": "Digit1",
		"keyCode": 49, "shiftKey": true, "ctrlKey": true, "altKey": true, "metaKey": true,
		"altGraph": true, "location": 3, "isComposing": true,
	}
	sent := labInputEvents(plus,
		map[string]any{"seq": 1, "atMs": 200, "kind": "contextmenu", "inFullscreen": true})
	answer := harness.do(t, http.MethodPost, "/lab-input-events", user, sent)
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /lab-input-events = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a button test heard a press"`, `"seq":0`, `"key":"+"`,
		`"location":3`, `"shiftKey":true,"ctrlKey":true,"altKey":true,"metaKey":true,"altGraph":true`,
		`"isComposing":true`)
	harness.logs.AwaitLine(t, `"msg":"a button test heard a context menu"`, `"seq":1`,
		`"kind":"contextmenu"`, `"inFullscreen":true`)

	key := func(field string, value any) map[string]any {
		event := labKeyEvent(0)
		event[field] = value
		return labInputEvents(event)
	}
	for name, refused := range map[string]map[string]any{
		"a location past the number pad": key("location", 4),
		"a location below 0":             key("location", -1),
		"Shift as text":                  key("shiftKey", "yes"),
		"AltGr as a number":              key("altGraph", 1),
	} {
		answer := harness.do(t, http.MethodPost, "/lab-input-events", user, refused)
		if answer.status != http.StatusUnprocessableEntity {
			t.Errorf("%s was taken: %d %s", name, answer.status, answer.body)
		}
	}
}
