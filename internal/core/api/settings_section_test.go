// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// A setting's section is one the API document lists, in the order the settings page lists them,
// which is the settings package's (the owner, 2026-10-21).
func TestSettingBody_ListsTheSectionsInTheSettingsOrder(t *testing.T) {
	t.Parallel()
	field, _ := reflect.TypeFor[SettingBody]().FieldByName("Section")
	listed := strings.Split(field.Tag.Get("enum"), ",")
	var want []string
	for _, section := range settings.Sections() {
		want = append(want, string(section))
	}
	if !slices.Equal(listed, want) {
		t.Errorf("the API lists the sections %v; the settings package has %v", listed, want)
	}
}
