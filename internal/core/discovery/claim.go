// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

var (
	// ErrWrongKey is a key the discovery did not take: mistyped, used, expired, or tried too often.
	ErrWrongKey = errors.New("that key was not accepted: check it, or make a new one on your Orpheum account")
	// ErrNotPending is a key entered for a core that is not waiting to be claimed.
	ErrNotPending = errors.New("this server is not waiting to be claimed")
)

// Claim sends the key the household's account made, which its admin typed, to the discovery, which
// claims the core for the account (the protocol's §2.2; D310 §5, §8), and returns how the core then
// stands.
func (service *Service) Claim(ctx context.Context, key string) (Status, error) {
	status := service.Status()
	if status.BaseURL == "" || status.State != discoveryproto.StatePending {
		return status, ErrNotPending
	}
	var claimed discoveryproto.ClaimResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathClaim,
		body: discoveryproto.ClaimRequest{Key: strings.TrimSpace(key)}, into: &claimed,
	})
	var refused *refusal
	if errors.As(err, &refused) && refused.code == discoveryproto.RefusalWrongKey {
		return service.Status(), ErrWrongKey
	}
	if err != nil {
		return service.Status(), err
	}
	service.parts.Logger.InfoContext(ctx, "discovery: the core was claimed for an account by its key")
	service.askStatus(ctx, status)
	service.nudge()
	return service.Status(), nil
}
