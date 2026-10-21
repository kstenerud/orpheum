// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// A root's disk measured says the facts the core tells its disk back by, its device and whether it
// is a mount point, and the log says when it stopped answering and when it answers again: neither
// was there to say where a share's minute went (staging, 2026-10-21).
func TestAgent_ARootsDiskSaysItsFactsAndWhenItStopsAndStartsAnswering(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	root, err := confine.OpenMediaRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	device, _ := confine.DeviceOf(info)
	isAway := true
	var logged bytes.Buffer
	node := &Agent{
		hello: nodeproto.Hello{RootPaths: []string{path}}, roots: map[string]*confine.MediaRoot{path: root},
		diskGate: diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 10, 21, 18, 0, 0, 0, time.UTC))),
		logger:   slog.New(slog.NewTextHandler(&logged, nil)),
		measureSpace: func(string) (diskspace.Space, error) {
			if isAway {
				return diskspace.Space{}, syscall.EIO // the share's host is gone
			}
			return diskspace.Space{TotalBytes: 1 << 30}, nil
		},
	}
	away := node.disks(t.Context())
	node.disks(t.Context())
	isAway = false
	back := node.disks(t.Context())
	if len(away) != 1 || !away[0].IsNotAnswering || away[0].Device != 0 {
		t.Errorf("away: %+v; want it not answering, with no facts", away)
	}
	if len(back) != 1 || back[0].Failure != "" || back[0].Device != device || back[0].IsMountPoint {
		t.Errorf("back: %+v; want it measured on device %d, not a mount point", back, device)
	}
	if stopped, again := strings.Count(logged.String(), "a disk stopped answering"),
		strings.Count(logged.String(), "a disk answers again"); stopped != 1 || again != 1 {
		t.Errorf("the log = %q; want it stopping said once and answering again once", logged.String())
	}
}
