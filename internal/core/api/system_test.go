// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// finishPrepare records a prepare node 1 finished of file, made with encoder at speed.
func finishPrepare(t *testing.T, harness testHarness, file int64, encoder string, speed float64) {
	t.Helper()
	err := harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		id, err := queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
			SourceFile: file, PayloadHash: []byte{1}, Profile: "h264-8m", NodeID: 1, SegmentLengthS: 6,
			RunID: "0123456789abcdef", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("insert the rendition: %w", err)
		}
		size, finished := int64(2048), int64(1_790_000_000_000)
		return queries.SetRenditionComplete(t.Context(), cachegen.SetRenditionCompleteParams{
			SizeBytes: &size, FinishedAt: &finished, VideoEncoder: &encoder, Speed: &speed, ID: id,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystem_ShowsTheServersFactsEachNodesAndDoctorsFindings(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	storetest.ExecRaw(t, harness.database, "INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)")
	harness.connectNode(t, "/media/films")
	harness.nodes.confinement = []nodeproto.Confinement{{Mechanism: "landlock", InForce: true}}
	harness.nodes.hardware = nodeproto.Hardware{
		GPUs: []nodeproto.GPU{{Device: "/dev/dri/renderD128", Driver: "i915"}},
		Trials: []nodeproto.EncoderTrial{
			{Encoder: "h264_qsv"}, {Encoder: "h264_nvenc", Failure: "Cannot load libcuda.so.1"},
		},
		Encoder: "h264_qsv",
	}
	harness.nodes.disks = []nodeproto.Disk{{Path: "/media/films", TotalBytes: 4000, FreeBytes: 1000}}
	harness.nodes.prepared = &nodeproto.PreparedUsage{
		Bytes: 10, BudgetBytes: 100, DiskPercent: 75, DiskLimitPercent: 95,
	}
	finishPrepare(t, harness, 42, "h264_qsv", 6.5)

	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	if got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	var system struct {
		Server   ServerBody       `json:"server"`
		Findings []FindingBody    `json:"findings"`
		Nodes    []SystemNodeBody `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(got.body), &system); err != nil {
		t.Fatal(err)
	}
	checkServerFacts(t, system.Server)
	if len(system.Nodes) != 2 || system.Nodes[0].Name != "attic" || system.Nodes[0].IsConnected {
		t.Fatalf("the nodes are %+v; want attic, not connected, then local", system.Nodes)
	}
	checkLocalFacts(t, system.Nodes[1])
	if !strings.Contains(got.body, `"check":"hardware encoding on node local","verdict":"ok"`) {
		t.Errorf("the findings are %+v; want doctor's, with the node's hardware encoding", system.Findings)
	}
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/system", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, CodeForbidden)
}

// checkServerFacts checks the server's own facts on the System page.
func checkServerFacts(t *testing.T, server ServerBody) {
	t.Helper()
	if server.Version != "0.3.0" || server.Platform == "" || server.GoVersion == "" || len(server.Disks) != 2 ||
		server.Disks[0].TotalBytes <= 0 || len(server.Databases) != 2 || server.Databases[0].SizeBytes <= 0 {
		t.Errorf("the server's facts are %+v; want its version, platform, both disks and both databases", server)
	}
}

// checkLocalFacts checks what the connected node said about itself, as the System page shows it.
func checkLocalFacts(t *testing.T, local SystemNodeBody) {
	t.Helper()
	if !local.IsConnected || local.Platform != "linux/amd64" || local.FFmpegVersion != "ffmpeg version 7.1" ||
		len(local.Confinement) != 1 || !local.Confinement[0].IsInForce || local.Prepared == nil ||
		len(local.Disks) != 1 || local.Disks[0].FreeBytes != 1000 {
		t.Errorf("local's facts are %+v; want what it said about itself (D225)", local)
	}
	checkHardware(t, local.Hardware)
	if len(local.RecentPrepares) != 1 || local.RecentPrepares[0].FileID != 42 ||
		local.RecentPrepares[0].VideoEncoder != "h264_qsv" || local.RecentPrepares[0].Speed != 6.5 {
		t.Errorf("local's recent prepares are %+v; want the one it finished, with Quick Sync at 6.5x (D225 §3)",
			local.RecentPrepares)
	}
}

// checkHardware checks the connected node's hardware encoding, as the System page shows it.
func checkHardware(t *testing.T, hardware *HardwareBody) {
	t.Helper()
	if hardware == nil || hardware.Encoder != "h264_qsv" || len(hardware.GPUs) != 1 ||
		hardware.GPUs[0].Driver != "i915" || len(hardware.Trials) != 2 ||
		hardware.Trials[1].Failure != "Cannot load libcuda.so.1" {
		t.Errorf("local's hardware is %+v; want its GPU, each trial and the encoder chosen (D225 §3)", hardware)
	}
}

func TestHardwareFinding_WarnsWhenANodeHasAGPUButEncodesOnTheProcessor(t *testing.T) {
	t.Parallel()
	quickSync := hardwareFinding("local", nodeproto.Hardware{
		GPUs: []nodeproto.GPU{{Device: "/dev/dri/renderD128"}}, Encoder: "h264_qsv",
	})
	if quickSync.Verdict != verdictOK || !strings.Contains(quickSync.Detail, "h264_qsv") {
		t.Errorf("a node encoding with Quick Sync: %+v, want ok, naming it", quickSync)
	}
	noGPU := hardwareFinding("local", nodeproto.Hardware{Encoder: "x264"})
	if noGPU.Verdict != verdictOK || !strings.Contains(noGPU.Detail, "no GPU") {
		t.Errorf("a node with no GPU: %+v, want ok, saying it has none", noGPU)
	}
	blocked := hardwareFinding("local", nodeproto.Hardware{
		GPUs:    []nodeproto.GPU{{Device: "/dev/dri/renderD128", NoAccess: "permission denied"}},
		Trials:  []nodeproto.EncoderTrial{{Encoder: "h264_qsv", Failure: "Failed to initialise VAAPI"}},
		Encoder: "x264",
	})
	if blocked.Verdict != verdictWarn || !strings.Contains(blocked.Detail, "Failed to initialise VAAPI") ||
		!strings.Contains(blocked.Action, "/dev/dri/renderD128") {
		t.Errorf("a node whose user cannot open its GPU: %+v, want a warning with the trial's words and the device "+
			"to give it (D225 §3)", blocked)
	}
}
