// SPDX-License-Identifier: GPL-3.0-or-later

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

// password is username's password under secret.
func password(secret secrets.Value, username string) string {
	mac := hmac.New(sha256.New, []byte(secret.Reveal()))
	mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// errMalformed is a username the relay did not issue.
var errMalformed = errors.New("not a relay username")

// parseUsername reads whose a username is, and until when.
func parseUsername(username string) (Membership, time.Time, error) {
	expiry, names, found := strings.Cut(username, ":")
	if !found {
		return Membership{}, time.Time{}, errMalformed
	}
	sessionText, memberText, found := strings.Cut(names, ".")
	if !found {
		return Membership{}, time.Time{}, errMalformed
	}
	expiresAt, expiryErr := strconv.ParseInt(expiry, 10, 64)
	sessionID, sessionErr := strconv.ParseInt(sessionText, 10, 64)
	memberID, memberErr := strconv.ParseInt(memberText, 10, 64)
	if err := errors.Join(expiryErr, sessionErr, memberErr); err != nil {
		return Membership{}, time.Time{}, errMalformed
	}
	return Membership{SessionID: sessionID, MemberID: memberID}, time.Unix(expiresAt, 0), nil
}

// Issuer hands members their relay credentials, as the browsers' ICE servers: where the relay is
// reached, over UDP and TCP, and the credentials Issue makes (D243 §8). A server with no relay has
// none to hand out, and its browsers try only to reach each other directly.
type Issuer struct {
	// Address is the relay's public host:port: turn.public_address and turn.listen's port.
	Address string
	Secrets Secrets
}

// ICE is a browser's RTCIceServer: where, and the credentials.
type ICE struct {
	URLs        []string
	Credentials Credentials
}

// ICEFor is membership's ICE servers now; none when the server relays nothing.
func (issuer *Issuer) ICEFor(ctx context.Context, membership Membership, now time.Time) ([]ICE, error) {
	if issuer == nil || issuer.Address == "" {
		return nil, nil
	}
	secret, err := issuer.Secrets.Open(ctx, secrets.TURNSecret)
	if err != nil {
		return nil, fmt.Errorf("read the TURN secret: %w", err)
	}
	return []ICE{{
		URLs:        []string{"turn:" + issuer.Address + "?transport=udp", "turn:" + issuer.Address + "?transport=tcp"},
		Credentials: Issue(secret, membership, now),
	}}, nil
}
