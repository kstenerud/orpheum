// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// signedCall makes a call with the session's token, renewing the session at once, and only once, when the
// discovery refuses its token, rather than failing and waiting a retry's length (the protocol's
// §2.3; C11). A second refusal is the call's failure.
func (service *Service) signedCall(ctx context.Context, call callOf) error {
	token, err := service.session(ctx)
	if err != nil {
		return err
	}
	call.token = token
	err = service.call(ctx, call)
	if !errors.Is(err, errUnauthorised) {
		return err
	}
	service.dropToken(token)
	if call.token, err = service.session(ctx); err != nil {
		return err
	}
	return service.call(ctx, call)
}

// dropToken forgets token, the discovery having refused it, unless another caller has renewed it.
func (service *Service) dropToken(token string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.token == token {
		service.token = ""
	}
}

// session is a bearer token for the discovery, opening a session when there is none or it ends soon:
// one at a time, every caller sharing it (renewal).
//
// A core that stands aside, refused as a copy, asks for none: it is refused at once with ErrAnotherCopy
// (D437 §3), whichever loop or call asks.
func (service *Service) session(ctx context.Context) (string, error) {
	if !service.Status().StoodAsideAt.IsZero() {
		return "", ErrAnotherCopy
	}
	if token, isLive := service.liveToken(); isLive {
		return token, nil
	}
	return service.renewal.do(ctx, service.openSession)
}

// liveToken is the session's token, while it does not end within sessionMargin.
func (service *Service) liveToken() (string, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	isLive := service.token != "" && service.parts.Clock.Now().Add(sessionMargin).Before(service.tokenExpiresAt)
	return service.token, isLive
}

// openSession opens a session, unless the renewal a caller waited for left one live.
func (service *Service) openSession(ctx context.Context) (string, error) {
	if token, isLive := service.liveToken(); isLive {
		return token, nil
	}
	status := service.Status()
	// The session's life counts from before the asking, so a slow answer only shortens it.
	asked := service.parts.Clock.Now()
	nonce, err := service.challenge(ctx, status.BaseURL)
	if err != nil {
		return "", err
	}
	var opened discoveryproto.SessionResponse
	err = service.call(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathSession,
		body: discoveryproto.SessionRequest{
			CoreID: status.CoreID, InstanceID: service.instanceID, InstallationID: service.parts.InstallationID,
			Nonce:     nonce,
			Signature: service.parts.Key.Sign(discoveryproto.PurposeSession, status.Identity, status.CoreID, nonce),
		}, into: &opened,
	})
	if refused, isRefused := errors.AsType[*refusal](err); isRefused &&
		refused.code == discoveryproto.RefusalAnotherInstance {
		service.standAside(ctx)
	}
	if err != nil {
		return "", err
	}
	service.mutex.Lock()
	service.token, service.tokenExpiresAt = opened.Token, asked.Add(time.Duration(opened.ExpiresInSeconds)*time.Second)
	service.mutex.Unlock()
	return opened.Token, nil
}

// challenge is a nonce from the discovery at baseURL, for the core's key to sign.
func (service *Service) challenge(ctx context.Context, baseURL string) (string, error) {
	var answered discoveryproto.ChallengeResponse
	err := service.call(ctx, callOf{
		base: baseURL, method: http.MethodPost, path: discoveryproto.PathChallenge,
		body: discoveryproto.ChallengeRequest{PublicKey: service.parts.Key.Public()}, into: &answered,
	})
	if err != nil {
		return "", fmt.Errorf("ask for a challenge: %w", err)
	}
	if answered.Nonce == "" {
		return "", errors.New("the discovery's challenge has no nonce")
	}
	return answered.Nonce, nil
}

// errUnauthorised is a discovery refusing the core's session: a new one is opened.
var errUnauthorised = errors.New("the discovery refused the core's session")

// refusal is a refusal the discovery names, which the core acts on (discoveryproto.Refusal), and the
// wait it names before asking again, on a discoveryproto.RefusalHolderAway.
type refusal struct {
	code, message string
	retryAfter    time.Duration
}

func (refused *refusal) Error() string {
	return "the discovery refused: " + refused.message
}

// callOf is one call to a discovery.
type callOf struct {
	base, method, path, token string
	body                      any
	into                      any
}

// call makes one call, carrying the protocol's version and the session's token, and reads its answer
// into call.into; a deprecation the discovery names is kept for the admin.
func (service *Service) call(ctx context.Context, call callOf) error {
	var body io.Reader
	if call.body != nil {
		encoded, err := json.Marshal(call.body)
		if err != nil {
			return fmt.Errorf("encode the request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, call.method, call.base+call.path, body)
	if err != nil {
		return fmt.Errorf("make the request: %w", err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	if call.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if call.token != "" {
		request.Header.Set("Authorization", "Bearer "+call.token)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return &unreachedError{err: err}
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	if deprecated := response.Header.Get(discoveryproto.DeprecatedHeader); deprecated != "" {
		service.mutex.Lock()
		service.status.DeprecatedUntil = deprecated
		service.mutex.Unlock()
	}
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("read the discovery's answer: %w", err)
	}
	switch {
	case response.StatusCode < 200 || response.StatusCode > 299:
		return failure(response, answer)
	case call.into == nil:
		return nil
	}
	if err = strictjson.Unmarshal(answer, call.into); err != nil {
		return fmt.Errorf("the discovery's answer is not what it should be: %w", err)
	}
	return nil
}

// failure is the error a refused call's answer stands for: a refused session, a refusal the
// discovery names, or the status and what it said.
func failure(response *http.Response, answer []byte) error {
	if response.StatusCode == http.StatusUnauthorized {
		return errUnauthorised
	}
	var named discoveryproto.Refusal
	if strictjson.Unmarshal(answer, &named) == nil && named.Code != "" {
		return &refusal{
			code: named.Code, message: named.Message, retryAfter: time.Duration(named.RetryAfterSeconds) * time.Second,
		}
	}
	return &statusError{
		code: response.StatusCode, message: fmt.Sprintf("the discovery answered %s: %s", response.Status,
			bytes.TrimSpace(answer)),
	}
}

// statusError is a call the discovery answered with a failing status it names no refusal for.
type statusError struct {
	code    int
	message string
}

func (answered *statusError) Error() string {
	return answered.message
}

// unreachedError is a call that reached no discovery: the network, or the discovery's host, is down.
type unreachedError struct {
	err error
}

func (unreached *unreachedError) Error() string {
	return "reach the discovery: " + unreached.err.Error()
}

func (unreached *unreachedError) Unwrap() error {
	return unreached.err
}

// isUnreached is whether err is a call that reached no discovery: none answered, or a proxy in front
// of it said it could not reach it. The step loop then looks for the network to come back (D437 §5).
func isUnreached(err error) bool {
	if unreached, isUnreached := errors.AsType[*unreachedError](err); isUnreached && unreached != nil {
		return true
	}
	answered, isStatus := errors.AsType[*statusError](err)
	return isStatus && isProxyUnreached(answered.code)
}
