// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// credentialLifetime is how long voice's relay credentials are good for: a day, longer than any
// evening's film, and a membership that ends first ends them with it (D243 §8).
const credentialLifetime = 24 * time.Hour

// secretBytes is the TURN shared secret's length before encoding: 256 random bits.
const secretBytes = 32

// Secrets is where the TURN shared secret is kept, sealed (D194).
type Secrets interface {
	Open(ctx context.Context, name secrets.Name) (secrets.Value, error)
	Set(ctx context.Context, name secrets.Name, value secrets.Value, setByUserID *int64) error
}

// EnsureSecret makes the TURN shared secret when there is none, or none this server can open, as
// after a restore without secret.key: we generate it, and making a new one costs only the relay
// credentials already given, which a client asks for again (D194).
func EnsureSecret(ctx context.Context, store Secrets, random io.Reader) error {
	_, err := store.Open(ctx, secrets.TURNSecret)
	if err == nil {
		return nil
	}
	if !errors.Is(err, secrets.ErrNotSet) && !errors.Is(err, secrets.ErrNeedsReentry) {
		return fmt.Errorf("read the TURN secret: %w", err)
	}
	made, err := randbytes.Base64URLSecret(random, secretBytes)
	if err != nil {
		return fmt.Errorf("make the TURN secret: %w", err)
	}
	if err = store.Set(ctx, secrets.TURNSecret, secrets.NewValue(made), nil); err != nil {
		return fmt.Errorf("keep the TURN secret: %w", err)
	}
	return nil
}

// Membership names whose relay credentials they are: one member of one Watch Together session.
type Membership struct {
	SessionID int64
	MemberID  int64
}

// Credentials are one membership's relay credentials, as a browser's RTCIceServer takes them.
type Credentials struct {
	Username  string
	Password  string
	ExpiresAt time.Time
}

// Issue makes membership's relay credentials, good until now plus credentialLifetime and while the
// membership lasts: the username says whose they are and until when, and the password is derived
// from the shared secret, so the relay checks them without keeping them (the TURN REST form).
func Issue(secret secrets.Value, membership Membership, now time.Time) Credentials {
	expiresAt := now.Add(credentialLifetime)
	username := fmt.Sprintf("%d:%d.%d", expiresAt.Unix(), membership.SessionID, membership.MemberID)
	return Credentials{Username: username, Password: password(secret, username), ExpiresAt: expiresAt}
}

// foundName is what a username names for a device its core's discovery found the core for (the
// discovery protocol's §2.4): it holds no membership, so the credential is good until it expires,
// and the relay still relays only to the public internet (relay.go's mayRelayTo).
const foundName = "found"

// IssueFound makes the relay credentials a core sends its discovery with each heartbeat, which the
// discovery hands the devices it finds the core for, so WebRTC through the discovery has a relay
// where no direct path works (the protocol's §2.4, the TURN REST form, C14). They last
// credentialLifetime, as a member's do, so an evening's film is not cut when they would end [I].
func IssueFound(secret secrets.Value, now time.Time) Credentials {
	expiresAt := now.Add(credentialLifetime)
	username := fmt.Sprintf("%d:%s", expiresAt.Unix(), foundName)
	return Credentials{Username: username, Password: password(secret, username), ExpiresAt: expiresAt}
}

// password is username's password under secret.
func password(secret secrets.Value, username string) string {
	mac := hmac.New(sha256.New, []byte(secret.Reveal()))
	mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// errMalformed is a username the relay did not issue.
var errMalformed = errors.New("not a relay username")

// holder is whose a username is: a membership, or a device the discovery found the core for.
type holder struct {
	membership Membership
	isFound    bool
}

// parseUsername reads whose a username is, and until when.
func parseUsername(username string) (holder, time.Time, error) {
	expiry, names, found := strings.Cut(username, ":")
	if !found {
		return holder{}, time.Time{}, errMalformed
	}
	expiresAt, expiryErr := strconv.ParseInt(expiry, 10, 64)
	if expiryErr != nil {
		return holder{}, time.Time{}, errMalformed
	}
	if names == foundName {
		return holder{isFound: true}, time.Unix(expiresAt, 0), nil
	}
	sessionText, memberText, found := strings.Cut(names, ".")
	if !found {
		return holder{}, time.Time{}, errMalformed
	}
	sessionID, sessionErr := strconv.ParseInt(sessionText, 10, 64)
	memberID, memberErr := strconv.ParseInt(memberText, 10, 64)
	if err := errors.Join(sessionErr, memberErr); err != nil {
		return holder{}, time.Time{}, errMalformed
	}
	return holder{membership: Membership{SessionID: sessionID, MemberID: memberID}}, time.Unix(expiresAt, 0), nil
}

// Issuer hands members their relay credentials, as the browsers' ICE servers: where the relay is
// reached, over UDP and TCP, and the credentials Issue makes (D243 §8). A server with no relay has
// none to hand out, and its browsers try only to reach each other directly.
type Issuer struct {
	// Address is the relay's public host:port: turn.public_address and turn.listen's port.
	Address string
	Secrets Secrets
}

// ICEServer is one of a browser's ICE servers (its RTCIceServer): where, and the credentials.
type ICEServer struct {
	URLs        []string
	Credentials Credentials
}

// ICEServersFor is membership's ICE servers now; none when the server relays nothing.
func (issuer *Issuer) ICEServersFor(ctx context.Context, membership Membership, now time.Time) ([]ICEServer, error) {
	return issuer.iceServers(ctx, func(secret secrets.Value) Credentials { return Issue(secret, membership, now) })
}

// ICEServersForFound is the ICE servers a core sends its discovery for the devices it finds the core
// for (IssueFound); none when the server relays nothing.
func (issuer *Issuer) ICEServersForFound(ctx context.Context, now time.Time) ([]ICEServer, error) {
	return issuer.iceServers(ctx, func(secret secrets.Value) Credentials { return IssueFound(secret, now) })
}

// iceServers is the relay's ICE servers, over UDP and TCP, with the credentials issue makes.
func (issuer *Issuer) iceServers(
	ctx context.Context, issue func(secret secrets.Value) Credentials,
) ([]ICEServer, error) {
	if issuer == nil || issuer.Address == "" {
		return nil, nil
	}
	secret, err := issuer.Secrets.Open(ctx, secrets.TURNSecret)
	if err != nil {
		return nil, fmt.Errorf("read the TURN secret: %w", err)
	}
	return []ICEServer{{
		URLs:        []string{"turn:" + issuer.Address + "?transport=udp", "turn:" + issuer.Address + "?transport=tcp"},
		Credentials: issue(secret),
	}}, nil
}
