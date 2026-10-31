// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// ClientProblemInput is a failure a page met, which it otherwise writes only to the browser's
// console: on a phone no one reads that, and a phone's playback that started over by itself left
// nothing to say why (staging, 2026-10-14). Signed in only, bounded, and only logged, as a player's
// report is; the page sends at most a few each time it loads.
type ClientProblemInput struct {
	AuthorizationHeader
	Body struct {
		Message string `doc:"What failed, in the page's own words" json:"message" maxLength:"500" minLength:"1"`
		Page    string `doc:"The page's path when it failed" json:"page" maxLength:"200"`
		// Level is "debug" for a note of what the page did rather than a failure, logged at the
		// debug level (a Fire TV's sign-in that submitted on a checkbox's OK, the owner, 2026-10-30).
		Level string `doc:"debug for a note of what the page did, logged at the debug level; absent for a failure" enum:"debug" json:"level,omitempty" required:"false"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// clientNoteLevel is the Level of a page's note of what it did, not of a failure.
const clientNoteLevel = "debug"

// logKeyClientMessage is the log's key for a page's own words of what failed or what it did.
const logKeyClientMessage = "message"

func (server *Server) registerClientProblems() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportClientProblem", Method: http.MethodPost, Path: "/client-problems",
		Summary:       "A failure the page met, for the server's log",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.reportClientProblem)
}

func (server *Server) reportClientProblem(ctx context.Context, input *ClientProblemInput) (*struct{}, error) {
	_, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	attributes := []any{
		logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID, "page", input.Body.Page,
		logKeyClientMessage, input.Body.Message, logKeyUserAgent, input.userAgent,
	}
	if input.Body.Level == clientNoteLevel {
		server.logger.DebugContext(ctx, "a page noted", attributes...)
	} else {
		server.logger.InfoContext(ctx, "a page reported a failure", attributes...)
	}
	return &struct{}{}, nil
}

// ClientPathInput is how a page served by a discovery chose its path to the core, and what WebRTC's
// pair is as its browser sees it: settling why a page at home went out through the provider and back
// took a database read, two services' logs and the sandbox's interfaces, where this line would have
// said it (local-path-2026-10-27.md §4 items 1, 2). Signed in only, bounded, sent once per reach, and
// only logged, as a page's failure is.
type ClientPathInput struct {
	AuthorizationHeader
	Body ClientPathBody
	ClientFacts
}

// ClientPathBody is a page's choice of its path to the core, and its reasons.
type ClientPathBody struct {
	Browser       string              `doc:"The browser's family, as the page judged it: firefox, chrome, safari or other" json:"browser" maxLength:"40"`                                                                     //nolint:lll // one struct tag
	IsLANNameUsed bool                `doc:"Whether the page tries the core's LAN name in this browser at all" json:"isLanNameUsed"`                                                                                          //nolint:lll // one struct tag
	LocalNetwork  string              `doc:"The browser's local-network permission as the page read it" enum:"granted,denied,prompt,unknown" json:"localNetwork"`                                                             //nolint:lll // one struct tag
	DirectOffered int64               `doc:"How many of the core's names the discovery offered" json:"directOffered" maximum:"100" minimum:"0"`                                                                               //nolint:lll // one struct tag
	LANOffered    int64               `doc:"How many of those are LAN names" json:"lanOffered" maximum:"100" minimum:"0"`                                                                                                     //nolint:lll // one struct tag
	Reached       string              `doc:"What the page reached the core by: \"webrtc\", or \"direct\" and the name's origin" json:"reached" maxLength:"300" minLength:"1"`                                                 //nolint:lll // one struct tag
	Names         []RacedNameBody     `doc:"Each of the core's names the page raced or left out, with what came of it" json:"names" maxItems:"16"`                                                                            //nolint:lll // one struct tag
	WebRTCPairs   []CandidatePairBody `doc:"Over WebRTC, its first connection's candidate pairs, the selected one and those to the core's host candidates first" json:"webrtcPairs,omitempty" maxItems:"16" required:"false"` //nolint:lll // one struct tag
}

// RacedNameBody is one of the core's names and what came of it in the page's first race.
type RacedNameBody struct {
	Name    string `json:"name"    maxLength:"300"`
	Outcome string `doc:"answered; failed, and how; not waited for; or left out, and why" json:"outcome" maxLength:"300"` //nolint:lll // one struct tag
}

// CandidatePairBody is one of a WebRTC connection's candidate pairs as the page's browser sees it
// (RTCPeerConnection.getStats): its state, and each end by candidate type, protocol and address, the
// browser's as it gives it (an mDNS name for a host candidate it hides). The pairs to the core's host
// candidates, its LAN address's among them, say whether a device in the house reached the core's LAN
// UDP port.
type CandidatePairBody struct {
	State      string `doc:"succeeded, failed, in-progress, waiting or frozen" json:"state" maxLength:"20"`
	Browser    string `doc:"The browser's end: type, protocol, address" json:"browser" maxLength:"200"`
	Core       string `doc:"The core's end: type, protocol, address" json:"core" maxLength:"200"`
	IsSelected bool   `doc:"Whether it is the pair the connection selected" json:"isSelected,omitempty" required:"false"` //nolint:lll // one struct tag
}

func (server *Server) registerClientPaths() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportClientPath", Method: http.MethodPost, Path: "/client-paths",
		Summary:       "How a page chose its path to the core, and why, for the server's log",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.reportClientPath)
}

func (server *Server) reportClientPath(ctx context.Context, input *ClientPathInput) (*struct{}, error) {
	_, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body := input.Body
	names := make([]string, 0, len(body.Names))
	for _, name := range body.Names {
		names = append(names, name.Name+" "+name.Outcome)
	}
	attributes := []any{
		logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID, logKeyUserAgent, input.userAgent,
		"over", input.over, "browser", body.Browser, "isLanNameUsed", body.IsLANNameUsed,
		"localNetwork", body.LocalNetwork, "directOffered", body.DirectOffered, "lanOffered", body.LANOffered,
		"names", strings.Join(names, "; "), "reached", body.Reached,
	}
	if len(body.WebRTCPairs) > 0 {
		selected, others := "not one", make([]string, 0, len(body.WebRTCPairs))
		for _, pair := range body.WebRTCPairs {
			text := pair.State + " " + pair.Browser + " -> " + pair.Core
			if pair.IsSelected {
				selected = text
			} else {
				others = append(others, text)
			}
		}
		attributes = append(attributes, "selectedPair", selected, "otherPairs", strings.Join(others, "; "))
	}
	server.logger.InfoContext(ctx, "a page chose its path to the core", attributes...)
	return &struct{}{}, nil
}
