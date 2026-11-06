// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"time"
)

// ErrAnotherCopy is the discovery refusing this core because another instance of it, a copy, is
// connected and still answers: what the admin is told. This one stops asking (D437 §3).
var ErrAnotherCopy = errors.New("another copy of this server serves at the discovery; this one stopped " +
	"asking for it, and serves only at home until an admin asks again or it restarts. Asking again " +
	"takes the discovery only once the other copy has stopped")

// ErrHolderAway is the discovery waiting for the installation that last held the core's realm to come
// back, before it admits this one (D437 §5): what the admin is told while the core waits to ask again.
var ErrHolderAway = errors.New("the discovery waits a few minutes for the copy of this server that last " +
	"served to come back; this one asks again then")

// standingAsideWait is the step loop's wait while the core stands aside, refused as a copy: it asks
// nothing, so it only waits to be woken by an admin asking again (D437 §3).
const standingAsideWait = 24 * time.Hour

// holderAwayFirst and holderAwayMost bound the wait a discovery names for the last holder's window:
// a wait of none, or of more than an hour, is taken as the bound (D437 §5).
const (
	holderAwayFirst = time.Second
	holderAwayMost  = time.Hour
)

// holderAwayUnnamed is the wait on a holder-away refusal naming none, though the protocol says it names one.
const holderAwayUnnamed = time.Minute

// AskAgain has a core that stood aside, refused as a copy, ask its discovery again now: what its
// admin does on the System page once the other copy has stopped (D437 §3). A core that does not stand
// aside is unchanged.
func (service *Service) AskAgain(ctx context.Context) Status {
	service.mutex.Lock()
	wasAside := !service.status.StoodAsideAt.IsZero()
	service.status.StoodAsideAt = time.Time{}
	service.mutex.Unlock()
	if wasAside {
		service.parts.Logger.InfoContext(ctx, "discovery: an admin asked to ask the discovery again")
		service.nudge()
	}
	return service.Status()
}

// standAside is the core refused as a copy of another that serves: it says so, to its admins and the
// log, and asks no more (D437 §3, §4).
func (service *Service) standAside(ctx context.Context) time.Duration {
	service.mutex.Lock()
	wasAside := !service.status.StoodAsideAt.IsZero()
	service.status.Problem = "open a session: " + ErrAnotherCopy.Error()
	if !wasAside {
		service.status.StoodAsideAt = service.parts.Clock.Now()
	}
	service.token, service.isUnreachable = "", false
	service.mutex.Unlock()
	if wasAside {
		return standingAsideWait
	}
	service.parts.Logger.WarnContext(ctx, "discovery: another copy of this server serves at the discovery; "+
		"this one stops asking until an admin asks again or it restarts, and serves only at home")
	return standingAsideWait
}

// waitForTheHolder is the core refused while the discovery waits for the last holder of the realm:
// it says so, and asks again after the wait the discovery named (D437 §5).
func (service *Service) waitForTheHolder(ctx context.Context, retryAfter time.Duration) time.Duration {
	wait := min(max(retryAfter, holderAwayFirst), holderAwayMost)
	if retryAfter <= 0 {
		wait = holderAwayUnnamed
	}
	problem := "open a session: " + ErrHolderAway.Error()
	service.mutex.Lock()
	isNew := service.status.Problem != problem
	service.status.Problem, service.isUnreachable = problem, false
	service.mutex.Unlock()
	if isNew {
		service.parts.Logger.WarnContext(ctx, "discovery: the discovery waits for the copy that last served; "+
			"asking again later", "waitMs", wait.Milliseconds())
	}
	return wait
}
