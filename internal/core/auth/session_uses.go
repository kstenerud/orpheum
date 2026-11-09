// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// sessionUses are the sessions used since their last use was written, in memory: a request that
// only reads never waits for the database's writer to record that it was made (D303 §4, DL10).
// FlushSessionUses writes them in one batch, every sessionLastUsedWriteIntervalSeconds and
// when the server stops; until then the idle check and a person's devices read the later of the
// stored and the remembered time.
type sessionUses struct {
	mutex sync.Mutex
	// latestBySession is each session's latest use not yet written, by its token hash in hex.
	latestBySession map[string]sessionUse
}

// sessionUse is one session's latest use: its token hash, its device (0 for none), and when.
type sessionUse struct {
	hash     []byte
	deviceID int64
	atMs     int64
}

// note remembers that the session hash, of device deviceID, was used at atMs.
func (uses *sessionUses) note(hash []byte, deviceID, atMs int64) {
	uses.mutex.Lock()
	defer uses.mutex.Unlock()
	key := hex.EncodeToString(hash)
	if uses.latestBySession == nil {
		uses.latestBySession = map[string]sessionUse{}
	}
	if known, ok := uses.latestBySession[key]; !ok || known.atMs < atMs {
		uses.latestBySession[key] = sessionUse{hash: hash, deviceID: deviceID, atMs: atMs}
	}
}

// latest is the session's latest use not yet written, if any.
func (uses *sessionUses) latest(hash []byte) (int64, bool) {
	uses.mutex.Lock()
	defer uses.mutex.Unlock()
	use, ok := uses.latestBySession[hex.EncodeToString(hash)]
	return use.atMs, ok
}

// latestOfDevice is the latest use not yet written of any session of the device, if any.
func (uses *sessionUses) latestOfDevice(deviceID int64) (int64, bool) {
	uses.mutex.Lock()
	defer uses.mutex.Unlock()
	var latest int64
	found := false
	for _, use := range uses.latestBySession {
		if use.deviceID == deviceID && use.atMs > latest {
			latest, found = use.atMs, true
		}
	}
	return latest, found
}

// take takes every use not yet written.
func (uses *sessionUses) take() []sessionUse {
	uses.mutex.Lock()
	defer uses.mutex.Unlock()
	taken := make([]sessionUse, 0, len(uses.latestBySession))
	for _, use := range uses.latestBySession {
		taken = append(taken, use)
	}
	uses.latestBySession = nil
	return taken
}

// putBack returns uses a flush could not write, so the next flush tries them again; a later use
// noted meanwhile wins.
func (uses *sessionUses) putBack(taken []sessionUse) {
	for _, use := range taken {
		uses.note(use.hash, use.deviceID, use.atMs)
	}
}

// FlushSessionUses writes every session's latest use since the last flush, in one transaction. A
// stored time is never moved back: a use written late never overwrites a later one. Uses it could
// not write are kept for the next flush.
func (service *Service) FlushSessionUses(ctx context.Context) error {
	taken := service.uses.take()
	if len(taken) == 0 {
		return nil
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, use := range taken {
			err := queries.UpdateGrantLastUsed(ctx, sqlcgen.UpdateGrantLastUsedParams{
				SecretSha256: use.hash, LastUsedAt: new(use.atMs),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		service.uses.putBack(taken)
		return fmt.Errorf("write when %d sessions were last used: %w", len(taken), err)
	}
	return nil
}
