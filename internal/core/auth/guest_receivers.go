// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
)

// grantScopeGuestReceiver is the session of a guest receiver (D465 §1, §2): a Google Cast device
// that is no device of the person's, signed in for one cast. Its principal carries no capability
// and is refused wherever a person signs in (IsGuestReceiver); only its cast's routes take it.
const grantScopeGuestReceiver = "guestReceiver"

// guestReceiverPlatform is a guest receiver's device's platform: every guest receiver is a Google
// Cast device running the receiver, as a paired one is (D288).
const guestReceiverPlatform = "chromecast"

// guestGrantLife is how long a guest receiver's grant waits to be traded (google-cast.md §2, step
// 1): the sender hands it over the Cast channel at once, and the receiver trades it at once.
const guestGrantLife = time.Minute

// guestGrantsPerDevice is how many grants one sending device has waiting at most; another forgets
// its oldest, so a page asking again and again keeps no more.
const guestGrantsPerDevice = 4

// ErrNoGuestGrant is a guest receiver's grant that is unknown, traded already, or expired: the three
// are not told apart, as a listener on the network who heard one should learn nothing (D135).
var ErrNoGuestGrant = errors.New("no such cast grant: it was used, or it expired")

// ErrGuestSender is a guest grant asked for by a session that cannot send one: a guest receiver's,
// or one of no device.
var ErrGuestSender = errors.New("this sign-in cannot cast to a Google Cast device")

// guestGrants are the grants waiting to be traded, by the hex of their secret's SHA-256: the
// secret itself is kept nowhere (D154 §1).
type guestGrants struct {
	mutex  sync.Mutex
	byHash map[string]guestGrant
}

// guestGrant is what a grant makes when traded: a guest receiver of the person's, cast to by the
// sending device alone, under the Cast device's name; and, while it waits, the key the Cast device
// proves its first offers under (proof.GuestGrantOfferKey), taken under the sending session's locator.
type guestGrant struct {
	userID         int64
	senderDeviceID int64
	// senderSession is the sending session's token's SHA-256, which its locator finds (sessionOfLocator).
	senderSession []byte
	offerKey      []byte
	receiverName  string
	madeAt        time.Time
}

// forgetLocked forgets every grant expired at now, and the oldest of senderDeviceID's while it has as
// many as it may; the mutex is held.
func (kept *guestGrants) forgetLocked(now time.Time, senderDeviceID int64) {
	oldest, count := "", 0
	for hash, waiting := range kept.byHash {
		if now.Sub(waiting.madeAt) >= guestGrantLife {
			delete(kept.byHash, hash)
			continue
		}
		if waiting.senderDeviceID != senderDeviceID {
			continue
		}
		count++
		if oldest == "" || waiting.madeAt.Before(kept.byHash[oldest].madeAt) {
			oldest = hash
		}
	}
	if count >= guestGrantsPerDevice {
		delete(kept.byHash, oldest)
	}
}

// GuestReceiver is a guest grant traded: the receiver's session, and the device it signed in as,
// its person's, named Name, which only SenderDeviceID casts to.
type GuestReceiver struct {
	Issued
	UserID         int64
	DeviceID       int64
	SenderDeviceID int64
	Name           string
}

// IssueGuestGrant mints a grant for a Google Cast device, named receiverName, to receive a cast from
// the sender's device (D465 §2): random, kept only as its SHA-256, traded once within
// guestGrantLife. ErrGuestSender for a guest receiver's session or one of no device.
func (service *Service) IssueGuestGrant(sender Principal, receiverName string) (string, error) {
	senderSession, err := hex.DecodeString(sender.SessionKey)
	if sender.isGuestReceiver || sender.DeviceID == 0 || err != nil || len(senderSession) == 0 {
		return "", ErrGuestSender
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	now := service.clock.Now()
	service.guestGrants.mutex.Lock()
	defer service.guestGrants.mutex.Unlock()
	service.guestGrants.forgetLocked(now, sender.DeviceID)
	service.guestGrants.byHash[hex.EncodeToString(tokenHash(secret))] = guestGrant{
		userID: sender.UserID, senderDeviceID: sender.DeviceID, senderSession: senderSession,
		offerKey: proof.GuestGrantOfferKey(secret), receiverName: receiverName, madeAt: now,
	}
	return secret, nil
}

// GuestGrantOfferKeys are the keys a Google Cast device holding a waiting grant proves its first
// offers under (proof.GuestGrantOfferKey), passed on by the discovery under the locator of the
// session that asked for the grant, which the sender handed the device with it: so a receiver served
// at the loader's address, reached over WebRTC alone, can make the trade (google-cast.md §2, step 2).
// None once the grant is traded or expired; none under any other locator.
func (service *Service) GuestGrantOfferKeys(ctx context.Context, locatorHash string) ([][]byte, error) {
	now := service.clock.Now()
	service.guestGrants.mutex.Lock()
	waiting := []guestGrant{}
	for _, each := range service.guestGrants.byHash {
		if now.Sub(each.madeAt) < guestGrantLife {
			waiting = append(waiting, each)
		}
	}
	service.guestGrants.mutex.Unlock()
	if len(waiting) == 0 {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(locatorHash)
	if err != nil {
		return nil, ErrNoOfferKey
	}
	sender, err := service.sessionOfLocator(ctx, raw)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keys := [][]byte{}
	for _, each := range waiting {
		if bytes.Equal(each.senderSession, sender) {
			keys = append(keys, each.offerKey)
		}
	}
	return keys, nil
}

// TradeGuestGrant trades a guest grant, once, for a session of a guest receiver of the grant's
// person (D465 §2): a device of its own, never listed among theirs (ListLiveDevicesOfUser), whose
// session reaches its cast alone. The grant is gone whatever the trade's outcome. ErrNoGuestGrant
// for one unknown, traded or expired; ErrNoUser for a person removed since.
func (service *Service) TradeGuestGrant(ctx context.Context, grant string) (GuestReceiver, error) {
	now := service.clock.Now()
	hash := hex.EncodeToString(tokenHash(grant))
	service.guestGrants.mutex.Lock()
	waiting, found := service.guestGrants.byHash[hash]
	delete(service.guestGrants.byHash, hash)
	service.guestGrants.mutex.Unlock()
	if !found || now.Sub(waiting.madeAt) >= guestGrantLife {
		return GuestReceiver{}, ErrNoGuestGrant
	}
	if _, err := liveUser(ctx, service.database.Read(), waiting.userID); err != nil {
		return GuestReceiver{}, err
	}
	issued, err := service.issueSession(ctx, waiting.userID, NewDevice{
		Name: waiting.receiverName, Platform: guestReceiverPlatform,
	}, grantScopeGuestReceiver)
	if err != nil {
		return GuestReceiver{}, err
	}
	return GuestReceiver{
		Issued: issued, UserID: waiting.userID, DeviceID: issued.DeviceID, SenderDeviceID: waiting.senderDeviceID,
		Name: waiting.receiverName,
	}, nil
}

// RemoveGuestReceiver removes a guest receiver's device as its cast ends (D465 §1), its session and
// device secret ended with it, as removing any device does (RemoveDevice): it keeps nothing.
func (service *Service) RemoveGuestReceiver(ctx context.Context, userID, deviceID int64) error {
	return service.RemoveDevice(ctx, userID, deviceID)
}

// RemoveEveryGuestReceiver removes every guest receiver's device still kept but those isLive names,
// as the core starts: the casts that made them were in memory, and ended with the core that held
// them; a guest receiver signed in since is live.
func (service *Service) RemoveEveryGuestReceiver(ctx context.Context, isLive func(userID, deviceID int64) bool) error {
	kept, err := service.database.Read().ListLiveDevicesOfSessionScope(ctx, grantScopeGuestReceiver)
	if err != nil {
		return fmt.Errorf("list the guest receivers kept: %w", err)
	}
	for _, each := range kept {
		userID := ptr.ValueOrZero(each.UserID)
		if isLive(userID, each.ID) {
			continue
		}
		err := service.RemoveDevice(ctx, userID, each.ID)
		if err != nil && !errors.Is(err, ErrNoDevice) {
			return fmt.Errorf("remove guest receiver %d: %w", each.ID, err)
		}
	}
	return nil
}
