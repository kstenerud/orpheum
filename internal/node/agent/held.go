// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// HeldResults are the results the node holds, too large to post (nodeproto.MaxPostedResultBytes),
// by their attempt's token: the byte socket serves each to the core, and the node forgets it once
// the core acknowledges it (D234 §5). In memory, as the node's work is (D238 §3).
type HeldResults struct {
	mutex   sync.Mutex
	byToken map[string][]byte // guarded by mutex
}

// NewHeldResults returns a node's held results, none yet.
func NewHeldResults() *HeldResults {
	return &HeldResults{byToken: map[string][]byte{}}
}

// Result is the encoded result held for an attempt's token, and whether one is.
func (held *HeldResults) Result(token string) ([]byte, bool) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	encoded, found := held.byToken[token]
	return encoded, found
}

// hold keeps an encoded result for the core to fetch, and names it.
func (held *HeldResults) hold(token string, encoded []byte) nodeproto.HeldResult {
	sum := sha256.Sum256(encoded)
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.byToken[token] = encoded
	return nodeproto.HeldResult{SizeBytes: int64(len(encoded)), Sha256: hex.EncodeToString(sum[:])}
}

// forget drops an attempt's result, once the core acknowledged it.
func (held *HeldResults) forget(token string) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	delete(held.byToken, token)
}
