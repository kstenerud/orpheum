// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// recordedShare is a WebRTC server's share as a test sees it: each limit it was given.
type recordedShare struct{ limits []int }

func (share *recordedShare) SetConnectionsPerNetwork(limit int) {
	share.limits = append(share.limits, limit)
}

// TestKeepWebRTCShareCurrent_GivesTheSettingNowAndAtEachChange: the owner's network was turned away
// past a fixed share (staging, 2026-10-25), which is now webrtc_connections_per_network, read at start
// and again whenever an admin changes it.
func TestKeepWebRTCShareCurrent_GivesTheSettingNowAndAtEachChange(t *testing.T) {
	database := storetest.OpenMigrated(t)
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	service := settings.NewService(database, clocktest.NewFake(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)), logger)
	if _, err := service.Seed(t.Context()); err != nil {
		t.Fatal(err)
	}
	share := &recordedShare{}
	keepWebRTCShareCurrent(t.Context(), share, service, logger)
	if len(share.limits) != 1 || share.limits[0] != 64 {
		t.Fatalf("at start the share was given %v, want [64], the setting's starting value", share.limits)
	}
	values := map[string]json.RawMessage{string(settings.WebRTCConnectionsPerNetwork): json.RawMessage("100")}
	if err := service.Set(t.Context(), values, nil, false); err != nil {
		t.Fatal(err)
	}
	if got := share.limits[len(share.limits)-1]; got != 100 {
		t.Fatalf("after the setting changed the share was given %v, want it to end at 100", share.limits)
	}
}
