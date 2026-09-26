// SPDX-License-Identifier: GPL-3.0-or-later

package langtag_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/langtag"
)

func TestBCP47_NormalisesEveryFormItMeets(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"en": "en", "eng": "en", "ger": "de", "deu": "de", "pt-BR": "pt-BR", "pt-br": "pt-BR",
		"und": "", "": "", "xx-not-a-tag!": "",
	} {
		if got := langtag.BCP47(raw); got != want {
			t.Errorf("BCP47(%q) = %q, want %q", raw, got, want)
		}
	}
}
