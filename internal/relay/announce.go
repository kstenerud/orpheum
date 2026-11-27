// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// announceEvery is how often the relay tests itself and announces to each discovery it serves: a
// discovery names it while it announced within three minutes (D567 §1–2).
const announceEvery = time.Minute

// selfTestFor is how long the relay's own credential lasts: one allocation's making.
const selfTestFor = time.Minute

// maxAnswerBytes is how much of a discovery's answer to an announcement is read.
const maxAnswerBytes = 4096

// Announce is a discovery the relay announces to (D567 §1): the HTTPS URL it POSTs to, and the
// secret, one of Config.Secrets, it shares with that discovery, which signs the announcement and
// makes the credential of the self-test before it.
type Announce struct {
	URL    string
	Secret relaycred.Secret
}

// check refuses an announcement's URL that is not an absolute HTTPS one.
func (announce Announce) check() error {
	parsed, err := url.Parse(announce.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("the relay announces to %q, which is no https URL", announce.URL)
	}
	return nil
}

// A target's state, as its log lines name it: each is logged once, when it changes.
const (
	stateSelfTestFailed = "relaySelfTestFailed"
	stateAnnounceFailed = "relayAnnounceFailed"
	stateAnnounced      = "relayAnnounced"
)

// announcer tests the relay on its public listener and announces it to each discovery, every
// announceEvery until its context ends.
type announcer struct {
	name    string
	target  *net.UDPAddr
	targets []Announce
	parts   Parts
	// states is, for each target, its state last logged: "" before its first round.
	states []string
}

// keep announces at once and then every announceEvery, until ctx ends.
func (announcer *announcer) keep(ctx context.Context) {
	for {
		for index := range announcer.targets {
			announcer.round(ctx, index)
		}
		timer := announcer.parts.Clock.NewTimer(announceEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
	}
}

// round tests the relay under one discovery's secret and, when an allocation works, announces it
// to that discovery.
func (announcer *announcer) round(ctx context.Context, index int) {
	target := announcer.targets[index]
	if err := announcer.selfTest(ctx, target.Secret); err != nil {
		announcer.became(ctx, index, stateSelfTestFailed, slog.LevelError,
			"the relay's own allocation failed, so it announces nothing", err)
		return
	}
	if err := announcer.announce(ctx, target); err != nil {
		announcer.became(ctx, index, stateAnnounceFailed, slog.LevelError,
			"the relay could not announce itself to a discovery", err)
		return
	}
	announcer.became(ctx, index, stateAnnounced, slog.LevelInfo, "the relay works and announces itself", nil)
}

// became logs a target's state once, when it changes.
func (announcer *announcer) became(
	ctx context.Context, index int, state string, level slog.Level, message string, err error,
) {
	if announcer.states[index] == state || ctx.Err() != nil {
		return
	}
	announcer.states[index] = state
	attributes := []slog.Attr{
		slog.String(slogx.EventKey, state), slog.String("discovery", announcer.targets[index].URL),
	}
	if err != nil {
		attributes = append(attributes, slog.String("err", err.Error()))
	}
	// Each state's message is a literal at its caller.
	announcer.parts.Logger.LogAttrs(ctx, level, message, attributes...) //nolint:sloglint // see above
}

// selfTest makes an allocation on the relay's public UDP listener under secret, as a household's
// core would, and ends it: an announcement says an allocation works from outside, not that the
// process runs (D567 §1). The retransmissions of pion bound it, to about eight seconds a request.
func (announcer *announcer) selfTest(ctx context.Context, secret relaycred.Secret) error {
	username, password, err := relaycred.Make(secret, relaycred.Credential{
		ExpiresAt: announcer.parts.Clock.Now().Add(selfTestFor), Role: relaycred.RoleCore,
		Household: relaycred.SelfTestHousehold, Kbps: relaycred.MaxKbps,
	})
	if err != nil {
		return fmt.Errorf("the self-test's credential: %w", err)
	}
	packets, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return fmt.Errorf("the self-test's socket: %w", err)
	}
	defer func() { _ = packets.Close() }() //nolint:errcheck // the test's result is already known
	server := announcer.target.String()
	turnClient, err := pionturn.NewClient(&pionturn.ClientConfig{
		STUNServerAddr: server, TURNServerAddr: server, Conn: packets,
		Username: username, Password: password, Realm: relaycred.Realm,
	})
	if err != nil {
		return fmt.Errorf("the self-test's client: %w", err)
	}
	defer turnClient.Close()
	if err = turnClient.Listen(); err != nil {
		return fmt.Errorf("the self-test's client: %w", err)
	}
	relayed, err := turnClient.Allocate()
	if err != nil {
		return fmt.Errorf("an allocation on %s: %w", server, err)
	}
	if err = relayed.Close(); err != nil {
		return fmt.Errorf("end the self-test's allocation: %w", err)
	}
	return nil
}

// errRefused is a discovery's answer that is not a success.
var errRefused = errors.New("the discovery refused the announcement")

// announce POSTs one signed announcement to target.
func (announcer *announcer) announce(ctx context.Context, target Announce) error {
	body, signature, err := relaycred.SignAnnouncement(target.Secret, announcer.name, announcer.parts.Clock.Now())
	if err != nil {
		return fmt.Errorf("sign the announcement: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("the announcement's request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(relaycred.AnnouncementHeader, signature)
	response, err := announcer.parts.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("announce: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // the status is the answer
	// Drained so the connection is reused; the status is the answer.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxAnswerBytes)) //nolint:errcheck // see above
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", errRefused, response.Status)
	}
	return nil
}
