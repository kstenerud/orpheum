// SPDX-License-Identifier: AGPL-3.0-or-later

package relaycred

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// AnnouncementHeader is the header an announcement's signature travels in: the standard base64,
// padded, of HMAC-SHA256 under the discovery's relay secret over the body's bytes (D567 §1).
const AnnouncementHeader = "Orpheum-Relay-Signature"

// AnnouncementSkew is how far an announcement's time may be from the discovery's clock, either
// way, before the discovery refuses it: a replayed one is stale within it.
const AnnouncementSkew = 2 * time.Minute

// SelfTestHousehold is the household a relay's own credential names for the allocation it makes on
// its public listeners before each announcement. A discovery's household names are 22 characters,
// so none is this one; the relay records nothing of it and counts it against no limit.
const SelfTestHousehold = "relay-self-test"

// Announcement is the body a relay POSTs to a discovery each minute an allocation on its public
// listeners works (D567 §1): its name, as the discovery names it in its TURN URLs, and when, in
// Unix milliseconds.
type Announcement struct {
	Name string `json:"name"`
	At   int64  `json:"at"`
}

// ErrNotAnAnnouncement is a body that is not a relay's announcement, a signature that does not
// match it, or a time too far from the discovery's.
var ErrNotAnAnnouncement = errors.New("not a relay announcement")

// SignAnnouncement is the body of an announcement that relay name works at at, and its signature
// under secret.
func SignAnnouncement(secret Secret, name string, at time.Time) (body []byte, signature string, err error) {
	if name == "" {
		return nil, "", fmt.Errorf("%w: no name", ErrNotAnAnnouncement)
	}
	body, err = json.Marshal(Announcement{Name: name, At: at.UnixMilli()})
	if err != nil {
		return nil, "", fmt.Errorf("an announcement: %w", err)
	}
	return body, announcementSignature(secret, body), nil
}

// ReadAnnouncement is the announcement body says, when signature is its signature under secret and
// its time is within AnnouncementSkew of now.
func ReadAnnouncement(secret Secret, body []byte, signature string, now time.Time) (Announcement, error) {
	if !hmac.Equal([]byte(signature), []byte(announcementSignature(secret, body))) {
		return Announcement{}, fmt.Errorf("%w: the signature does not match", ErrNotAnAnnouncement)
	}
	var announcement Announcement
	if err := strictjson.Unmarshal(body, &announcement); err != nil {
		return Announcement{}, fmt.Errorf("%w: %w", ErrNotAnAnnouncement, err)
	}
	if announcement.Name == "" {
		return Announcement{}, fmt.Errorf("%w: no name", ErrNotAnAnnouncement)
	}
	if off := now.Sub(time.UnixMilli(announcement.At)).Abs(); off > AnnouncementSkew {
		return Announcement{}, fmt.Errorf("%w: its time is %s from the discovery's", ErrNotAnAnnouncement, off)
	}
	return announcement, nil
}

func announcementSignature(secret Secret, body []byte) string {
	mac := hmac.New(sha256.New, secret.key)
	_, _ = mac.Write(body) // a hash.Hash's Write never returns an error
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
