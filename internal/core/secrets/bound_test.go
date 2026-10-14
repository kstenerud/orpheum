// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"crypto/rand"
	"errors"
	"testing"
)

// A bound value opens for its own row under its own key, and nowhere else.
func TestStore_ABoundValueOpensOnlyForItsRowUnderItsKey(t *testing.T) {
	t.Parallel()
	key, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sealer := &Store{key: key, random: rand.Reader}
	sealed, err := sealer.SealBound("grant 1", "a links secret")
	if err != nil {
		t.Fatal(err)
	}
	if opened, openErr := sealer.OpenBound("grant 1", sealed); openErr != nil || opened != "a links secret" {
		t.Errorf("OpenBound for its own row = %q, %v; want the secret", opened, openErr)
	}
	for name, open := range map[string]func() (string, error){
		"another row":  func() (string, error) { return sealer.OpenBound("grant 2", sealed) },
		"another key":  func() (string, error) { return (&Store{key: other}).OpenBound("grant 1", sealed) },
		"cut short":    func() (string, error) { return sealer.OpenBound("grant 1", sealed[:10]) },
		"altered byte": func() (string, error) { return sealer.OpenBound("grant 1", flipLast(sealed)) },
	} {
		if opened, openErr := open(); !errors.Is(openErr, ErrBoundUnopened) || opened != "" {
			t.Errorf("OpenBound under %s = %q, %v; want ErrBoundUnopened", name, opened, openErr)
		}
	}
}

func flipLast(sealed []byte) []byte {
	altered := append([]byte{}, sealed...)
	altered[len(altered)-1] ^= 1
	return altered
}
