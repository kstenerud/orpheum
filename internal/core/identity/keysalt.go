// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
)

// KeySaltFileName is the key salt's file in the instance's persistent directory, beside identity.key.
// Unlike the key it is in every backup: a restore that lost it would leave every person's
// away-from-home key unusable until each signed in at home again (D527 §2.2).
const KeySaltFileName = "key-salt"

// LoadOrMakeKeySalt reads the core's key salt, making and keeping one the first time: KeySaltBytes
// random bytes, made once and never changed, which salt each person's version-2 key (proof.PersonKey)
// and which the core sends its discovery in every heartbeat, so a page can derive the same key (D527
// §2.2). A file that holds no salt is refused, never replaced.
func LoadOrMakeKeySalt(files Files, random io.Reader) ([]byte, error) {
	contents, err := files.ReadPersistentFile(KeySaltFileName)
	if err == nil {
		salt, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(contents)))
		if decodeErr != nil || len(salt) != proof.KeySaltBytes {
			return nil, fmt.Errorf("%s: holds no key salt of %d bytes", KeySaltFileName, proof.KeySaltBytes)
		}
		return salt, nil
	}
	if !errors.Is(err, instancedir.ErrNotFound) {
		return nil, fmt.Errorf("read %s: %w", KeySaltFileName, err)
	}
	salt := make([]byte, proof.KeySaltBytes)
	if _, err = io.ReadFull(random, salt); err != nil {
		return nil, fmt.Errorf("make a key salt: %w", err)
	}
	kept := []byte(base64.RawURLEncoding.EncodeToString(salt) + "\n")
	if err = files.WritePersistentFile(KeySaltFileName, kept); err != nil {
		return nil, fmt.Errorf("keep %s: %w", KeySaltFileName, err)
	}
	return salt, nil
}
