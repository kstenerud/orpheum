// SPDX-License-Identifier: AGPL-3.0-or-later

package relaycred

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // HMAC-SHA1 is the TURN REST form every TURN client and server speaks
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Realm is the relay's TURN realm, which a client hashes its credentials with.
const Realm = "orpheum-relay"

// Role is which end of a household's connection a credential is for: its core or one of its
// clients. The relay relays only between the two (D558 §4).
type Role string

const (
	// RoleCore is a household's core's credential: an hour, made at each heartbeat's answer.
	RoleCore Role = "core"
	// RoleClient is a device's credential, made in each `found` answer: a quarter of an hour.
	RoleClient Role = "client"
)

// MaxKbps is the highest rate a credential may name, a gigabit.
const MaxKbps = 1_000_000

// Credential is what a relay username says: until when it admits a new allocation, whose end it
// is, the household's opaque name and its rate.
type Credential struct {
	// ExpiresAt is the Unix second after which no new allocation is admitted.
	ExpiresAt time.Time
	Role      Role
	// Household is the discovery's opaque name for a household, which names no account and no core.
	Household string
	// Kbps is the household's rate toward its clients, in kbit/s.
	Kbps int
}

// household is a household's name: 8 to 32 of [A-Za-z0-9_-].
var household = regexp.MustCompile(`^[A-Za-z0-9_-]{8,32}$`)

// ErrNotACredential is a username that is not a relay credential's.
var ErrNotACredential = errors.New("not a relay credential")

// Username is the credential's TURN username, or why it cannot be written.
func (credential Credential) Username() (string, error) {
	if err := credential.check(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%s:%s:%d",
		credential.ExpiresAt.Unix(), credential.Role, credential.Household, credential.Kbps), nil
}

// check is why the credential cannot be written, or nil.
func (credential Credential) check() error {
	switch {
	case credential.ExpiresAt.Unix() <= 0:
		return fmt.Errorf("%w: an expiry before 1970", ErrNotACredential)
	case credential.Role != RoleCore && credential.Role != RoleClient:
		return fmt.Errorf("%w: the role %q", ErrNotACredential, credential.Role)
	case !household.MatchString(credential.Household):
		return fmt.Errorf("%w: a household not 8 to 32 of [A-Za-z0-9_-]", ErrNotACredential)
	case credential.Kbps < 1 || credential.Kbps > MaxKbps:
		return fmt.Errorf("%w: the rate %d kbit/s, not 1 to %d", ErrNotACredential, credential.Kbps, MaxKbps)
	}
	return nil
}

// Parse reads a username Username wrote. Anything else is refused, a number written any other way
// than Username writes it too, so one credential has one username.
func Parse(username string) (Credential, error) {
	parts := strings.Split(username, ":")
	if len(parts) != 4 {
		return Credential{}, ErrNotACredential
	}
	expiry, expiryErr := canonicalNumber(parts[0])
	kbps, kbpsErr := canonicalNumber(parts[3])
	if expiryErr != nil || kbpsErr != nil || kbps > MaxKbps {
		return Credential{}, ErrNotACredential
	}
	credential := Credential{
		ExpiresAt: time.Unix(expiry, 0).UTC(), Role: Role(parts[1]), Household: parts[2], Kbps: int(kbps),
	}
	if err := credential.check(); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

// canonicalNumber reads a positive decimal number written without a sign or a leading zero.
func canonicalNumber(text string) (int64, error) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number <= 0 || strconv.FormatInt(number, 10) != text {
		return 0, ErrNotACredential
	}
	return number, nil
}

// Make is the credential's username and its password under secret.
func Make(secret Secret, credential Credential) (username, password string, err error) {
	username, err = credential.Username()
	if err != nil {
		return "", "", err
	}
	return username, Password(secret, username), nil
}

// Password is username's password under secret: the standard base64, padded, of HMAC-SHA1 over the
// username's bytes.
func Password(secret Secret, username string) string {
	mac := hmac.New(sha1.New, secret.key)
	_, _ = mac.Write([]byte(username)) // a hash.Hash's Write never returns an error
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Matches is whether password is username's under secret, compared in constant time.
func Matches(secret Secret, username, password string) bool {
	return subtle.ConstantTimeCompare([]byte(Password(secret, username)), []byte(password)) == 1
}

// MinSecretBytes is a secret's least length.
const MinSecretBytes = 32

// redacted is what every printing, logging or serialising method of Secret emits.
const redacted = "[redacted]"

// Secret is a shared secret credentials are made under. Nothing that prints, logs or serialises it
// shows it, and no method returns it: it is used only to make passwords, here (rule 12).
type Secret struct {
	key []byte
}

// NewSecret is key as a secret, refused when shorter than MinSecretBytes.
func NewSecret(key []byte) (Secret, error) {
	if len(key) < MinSecretBytes {
		return Secret{}, fmt.Errorf("a relay secret of %d bytes; it needs at least %d", len(key), MinSecretBytes)
	}
	return Secret{key: append([]byte(nil), key...)}, nil
}

// String returns "[redacted]".
func (Secret) String() string { return redacted }

// GoString returns "[redacted]", so %#v cannot show the field.
func (Secret) GoString() string { return redacted }

// Format writes "[redacted]" for every verb, so no fmt verb reaches the field.
func (Secret) Format(state fmt.State, _ rune) {
	// fmt.Formatter has no way to report a failed write, and fmt ignores it too.
	fmt.Fprint(state, redacted) //nolint:errcheck // Format cannot return an error; see above
}

// MarshalJSON returns the JSON string "[redacted]".
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// MarshalText returns "[redacted]".
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// LogValue logs as "[redacted]".
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }
