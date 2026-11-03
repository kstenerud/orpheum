// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// signTask is a payload to be signed: its namespace, the file it is in, its bytes, and the check that a
// signature of it holds.
type signTask struct {
	namespace, path string
	payload         []byte
	verify          func(trust.Signed) error
}

// signWith has each of signers sign the payload file at path in namespace, one after another, each
// signature checked (by verify, and as the key expected) before the next is asked for, and writes the
// signatures, one file, beside the payload; it returns the payload with them.
func (guide *guided) signWith(place *folder, namespace, path string, signers []keyEntry,
	verify func(trust.Signed) error,
) (trust.Signed, error) {
	payload, err := os.ReadFile(path) //nolint:gosec // the owner's own ledger
	if err != nil {
		return trust.Signed{}, err
	}
	task := signTask{namespace: namespace, path: path, payload: payload, verify: verify}
	var combined strings.Builder
	for _, key := range signers {
		block, signErr := guide.signOne(place, task, key)
		if signErr != nil {
			return trust.Signed{}, signErr
		}
		combined.WriteString(block)
	}
	signature := combined.String()
	if err = os.WriteFile(path+addressdoc.SignatureEnd, []byte(signature), 0o600); err != nil {
		return trust.Signed{}, fmt.Errorf("write the signature: %w", err)
	}
	return trust.Signed{Payload: string(payload), Signature: signature}, nil
}

// signOne is key's signature of the task, which the owner may try again when it is refused.
func (guide *guided) signOne(place *folder, task signTask, key keyEntry) (string, error) {
	expected, err := readKey(place.join(key.Pub))
	if err != nil {
		return "", err
	}
	for attempt := 1; ; attempt++ {
		block, signErr := guide.signAttempt(task, key, expected, attempt)
		if signErr == nil {
			return block, nil
		}
		guide.say.sayf("Refused: %v", signErr)
		again, askErr := guide.say.yes("Try this key again?", true)
		if askErr != nil {
			return "", askErr
		}
		if !again {
			return "", fmt.Errorf("the signature of %s-%s was refused and the ceremony stopped there; nothing "+
				"after it was done (run it again to carry on): %w", key.Chain, key.Name, signErr)
		}
	}
}

// signAttempt asks the owner for key's signature once and checks it.
func (guide *guided) signAttempt(task signTask, key keyEntry, expected trust.RootKey, attempt int) (string, error) {
	if key.Kind != kindSoftware {
		plug := fmt.Sprintf("Plug in the key named %q (only that one). %s", key.Name, touchHint(key.Kind))
		if err := guide.say.press(plug); err != nil {
			return "", err
		}
	}
	// Each attempt signs its own copy, so a signature is never mistaken for an older one.
	copyDirectory := task.path + ".parts"
	copied := filepath.Join(copyDirectory, key.Chain+"-"+key.Name+"-"+strconv.Itoa(attempt))
	if err := os.MkdirAll(copyDirectory, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(copied, task.payload, 0o600); err != nil {
		return "", err
	}
	spec := signSpec{Namespace: task.namespace, KeyFile: key.File, Payload: copied}
	if err := guide.keys.Sign(spec); err != nil {
		return "", err
	}
	signature, err := os.ReadFile(copied + addressdoc.SignatureEnd) //nolint:gosec // as above
	if err != nil {
		return "", fmt.Errorf("ssh-keygen wrote no signature: %w", err)
	}
	blocks, err := trust.Inspect(string(signature))
	if err != nil || len(blocks) != 1 {
		return "", errors.Join(err, errors.New("that is not one signature"))
	}
	if blocks[0].Fingerprint != expected.Fingerprint() {
		return "", fmt.Errorf("that signature is by another key (%s), not %q (%s): the wrong key was plugged in",
			blocks[0].Fingerprint, key.Name, expected.Fingerprint())
	}
	signed := trust.Signed{Payload: string(task.payload), Signature: string(signature)}
	if verifyErr := task.verify(signed); verifyErr != nil {
		return "", verifyErr
	}
	guide.say.sayf("Good: %q signed, and the signature holds.", key.Name)
	return string(signature), nil
}
