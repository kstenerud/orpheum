// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth/authtest"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The server's library admin service names as many of a held-back scan's removals as its setting
// says (the constants sweep, 2026-10-20).
func TestLibraryAdminOf_NamesAsManyRemovalsAsTheServersSetting(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	settingstest.Set(t, config, settings.ScanWithheldRemovalsNamed, "1")
	nodeID := librarytest.Node(t, database, "nas")
	libraryID := librarytest.LibraryOn(t, database, nodeID,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media"})
	librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: libraryID, NodeID: nodeID, Path: "/media/a.mkv", Filing: "skipped",
	})
	librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: libraryID, NodeID: nodeID, Path: "/media/b.mkv", Filing: "skipped",
	})
	storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason) VALUES (%d, '/media', 'withheld', 9000, 5000, 'the scan would remove 2 of 2 files')`,
		nodeID))
	now := clocktest.NewFake(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	parts := Parts{Database: database, Clock: now, Settings: config}
	waiting, err := (&Services{}).libraryAdmin(parts).Waiting(t.Context())
	if err != nil || len(waiting) != 1 || waiting[0].FileCount != 2 ||
		!slices.Equal(waiting[0].FirstPaths, []string{"/media/a.mkv"}) {
		t.Errorf("waiting = %+v (%v); want both counted and the first alone named", waiting, err)
	}
}

// A full failed-sign-in limit is warned of, so the operator learns that sign-ins are being refused
// for want of room (c21fe368).
func TestWarnSignInLimitFull_WarnsThatSignInsAreRefused(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	warnSignInLimitFull(t.Context(), slog.New(slog.NewJSONHandler(&logged, nil)))()
	if !strings.Contains(logged.String(), `"level":"WARN"`) ||
		len(testutil.LinesWith(logged.String(), "signInLimitFull")) == 0 {
		t.Errorf("logged %s; want a warning that the limit is full", logged.String())
	}
}

// testParts are what Build is given in a test: a migrated database, a cache, an instance's
// directories, seeded settings, a new secret key and a fake clock.
func testParts(t *testing.T) Parts {
	t.Helper()
	database := storetest.OpenMigrated(t)
	directories, err := instancedir.Open(instancedir.PathsIn(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Parts{
		Database: database, Cache: storetest.OpenCache(t), Directories: directories,
		Settings: settingstest.Seeded(t, database), SecretKey: key, SetupCodes: &authtest.SetupCodes{},
		Clock: clocktest.NewFake(time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)), Random: rand.Reader,
		Logger: slog.New(slog.DiscardHandler), Build: build.Provenance{Version: "test"},
	}
}

// quietNodes is a faked node channel that takes the services' wiring and is asked nothing else
// while they are built.
type quietNodes struct{ Nodes }

func (quietNodes) SetResultSink(nodes.ResultSink) {}
func (quietNodes) SetScans(nodes.ScanRequester)   {}
func (quietNodes) SetChanges(nodes.ChangeTaker)   {}
func (quietNodes) SetStoreTiers(nodes.StoreTiers) {}

// fakeNodeBytes reads no node's bytes.
type fakeNodeBytes struct{}

func (fakeNodeBytes) Do(context.Context, int64, string, string, http.Header) (*http.Response, error) {
	return nil, errors.New("a test reads no node's bytes")
}

// The core's own node channel has a byte socket the API reads a node's bytes over; a test that fakes
// the channel fakes its bytes too, or Build refuses, and the fakes are what the API is given (D351 §3).
func TestBuild_AFakedNodeChannelNeedsItsBytesFakedToo(t *testing.T) {
	t.Parallel()
	own, err := Build(t.Context(), testParts(t))
	if err != nil || own.NodeChannel == nil || own.API.NodeBytes == nil || own.API.NodeReports != own.NodeChannel {
		t.Fatalf("the core's own node channel: %+v (%v); want it built, and the API reading its bytes", own, err)
	}
	parts := testParts(t)
	parts.Nodes = quietNodes{}
	if _, unfakedErr := Build(t.Context(), parts); !errors.Is(unfakedErr, errNodeBytesNotFaked) {
		t.Errorf("a faked node channel with no faked bytes: %v; want %v", unfakedErr, errNodeBytesNotFaked)
	}
	parts.NodeBytes = fakeNodeBytes{}
	faked, err := Build(t.Context(), parts)
	if err != nil || faked.NodeChannel != nil || faked.API.NodeBytes != (fakeNodeBytes{}) ||
		faked.API.NodeReports != (quietNodes{}) {
		t.Errorf("a faked node channel and bytes: %+v (%v); want the fakes, and no channel of the core's own",
			faked, err)
	}
}
