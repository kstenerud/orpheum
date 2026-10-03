// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"encoding/hex"
	"strconv"
	"sync"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// keptTexts tells whoever waits for a subtitle's text, by where it is kept, that it was kept: made,
// or the converter's word that it cannot be (finishSubtitle). A fetch waits so for the conversion
// job it queued while its person waits.
type keptTexts struct {
	mutex   sync.Mutex
	waiting map[string][]chan struct{} // by textKeyName; guarded by mutex
}

func newKeptTexts() *keptTexts {
	return &keptTexts{waiting: map[string][]chan struct{}{}}
}

// textKeyName is a text key as one string.
func textKeyName(key sqlcgen.GetSubtitleTextParams) string {
	return hex.EncodeToString(key.SourceHash) + "/" + strconv.FormatInt(key.StreamIndex, 10)
}

// await is a channel closed when the text kept where key says is next kept.
func (texts *keptTexts) await(key sqlcgen.GetSubtitleTextParams) <-chan struct{} {
	kept := make(chan struct{})
	texts.mutex.Lock()
	defer texts.mutex.Unlock()
	name := textKeyName(key)
	texts.waiting[name] = append(texts.waiting[name], kept)
	return kept
}

// wasKept tells everyone waiting for the text kept where key says.
func (texts *keptTexts) wasKept(key sqlcgen.GetSubtitleTextParams) {
	texts.mutex.Lock()
	defer texts.mutex.Unlock()
	name := textKeyName(key)
	for _, kept := range texts.waiting[name] {
		close(kept)
	}
	delete(texts.waiting, name)
}
