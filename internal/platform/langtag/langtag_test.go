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

func TestIsWellFormed_AcceptsBCP47sShapeOnly(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]bool{
		"en": true, "pt-BR": true, "zh-Hant-TW": true, "en-x-foo": true, "de-1996": true,
		"": false, "e": false, "english": false, "en-": false, "en-toolongsub": false, "en;rm": false,
		"en_GB": false,
	} {
		if got := langtag.IsWellFormed(tag); got != want {
			t.Errorf("IsWellFormed(%q) = %t, want %t", tag, got, want)
		}
	}
}
