// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestWrite_TheOverrideGivesTheNodeTheNVIDIAGPUsAsDockerHandsThemOver(t *testing.T) {
	t.Parallel()
	type reservation struct {
		Driver       string   `json:"driver"`
		Count        string   `json:"count"`
		Capabilities []string `json:"capabilities"`
	}
	for _, tc := range []struct {
		name            string
		nvidia          NVIDIAAccess
		wantDevices     []string
		wantReservation []reservation
	}{
		{"by CDI", NVIDIAByCDI, []string{"/dev/dri:/dev/dri", "nvidia.com/gpu=all"}, nil},
		{
			"by Docker's driver", NVIDIAByDriver,
			[]string{"/dev/dri:/dev/dri"},
			[]reservation{{"nvidia", "all", []string{"gpu", "compute", "video", "utility"}}},
		},
		{"not at all", NVIDIANone, []string{"/dev/dri:/dev/dri"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := mediaOverride([]string{"/srv/c"}, 105, tc.nvidia)
			if err != nil {
				t.Fatal(err)
			}
			var override struct {
				Services map[string]struct {
					Devices []string `json:"devices"`
					Deploy  struct {
						Resources struct {
							Reservations struct {
								Devices []reservation `json:"devices"`
							} `json:"reservations"`
						} `json:"resources"`
					} `json:"deploy"`
				} `json:"services"`
			}
			if err = json.Unmarshal(text, &override); err != nil {
				t.Fatal(err)
			}
			node := override.Services["orpheum-node"]
			if !slices.Equal(node.Devices, tc.wantDevices) {
				t.Errorf("the node's devices are %v; want %v", node.Devices, tc.wantDevices)
			}
			got := node.Deploy.Resources.Reservations.Devices
			if !slices.EqualFunc(got, tc.wantReservation, func(a, b reservation) bool {
				return a.Driver == b.Driver && a.Count == b.Count && slices.Equal(a.Capabilities, b.Capabilities)
			}) {
				t.Errorf("the node reserves %+v; want %+v", got, tc.wantReservation)
			}
		})
	}
}

func TestParseNVIDIAAccess_TakesCDIDriverOrNothing(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]NVIDIAAccess{"": NVIDIANone, "cdi": NVIDIAByCDI, "driver": NVIDIAByDriver} {
		if got, err := ParseNVIDIAAccess(name); err != nil || got != want {
			t.Errorf("ParseNVIDIAAccess(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := ParseNVIDIAAccess("all"); err == nil {
		t.Error(`ParseNVIDIAAccess("all") succeeded; want an error`)
	}
}
