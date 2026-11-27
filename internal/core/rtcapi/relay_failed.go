// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// relayWait is the longest an answer through a relay waits for the core's relayed candidate: the
// wait a STUN server has (stunGatherTimeout). A relay that gave none by then is marked failed and
// the answer goes with the candidates the core has (D567 §3).
const relayWait = stunGatherTimeout

// relayLeftOutFor is how long a failed relay is left out when the core's discovery says nothing of
// its announcements, as one older than protocol 1.1 does (D567 §3).
const relayLeftOutFor = 5 * time.Minute

// RelayFailure is a relay the core asked for an address for an answer and that gave none within
// relayWait: its name, its TURN URLs' host, and when the core marked it.
type RelayFailure struct {
	Name string
	At   time.Time
}

// RelayName is the host a TURN URL names, "" for a URL that is not TURN's: what a failure names the
// relay by, and what the discovery's status says its announcements of.
func RelayName(turnURL string) string {
	if !isTURN(turnURL) {
		return ""
	}
	_, rest, _ := strings.Cut(turnURL, ":")
	rest, _, _ = strings.Cut(rest, "?")
	parsed, err := url.Parse("//" + rest)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// ReportRelayFailuresWith has report told of each relay the core marks failed, once a marking. It
// must not wait: the answer is on its way.
func (server *Server) ReportRelayFailuresWith(report func(context.Context, RelayFailure)) {
	server.relayReport.Store(&report)
}

// UseRelayAnnouncements has the core read, of a relay it marked failed, when its discovery last
// heard the relay announce itself (announced names it by RelayName; false when the discovery says
// nothing of it). A failed relay is taken back once an announcement is after its failure.
func (server *Server) UseRelayAnnouncements(announced func(name string) (time.Time, bool)) {
	server.relayAnnounced.Store(&announced)
}

// leaveOutFailed are servers less each TURN URL of a relay marked failed and not yet taken back, and
// less each server that had no other URL; and the names of the relays they still ask. The server's
// mutex is held.
func (server *Server) leaveOutFailed(servers []webrtc.ICEServer) ([]webrtc.ICEServer, []string) {
	server.takeBackLocked()
	var kept []webrtc.ICEServer
	var asked []string
	for _, each := range servers {
		urls := slices.DeleteFunc(slices.Clone(each.URLs), func(turnURL string) bool {
			_, isFailed := server.failedRelays[RelayName(turnURL)]
			return isFailed
		})
		if len(urls) == 0 {
			continue
		}
		for _, turnURL := range urls {
			if name := RelayName(turnURL); name != "" && !slices.Contains(asked, name) {
				asked = append(asked, name)
			}
		}
		each.URLs = urls
		kept = append(kept, each)
	}
	return kept, asked
}

// takeBackLocked takes back each failed relay its discovery heard announce itself since it failed,
// or, when the discovery says nothing of it, relayLeftOutFor after it failed. The server's mutex is
// held.
func (server *Server) takeBackLocked() {
	if len(server.failedRelays) == 0 {
		return
	}
	var announced func(string) (time.Time, bool)
	if source := server.relayAnnounced.Load(); source != nil {
		announced = *source
	}
	now := server.clock.Now()
	for name, failedAt := range server.failedRelays {
		var lastAnnounced time.Time
		isKnown := false
		if announced != nil {
			lastAnnounced, isKnown = announced(name)
		}
		switch {
		case isKnown && lastAnnounced.After(failedAt):
		case !isKnown && now.Sub(failedAt) >= relayLeftOutFor:
		default:
			continue
		}
		delete(server.failedRelays, name)
		server.logger.Info("webrtc: a relay is offered again", slog.String(slogx.EventKey, "relayTakenBack"),
			slog.String("relay", name), slog.Bool("announced", isKnown))
	}
}

// markFailed marks each relay asked as failed, unless already, logs each once and reports it.
func (server *Server) markFailed(ctx context.Context, asked []string) {
	server.mutex.Lock()
	now := server.clock.Now()
	if server.failedRelays == nil {
		server.failedRelays = map[string]time.Time{}
	}
	var marked []string
	for _, name := range asked {
		if _, isFailed := server.failedRelays[name]; !isFailed {
			server.failedRelays[name] = now
			marked = append(marked, name)
		}
	}
	server.mutex.Unlock()
	report := server.relayReport.Load()
	for _, name := range marked {
		server.logger.WarnContext(ctx, "webrtc: a relay gave no address in time, left out until it announces again",
			slog.String(slogx.EventKey, "relayFailed"), slog.String("relay", name), slog.Duration("waited", relayWait))
		if report != nil {
			(*report)(ctx, RelayFailure{Name: name, At: now})
		}
	}
}

// gathering is what an answer through a relay has gathered so far.
type gathering struct {
	candidates atomic.Int64
	// relayed is closed gatherAfterReflexive after the first relayed candidate.
	relayed    chan struct{}
	isRelayed  atomic.Bool
	relayingAt sync.Once
}

// heard counts a candidate the connection gathered.
func (gathered *gathering) heard(candidate *webrtc.ICECandidate) {
	if candidate == nil {
		return
	}
	gathered.candidates.Add(1)
	if candidate.Typ == webrtc.ICECandidateTypeRelay {
		gathered.isRelayed.Store(true)
		gathered.relayingAt.Do(func() {
			time.AfterFunc(gatherAfterReflexive, func() { close(gathered.relayed) })
		})
	}
}

// errNoCandidate is an answer through a relay with no candidate at all once relayWait is over.
var errNoCandidate = errors.New("gather the core's candidates: none within the relay's wait")

// awaitRelay waits for the core's candidates through the relays asked: until they are gathered,
// gatherAfterReflexive after the first relayed one, or relayWait. Past relayWait with no relayed
// candidate it marks every relay asked failed and answers with what it has, failing only with no
// candidate at all (D567 §3). A context ended first judges no relay.
func (server *Server) awaitRelay(
	ctx context.Context, gathered <-chan struct{}, heard *gathering, asked []string,
) error {
	server.mutex.Lock()
	wait := server.clock.NewTimer(relayWait)
	server.mutex.Unlock()
	defer wait.Stop()
	select {
	case <-gathered:
		return nil
	case <-heard.relayed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("gather the core's candidates: %w", ctx.Err())
	case <-wait.C():
	}
	if !heard.isRelayed.Load() {
		server.markFailed(ctx, asked)
	}
	if heard.candidates.Load() == 0 {
		return errNoCandidate
	}
	return nil
}
