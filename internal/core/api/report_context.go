// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "context"

// EventContextBody is what was true of a player's page as it paused or moved, which says where the
// pause or the move came from: the page hidden or its window unfocused, the person's own gesture, a
// media-session action (a media key, the desktop's controls), or hls.js moving the video itself over
// a hole or a stall. Logged only; nothing reads it. Two players' 0.2 s pauses and one 0.25 s jump
// forward were heard as hiccups, and the log could say nothing of why (the owner, 2026-10-17).
type EventContextBody struct {
	IsPageHidden       bool   `json:"isPageHidden,omitempty"       required:"false"`
	HasFocus           bool   `json:"hasFocus,omitempty"           required:"false"`
	IsUserActive       bool   `doc:"The person's gesture is active (navigator.userActivation)" json:"isUserActive,omitempty" required:"false"`         //nolint:lll // one struct tag
	MediaSessionAction string `doc:"A media-session action of the last second" enum:"play,pause" json:"mediaSessionAction,omitempty" required:"false"` //nolint:lll // one struct tag
	SkippedBy          string `doc:"What hls.js moved the video over itself" enum:"hole,nudge" json:"skippedBy,omitempty" required:"false"`            //nolint:lll // one struct tag
	ReadyState         int64  `json:"readyState,omitempty"         maximum:"4"       minimum:"0" required:"false"`
	VideoBuffered      string `json:"videoBuffered,omitempty"      maxLength:"400"   required:"false"`
	AudioBuffered      string `json:"audioBuffered,omitempty"      maxLength:"400"   required:"false"`
}

// PathMoveBody is one move of a player's page to another path to the core: to the name it now
// goes over, when, in the report's clock. Logged only.
type PathMoveBody struct {
	To             string `json:"to"             maxLength:"200" minLength:"1"`
	HappenedMonoMs int64  `json:"happenedMonoMs" minimum:"0"`
}

// eventContextAttributes are an event's context's log attributes; none when it carries none.
func eventContextAttributes(event EventBody) []any {
	held := event.Context
	if held == nil {
		return nil
	}
	return []any{
		"isPageHidden", held.IsPageHidden, "hasFocus", held.HasFocus, "isUserActive", held.IsUserActive,
		"mediaSessionAction", held.MediaSessionAction, "skippedBy", held.SkippedBy,
		"readyState", held.ReadyState, "videoBuffered", held.VideoBuffered,
		"audioBuffered", held.AudioBuffered, "happenedMonoMs", event.HappenedMonoMs,
	}
}

// logPathMoves says each path a player's page moved to, beside its play state's changes, so a
// hiccup that follows a move is seen to.
func (server *Server) logPathMoves(ctx context.Context, reporter reporter, moves []PathMoveBody) {
	for _, move := range moves {
		server.logger.InfoContext(ctx, "a player's page moved to another path to the core",
			append(reporter.logAttributes(), "to", move.To, "happenedMonoMs", move.HappenedMonoMs)...)
	}
}
