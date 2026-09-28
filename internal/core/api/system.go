// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// recentPrepares is how many of a node's finished prepares the System page lists.
const recentPrepares = 10

// NodeReports is what each connected node said about itself: nodes.Service.
type NodeReports interface {
	Facts(name string) (nodes.Facts, bool)
}

// Sized is a database that can say how large it is: store.Database, store.Cache.
type Sized interface {
	SizeBytes(ctx context.Context) (uint64, error)
}

// System is what the System page shows of the server itself (D225).
type System struct {
	Version   string
	StartedAt time.Time
	// StateDirectory and CacheDirectory are the server's own, whose disks the page shows.
	StateDirectory string
	CacheDirectory string
	// Library and Cache are library.db and cache.db, measured when the page asks.
	Library Sized
	Cache   Sized
}

// SystemOutput is the System page: the server's facts, what doctor finds, and each node's facts
// (D225).
type SystemOutput struct {
	Body struct {
		Server   ServerBody       `json:"server"`
		Findings []FindingBody    `json:"findings"`
		Nodes    []SystemNodeBody `json:"nodes"`
	}
}

// ServerBody is the server's own facts.
type ServerBody struct {
	Version   string         `json:"version"`
	Platform  string         `doc:"Operating system and architecture, as Go names them" json:"platform"`
	GoVersion string         `json:"goVersion"`
	StartedAt int64          `doc:"Unix milliseconds" json:"startedAt"`
	Disks     []DiskBody     `json:"disks"`
	Databases []DatabaseBody `json:"databases"`
}

// DiskBody is one filesystem, named by a directory on it.
type DiskBody struct {
	Path       string `json:"path"`
	TotalBytes int64  `json:"totalBytes"`
	FreeBytes  int64  `doc:"What a user who is not root may still write" json:"freeBytes"`
	Failure    string `doc:"Why it could not be measured; the sizes are zero then" json:"failure,omitempty"`
}

// DatabaseBody is one of the server's database files and its size.
type DatabaseBody struct {
	Name      string `enum:"library.db,cache.db" json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

// SystemNodeBody is one node's facts, as it last said them; only its name and whether it is
// connected while it is not.
type SystemNodeBody struct {
	Name        string `json:"name"`
	IsConnected bool   `doc:"Whether it has said hello since the server started" json:"isConnected"`
	// LastHeardAt is when it last spoke since the server started, in Unix milliseconds.
	LastHeardAt   *int64 `json:"lastHeardAt,omitempty"`
	Version       string `json:"version,omitempty"`
	Platform      string `json:"platform,omitempty"`
	FFmpegVersion string `json:"ffmpegVersion,omitempty"`
	FFmpegHash    string `json:"ffmpegHash,omitempty"`
	UID           int    `doc:"The user the node runs as" json:"uid"`
	GID           int    `json:"gid"`
	// Confinement is how its FFmpeg runs are confined, per mechanism (D112).
	Confinement []ConfinementBody `json:"confinement"`
	// Hardware is what it found for hardware encoding; absent while it is not connected.
	Hardware *HardwareBody `json:"hardware,omitempty"`
	// Prepared is what its prepared directory holds against its budget (D217 §5).
	Prepared *PreparedBody `json:"prepared,omitempty"`
	// Disks are the filesystems its roots and prepared directory are on.
	Disks []DiskBody `json:"disks"`
	// RecentPrepares are its latest finished prepares, newest first, each with what made it.
	RecentPrepares []FinishedPrepareBody `json:"recentPrepares"`
}

// ConfinementBody is one mechanism on a node: in force, or not, why, and what would put it there.
type ConfinementBody struct {
	Mechanism string `enum:"landlock,seccomp" json:"mechanism"`
	IsInForce bool   `json:"isInForce"`
	// Cause names why it is not in force, for a client to say in its viewer's words.
	Cause  string `enum:"disabled_at_boot,kernel_without,blocked_by_runtime,not_linux,architecture_unwritten,unexplained" json:"cause,omitempty"` //nolint:lll // one struct tag
	Reason string `json:"reason,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

// HardwareBody is a node's hardware encoding: what it sees, whether it may open it, what worked
// and what it chose (D225 §3).
type HardwareBody struct {
	GPUs []GPUBody `json:"gpus"`
	// Trials are each hardware encoder's trial at startup, confined as a prepare is.
	Trials []EncoderTrialBody `json:"trials"`
	// Encoder is what its encodes use.
	Encoder string `enum:"h264_qsv,h264_nvenc,x264" json:"encoder"`
}

// GPUBody is one GPU device a node found.
type GPUBody struct {
	Device string `json:"device"`
	Driver string `json:"driver,omitempty"`
	// NoAccess is why the node's user may not open it, in the system's words; absent when it may.
	NoAccess string `json:"noAccess,omitempty"`
}

// EncoderTrialBody is one hardware encoder's trial: it worked, or why not, in FFmpeg's words.
type EncoderTrialBody struct {
	Encoder string `enum:"h264_qsv,h264_nvenc" json:"encoder"`
	Failure string `json:"failure,omitempty"`
}

// PreparedBody is a node's prepared directory against its budget and its disk's limit.
type PreparedBody struct {
	Bytes            int64 `json:"bytes"`
	BudgetBytes      int64 `json:"budgetBytes"`
	DiskPercent      int   `json:"diskPercent"`
	DiskLimitPercent int   `json:"diskLimitPercent"`
}

// FinishedPrepareBody is one finished prepare: its file, how it ended, what made its video and how
// fast.
type FinishedPrepareBody struct {
	FileID int64  `json:"fileId"`
	Path   string `doc:"Empty when the library no longer has the file" json:"path"`
	// Profile is what was made; State how it ended.
	Profile      string  `enum:"remux,h264-8m" json:"profile"`
	State        string  `enum:"complete,failed" json:"state"`
	VideoEncoder string  `doc:"h264_qsv, h264_nvenc, x264, or copy for a remux" json:"videoEncoder,omitempty"`
	Speed        float64 `doc:"Seconds of the source prepared per second of running" json:"speed,omitempty"`
	SizeBytes    int64   `json:"sizeBytes,omitempty"`
	Error        string  `json:"error,omitempty"`
	FinishedAt   int64   `doc:"Unix milliseconds" json:"finishedAt"`
}

func (server *Server) registerSystem() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getSystem", Method: http.MethodGet, Path: "/system", Tags: []string{tagRunningIt},
		Summary: "Every fact about the server and each node, with doctor's findings: the System page (admin; D225)",
	}, server.system)
}

func (server *Server) system(ctx context.Context, input *AuthorizationHeader) (*SystemOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &SystemOutput{}
	output.Body.Server, err = serverFacts(ctx, dependencies.System)
	if err != nil {
		return nil, server.internal(ctx, "measure the databases", err)
	}
	output.Body.Findings, err = server.findings(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	listed, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list nodes", err)
	}
	output.Body.Nodes = make([]SystemNodeBody, 0, len(listed))
	for _, node := range listed {
		body := SystemNodeBody{
			Name: node.Name, Confinement: []ConfinementBody{}, Disks: []DiskBody{},
			RecentPrepares: []FinishedPrepareBody{},
		}
		if facts, found := dependencies.NodeReports.Facts(node.Name); found {
			body.RecentPrepares, err = finishedPrepares(ctx, dependencies, facts.NodeID)
			if err != nil {
				return nil, server.internal(ctx, "list a node's prepares", err)
			}
			describeNode(&body, facts)
		}
		output.Body.Nodes = append(output.Body.Nodes, body)
	}
	return output, nil
}

// serverFacts are the server's version, platform, disks and database sizes.
func serverFacts(ctx context.Context, system System) (ServerBody, error) {
	facts := ServerBody{
		Version: system.Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(),
		StartedAt: system.StartedAt.UnixMilli(),
		Disks:     []DiskBody{measured(system.StateDirectory), measured(system.CacheDirectory)},
	}
	for _, database := range []struct {
		name  string
		sized Sized
	}{{"library.db", system.Library}, {"cache.db", system.Cache}} {
		size, err := database.sized.SizeBytes(ctx)
		if err != nil {
			return ServerBody{}, fmt.Errorf("measure %s: %w", database.name, err)
		}
		facts.Databases = append(facts.Databases, DatabaseBody{
			Name: database.name, SizeBytes: int64(size), //nolint:gosec // a database's size fits
		})
	}
	return facts, nil
}

// measured is the disk that holds path, or why it could not be measured.
func measured(path string) DiskBody {
	space, err := diskspace.Measure(path)
	if err != nil {
		return DiskBody{Path: path, Failure: err.Error()}
	}
	return DiskBody{Path: path, TotalBytes: space.TotalBytes, FreeBytes: space.FreeBytes}
}

// describeNode fills in what a connected node last said about itself.
func describeNode(body *SystemNodeBody, facts nodes.Facts) {
	hello := facts.Hello
	heard := facts.LastHeard.UnixMilli()
	body.IsConnected, body.LastHeardAt = true, &heard
	body.Version, body.Platform, body.FFmpegVersion, body.FFmpegHash = hello.Version, hello.Platform,
		hello.FFmpegVersion, hello.FFmpegHash
	body.UID, body.GID = hello.UID, hello.GID
	body.Confinement = confinementBodies(hello.Confinement)
	body.Hardware = hardwareBody(hello.Hardware)
	if facts.Status == nil {
		return
	}
	if usage := facts.Status.Prepared; usage != nil {
		body.Prepared = &PreparedBody{
			Bytes: usage.Bytes, BudgetBytes: usage.BudgetBytes, DiskPercent: usage.DiskPercent,
			DiskLimitPercent: usage.DiskLimitPercent,
		}
	}
	for _, disk := range facts.Status.Disks {
		body.Disks = append(body.Disks, DiskBody{
			Path: disk.Path, TotalBytes: disk.TotalBytes, FreeBytes: disk.FreeBytes, Failure: disk.Failure,
		})
	}
}

// confinementBodies are a node's confinement as the API shows it.
func confinementBodies(reported []nodeproto.Confinement) []ConfinementBody {
	bodies := make([]ConfinementBody, 0, len(reported))
	for _, state := range reported {
		bodies = append(bodies, ConfinementBody{
			Mechanism: state.Mechanism, IsInForce: state.InForce, Cause: state.Cause, Reason: state.Reason,
			Remedy: state.Remedy,
		})
	}
	return bodies
}

func hardwareBody(hardware nodeproto.Hardware) *HardwareBody {
	body := &HardwareBody{GPUs: []GPUBody{}, Trials: []EncoderTrialBody{}, Encoder: hardware.Encoder}
	for _, gpu := range hardware.GPUs {
		body.GPUs = append(body.GPUs, GPUBody{Device: gpu.Device, Driver: gpu.Driver, NoAccess: gpu.NoAccess})
	}
	for _, trial := range hardware.Trials {
		body.Trials = append(body.Trials, EncoderTrialBody{Encoder: trial.Encoder, Failure: trial.Failure})
	}
	return body
}

func finishedPrepares(ctx context.Context, dependencies *Dependencies, nodeID int64) ([]FinishedPrepareBody, error) {
	finished, err := dependencies.Playback.FinishedPrepares(ctx, nodeID, recentPrepares)
	if err != nil {
		return nil, fmt.Errorf("list node %d's prepares: %w", nodeID, err)
	}
	bodies := make([]FinishedPrepareBody, 0, len(finished))
	for _, prepare := range finished {
		bodies = append(bodies, FinishedPrepareBody{
			FileID: prepare.FileID, Path: prepare.Path, Profile: prepare.Profile, State: prepare.State,
			VideoEncoder: prepare.VideoEncoder, Speed: prepare.Speed, SizeBytes: prepare.SizeBytes,
			Error: prepare.Error, FinishedAt: prepare.FinishedAt.UnixMilli(),
		})
	}
	return bodies, nil
}

// hardwareFinding is whether a node's encodes use its GPU: a warning when it has one and no
// hardware encoder works, with what to do about it (D225 §3).
func hardwareFinding(node string, hardware nodeproto.Hardware) FindingBody {
	finding := FindingBody{Check: "hardware encoding on node " + node, Verdict: verdictOK}
	switch {
	case hardware.Encoder != "" && hardware.Encoder != "x264":
		finding.Detail = "encodes use " + hardware.Encoder
	case len(hardware.GPUs) == 0:
		finding.Detail = "no GPU found; encodes use x264, on the processor"
	default:
		finding.Verdict = verdictWarn
		var why []string
		for _, trial := range hardware.Trials {
			why = append(why, trial.Encoder+": "+trial.Failure)
		}
		finding.Detail = "this node has a GPU, but no hardware encoder works on it, so encodes use x264: " +
			strings.Join(why, "; ")
		finding.Action = "see the node's hardware on the System page"
		for _, gpu := range hardware.GPUs {
			if gpu.NoAccess != "" {
				finding.Action = fmt.Sprintf("let the node's user open %s: add it to the group that owns it "+
					"(usually render or video), then restart the node", gpu.Device)
				break
			}
		}
	}
	return finding
}
