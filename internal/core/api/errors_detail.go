// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// InAnotherSessionDetailBody is in_another_session's detail: whose the other session is, for the
// question "Leave ‹session› to join this one?" (D372 §8).
type InAnotherSessionDetailBody struct {
	HostName string `doc:"The other session's host" json:"hostName"`
	IsHost   bool   `doc:"Whether the other session is the person's own" json:"isHost"`
}

func (InAnotherSessionDetailBody) problemCode() Code { return CodeInAnotherSession }

// MediaUnreachableDetailBody is media_unreachable's detail: why the file cannot be read now, on
// which machine, the folder or disk missing, and since when (D241 §4).
type MediaUnreachableDetailBody struct {
	Reason   string `enum:"node_away,root_missing,disk_missing,still_reading" json:"reason"`
	NodeName string `doc:"The machine that holds the file" json:"nodeName"`
	Path     string `doc:"The folder or disk missing on it; absent for a machine away" json:"path,omitempty"`
	// UnavailableAt is when it became unavailable, in Unix milliseconds; absent when not known.
	UnavailableAt int64 `json:"unavailableAt,omitempty"`
}

func (MediaUnreachableDetailBody) problemCode() Code { return CodeMediaUnreachable }

// mediaUnreachable is a play refused because its file cannot be read now, its node named by
// nodeName (D241 §4).
func mediaUnreachable(unreadable *library.UnreadableError, nodeName func(nodeID int64) string) error {
	detail := MediaUnreachableDetailBody(*unavailableBody(&unreadable.Unreadable, nodeName))
	return newDetailedError(http.StatusServiceUnavailable, "this file cannot be read now: "+unreadable.Error(), detail)
}

// SignedOutDetailBody is auth_required's detail: why there is no session, so a client can tell
// the person (cut-a §5.2).
type SignedOutDetailBody struct {
	Reason string `enum:"session_ended,session_expired,no_session" json:"reason"`
}

func (SignedOutDetailBody) problemCode() Code { return CodeAuthRequired }
