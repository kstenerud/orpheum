// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"slices"
	"testing"
	"time"
)

func TestSim_EventsRunInTheirTimeOrderAndTiesInTheirOrder(t *testing.T) {
	t.Parallel()
	var sim Sim
	var ran []string
	sim.At(3*time.Second, func() { ran = append(ran, "c") })
	sim.At(time.Second, func() { ran = append(ran, "a") })
	sim.At(time.Second, func() {
		ran = append(ran, "b")
		sim.After(time.Second, func() { ran = append(ran, "b+1s at "+sim.Now().String()) })
	})
	sim.RunUntil(10 * time.Second)
	want := []string{"a", "b", "b+1s at 2s", "c"}
	if !slices.Equal(ran, want) || sim.Now() != 10*time.Second {
		t.Errorf("ran %v, now %v; want %v, now 10s", ran, sim.Now(), want)
	}
}
