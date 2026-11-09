// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A node whose config requires confinement, on a kernel without Landlock, starts locked: it tells the
// core what it lacks and runs no FFmpeg and says no hello until the core says an admin lets it run
// unconfined; it then starts, saying it runs by that leave (D450 §1).
func TestRunNode_StartsLockedWithoutLandlockUntilAnAdminLetsItRunUnconfined(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.Node.ConfinementPolicy = "require"
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.FFmpegDir = fake.Directory
	withoutLandlock := nodeSeams{isUnconfined: true, statusEvery: time.Second, mechanismStates: func() []confine.State {
		return []confine.State{
			{Mechanism: confine.MechanismLandlock, Reason: "not enabled at boot", Remedy: "add lsm=landlock"},
			{Mechanism: confine.MechanismSeccomp, IsInForce: true},
		}
	}}
	core := serveLockedCore(t, config.Node.CoreSocket)
	captured := newCapturedLog()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runNodeWith(ctx, config, captured.logger, withoutLandlock) }()
	defer func() {
		cancel()
		if err := testutil.Receive(t, done, "the node to stop"); err != nil {
			t.Errorf("runNode returned %v", err)
		}
	}()
	first := testutil.Receive(t, core.reports, "the locked node's first report")
	if first.Name != config.Node.Name || len(first.Confinement) != 2 {
		t.Errorf("the locked report = %+v; want the node's name and each mechanism's state", first)
	}
	testutil.Receive(t, core.reports, "the locked node's report at its next tick")
	if calls := fake.Calls(t); len(calls) != 0 {
		t.Errorf("a locked node ran FFmpeg %d times; want none", len(calls))
	}
	if logged := captured.String(); !strings.Contains(logged, "add lsm=landlock") ||
		!strings.Contains(logged, "nodeLocked") {
		t.Errorf("the log = %s; want the missing mechanism's remedy and that the node is locked", logged)
	}
	core.mayRunUnconfined.Store(true)
	hello := testutil.Receive(t, core.hellos, "the hello once an admin lets the node run")
	if !hello.IsUnconfinedByLeave {
		t.Error("the hello does not say the node runs by an admin's leave; want it to")
	}
	if calls := fake.Calls(t); len(calls) == 0 {
		t.Error("the node let run unconfined ran no FFmpeg before its hello; want its probe of ffprobe")
	}
}

// lockedCore is a fake core that answers a locked node's reports with its leave.
type lockedCore struct {
	reports          chan nodeproto.LockedReport
	hellos           chan nodeproto.Hello
	mayRunUnconfined atomic.Bool
}

// serveLockedCore serves a lockedCore on socket: it passes on each locked report and hello, and holds
// each poll for commands open.
func serveLockedCore(t *testing.T, socket string) *lockedCore {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	core := &lockedCore{reports: make(chan nodeproto.LockedReport, 8), hellos: make(chan nodeproto.Hello, 4)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathLocked, func(writer http.ResponseWriter, request *http.Request) {
		var report nodeproto.LockedReport
		if err := json.NewDecoder(request.Body).Decode(&report); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		reply := nodeproto.LockedReply{MayRunUnconfined: core.mayRunUnconfined.Load()}
		select {
		case core.reports <- report:
		default:
		}
		_ = json.NewEncoder(writer).Encode(reply) //nolint:errcheck // the node says if not
	})
	mux.HandleFunc("POST "+nodeproto.PathHello, func(writer http.ResponseWriter, request *http.Request) {
		var hello nodeproto.Hello
		if err := json.NewDecoder(request.Body).Decode(&hello); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case core.hellos <- hello:
		default:
		}
		reply := nodeproto.HelloReply{NodeID: 1}
		_ = json.NewEncoder(writer).Encode(reply) //nolint:errcheck // the node says if not
	})
	mux.HandleFunc("GET "+nodeproto.PathCommands, func(_ http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-t.Context().Done():
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathStatus, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // the node says if not
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: serveWaitLimit}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close() //nolint:errcheck // a test's server, closed at its end
		if err := testutil.Receive(t, served, "the fake core to stop"); !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("the fake core stopped with %v", err)
		}
	})
	return core
}

// A node that takes what confinement is available runs without a mechanism its kernel lacks, unasked,
// and says which; one whose kernel has a mechanism that cannot be put in force starts locked, as does
// any node whose config requires every mechanism (D454 §3).
func TestCheckConfinement_AKernelWithoutAMechanismRunsWithoutItButOneWhereItFailsIsLocked(t *testing.T) {
	t.Parallel()
	isInForce := confine.State{Mechanism: confine.MechanismSeccomp, IsInForce: true}
	tooOld := confine.State{
		Mechanism: confine.MechanismLandlock, CauseCode: confine.CauseKernelWithout, Remedy: "a kernel of 5.13",
	}
	notAtBoot := confine.State{Mechanism: confine.MechanismLandlock, CauseCode: confine.CauseDisabledAtBoot}
	blocked := confine.State{Mechanism: confine.MechanismLandlock, CauseCode: confine.CauseBlockedByRuntime}
	unexplained := confine.State{Mechanism: confine.MechanismSeccomp, CauseCode: confine.CauseUnexplained}
	for _, want := range []struct {
		policy   string
		states   []confine.State
		isLocked bool
	}{
		{policyAvailable, []confine.State{tooOld, isInForce}, false},
		{policyAvailable, []confine.State{notAtBoot, isInForce}, false},
		{policyAvailable, []confine.State{blocked, isInForce}, true},
		{policyAvailable, []confine.State{tooOld, unexplained}, true},
		{policyRequire, []confine.State{tooOld, isInForce}, true},
		{policyReport, []confine.State{blocked, isInForce}, false},
	} {
		config, _ := testConfig(t)
		config.Node.ConfinementPolicy = want.policy
		seams := nodeSeams{mechanismStates: func() []confine.State { return want.states }}
		reported, isLocked := checkConfinement(t.Context(), config, newCapturedLog().logger, seams)
		if isLocked != want.isLocked || len(reported) != len(want.states) {
			t.Errorf("%q with %+v: locked %v, reporting %d states; want locked %v and every state",
				want.policy, want.states, isLocked, len(reported), want.isLocked)
		}
	}
	config, _ := testConfig(t)
	config.Node.ConfinementPolicy = policyAvailable
	captured := newCapturedLog()
	checkConfinement(t.Context(), config, captured.logger,
		nodeSeams{mechanismStates: func() []confine.State { return []confine.State{tooOld, isInForce} }})
	if logged := captured.String(); !strings.Contains(logged, "a kernel of 5.13") {
		t.Errorf("the log = %s; want the missing mechanism and the kernel that brings it", logged)
	}
}
