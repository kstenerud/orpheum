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
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/platform/host"
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
	// Build is what the server's binary was built from.
	Build     build.Provenance
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
	Build     BuildBody      `json:"build"`
	Host      HostBody       `json:"host"`
	GoVersion string         `json:"goVersion"`
	StartedAt int64          `doc:"Unix milliseconds" json:"startedAt"`
	Disks     []DiskBody     `json:"disks"`
	Databases []DatabaseBody `json:"databases"`
}

// BuildBody is what a binary was built from (D225).
type BuildBody struct {
	// Name is the build in a word: its tag, or dev and its short commit, and -modified.
	Name string `json:"name"`
	// Version is the release tag its commit carries, or dev.
	Version     string `json:"version"`
	Commit      string `doc:"Empty when the build recorded none" json:"commit,omitempty"`
	IsModified  bool   `doc:"Built from a tree with uncommitted changes" json:"isModified"`
	CommittedAt int64  `doc:"Unix milliseconds; absent when unrecorded" json:"committedAt,omitempty"`
	// BinarySHA256 is the binary's hash, which tells two builds of one modified tree apart.
	BinarySHA256 string `json:"binarySha256,omitempty"`
}

// HostBody is a machine's operating system, as lsb_release -a shows it, and its kernel.
type HostBody struct {
	Distribution string `json:"distribution,omitempty"`
	Description  string `json:"description,omitempty"`
	Release      string `json:"release,omitempty"`
	Codename     string `json:"codename,omitempty"`
	Kernel       string `json:"kernel,omitempty"`
	Platform     string `doc:"Operating system and architecture, as Go names them" json:"platform"`
}

func buildBody(info build.Provenance) BuildBody {
	return BuildBody{
		Name: info.String(), Version: info.Version, Commit: info.Commit, IsModified: info.IsModified,
		CommittedAt: info.CommittedAt, BinarySHA256: info.BinarySHA256,
	}
}

func hostBody(info host.System) HostBody {
	return HostBody{
		Distribution: info.Distribution, Description: info.Description, Release: info.Release,
		Codename: info.Codename, Kernel: info.Kernel, Platform: info.Platform,
	}
}

// DiskBody is one filesystem, named by a directory on it.
type DiskBody struct {
	Path           string `json:"path"`
	TotalBytes     int64  `json:"totalBytes"`
	AvailableBytes int64  `doc:"What a user who is not root may still write" json:"availableBytes"`
	Failure        string `doc:"Why it could not be measured; the sizes are zero then" json:"failure,omitempty"`
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
	LastHeardAt *int64 `json:"lastHeardAt,omitempty"`
	// Build is what its binary was built from; IsServersBuild whether that is the server's own.
	Build          *BuildBody `json:"build,omitempty"`
	IsServersBuild bool       `json:"isServersBuild"`
	Host           *HostBody  `json:"host,omitempty"`
	FFmpegVersion  string     `json:"ffmpegVersion,omitempty"`
	FFmpegSha256   string     `json:"ffmpegSha256,omitempty"`
	UID            int        `doc:"The user the node runs as" json:"uid"`
	GID            int        `json:"gid"`
	// Confinement is how its FFmpeg runs are confined, per mechanism (D112).
	Confinement []ConfinementBody `json:"confinement"`
	// Hardware is what it found for hardware encoding; absent while it is not connected.
	Hardware *HardwareBody `json:"hardware,omitempty"`
	// PreparedUsage is what its prepared directory holds against its budget (D217 §5).
	PreparedUsage *PreparedBody `json:"preparedUsage,omitempty"`
	// Disks are the filesystems its roots and prepared directory are on.
	Disks []DiskBody `json:"disks"`
	// RecentPrepares are its latest finished prepares, newest first, each with what made it.
	RecentPrepares []FinishedPrepareBody `json:"recentPrepares"`
}

// ConfinementBody is one mechanism on a node: in force, or not, why, and what would put it there.
type ConfinementBody struct {
	Mechanism string `enum:"landlock,seccomp" json:"mechanism"`
	IsInForce bool   `json:"isInForce"`
	// CauseCode names why it is not in force, for a client to say in its viewer's words.
	CauseCode string `enum:"disabled_at_boot,kernel_without,blocked_by_runtime,not_linux,architecture_unwritten,unexplained" json:"causeCode,omitempty"` //nolint:lll // one struct tag
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
}

// HardwareBody is a node's hardware encoding: what it sees, whether it may open it, what worked
// and what it chose (D225 §3).
type HardwareBody struct {
	GPUs []GPUBody `json:"gpus"`
	// Trials are each hardware encoder's trial at startup, confined as a prepare is.
	Trials []EncoderTrialBody `json:"trials"`
	// ChosenEncoder is what its encodes use.
	ChosenEncoder string `enum:"h264_qsv,h264_nvenc,x264" json:"chosenEncoder"`
}

// GPUBody is one GPU device a node found.
type GPUBody struct {
	DevicePath string `json:"devicePath"`
	Driver     string `json:"driver,omitempty"`
	// NoAccessReason is why the node's user may not open it, in the system's words; absent when it may.
	NoAccessReason string `json:"noAccessReason,omitempty"`
}

// EncoderTrialBody is one hardware encoder's trial: it worked, or why not, in FFmpeg's words.
type EncoderTrialBody struct {
	Encoder string `enum:"h264_qsv,h264_nvenc" json:"encoder"`
	Failure string `json:"failure,omitempty"`
}

// PreparedBody is a node's prepared directory against its budget and its disk's limit.
type PreparedBody struct {
	UsedBytes            int64 `json:"usedBytes"`
	BudgetBytes          int64 `json:"budgetBytes"`
	DiskUsedPercent      int   `json:"diskUsedPercent"`
	DiskUsedLimitPercent int   `json:"diskUsedLimitPercent"`
}

// FinishedPrepareBody is one finished prepare: its file, how it ended, what made its video and how
// fast.
type FinishedPrepareBody struct {
	SourceFileID int64  `json:"sourceFileId"`
	Path         string `doc:"Empty when the library no longer has the file" json:"path"`
	// Profile is what was made; State how it ended.
	Profile        string  `enum:"remux,h264-8m" json:"profile"`
	State          string  `enum:"complete,failed" json:"state"`
	VideoEncoder   string  `doc:"h264_qsv, h264_nvenc, x264, or copy for a remux" json:"videoEncoder,omitempty"`
	RealtimeFactor float64 `doc:"Seconds of the source prepared per second of running" json:"realtimeFactor,omitempty"`
	SizeBytes      int64   `json:"sizeBytes,omitempty"`
	Failure        string  `json:"failure,omitempty"`
	FinishedAt     int64   `doc:"Unix milliseconds" json:"finishedAt"`
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
			describeNode(&body, facts, dependencies.System.Build)
		}
		output.Body.Nodes = append(output.Body.Nodes, body)
	}
	return output, nil
}

// serverFacts are the server's version, platform, disks and database sizes.
func serverFacts(ctx context.Context, system System) (ServerBody, error) {
	facts := ServerBody{
		Build: buildBody(system.Build), Host: hostBody(host.Describe()), GoVersion: runtime.Version(),
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
	return DiskBody{Path: path, TotalBytes: space.TotalBytes, AvailableBytes: space.AvailableBytes}
}

// describeNode fills in what a connected node last said about itself.
func describeNode(body *SystemNodeBody, facts nodes.Facts, serverBuild build.Provenance) {
	hello := facts.Hello
	heard := facts.LastHeard.UnixMilli()
	nodeBuild, nodeHost := buildBody(hello.Build), hostBody(hello.Host)
	body.IsConnected, body.LastHeardAt = true, &heard
	body.Build, body.IsServersBuild, body.Host = &nodeBuild, hello.Build.IsSame(serverBuild), &nodeHost
	body.FFmpegVersion, body.FFmpegSha256 = hello.FFmpegVersion, hello.FFmpegSha256
	body.UID, body.GID = hello.UID, hello.GID
	body.Confinement = confinementBodies(hello.Confinement)
	body.Hardware = hardwareBody(hello.Hardware)
	if facts.Status == nil {
		return
	}
	if usage := facts.Status.PreparedUsage; usage != nil {
		body.PreparedUsage = &PreparedBody{
			UsedBytes: usage.UsedBytes, BudgetBytes: usage.BudgetBytes, DiskUsedPercent: usage.DiskUsedPercent,
			DiskUsedLimitPercent: usage.DiskUsedLimitPercent,
		}
	}
	for _, disk := range facts.Status.Disks {
		body.Disks = append(body.Disks, DiskBody{
			Path: disk.Path, TotalBytes: disk.TotalBytes, AvailableBytes: disk.AvailableBytes, Failure: disk.Failure,
		})
	}
}

// confinementBodies are a node's confinement as the API shows it.
func confinementBodies(reported []nodeproto.Confinement) []ConfinementBody {
	bodies := make([]ConfinementBody, 0, len(reported))
	for _, state := range reported {
		bodies = append(bodies, ConfinementBody{
			Mechanism: state.Mechanism, IsInForce: state.IsInForce, CauseCode: state.CauseCode, Reason: state.Reason,
			Remedy: state.Remedy,
		})
	}
	return bodies
}

func hardwareBody(hardware nodeproto.Hardware) *HardwareBody {
	body := &HardwareBody{GPUs: []GPUBody{}, Trials: []EncoderTrialBody{}, ChosenEncoder: hardware.ChosenEncoder}
	for _, gpu := range hardware.GPUs {
		body.GPUs = append(body.GPUs, GPUBody{
			DevicePath: gpu.DevicePath, Driver: gpu.Driver,
			NoAccessReason: gpu.NoAccessReason,
		})
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
			SourceFileID: prepare.FileID, Path: prepare.Path, Profile: prepare.Profile, State: prepare.State,
			VideoEncoder: prepare.VideoEncoder, RealtimeFactor: prepare.Speed, SizeBytes: prepare.SizeBytes,
			Failure: prepare.Error, FinishedAt: prepare.FinishedAt.UnixMilli(),
		})
	}
	return bodies, nil
}

// buildFinding is whether a node runs the server's own build: a node on another release, or a
// build of other changes, may not speak the same protocol or behave the same (D225).
func buildFinding(node string, nodeBuild, serverBuild build.Provenance) FindingBody {
	finding := FindingBody{
		Check: "build on node " + node, Verdict: verdictOK, Detail: "the server's own build, " + serverBuild.String(),
	}
	if !nodeBuild.IsSame(serverBuild) {
		finding.Verdict = verdictWarn
		finding.Detail = fmt.Sprintf("the node runs %s, the server %s", nodeBuild.String(), serverBuild.String())
		finding.Action = "install the server's build on the node, or the node's on the server, and restart both"
		if nodeBuild.Commit != "" && nodeBuild.Commit == serverBuild.Commit {
			finding.Detail += "; both are built from one commit with uncommitted changes, which may differ"
		}
	}
	return finding
}

// hardwareFinding is whether a node's encodes use its GPU: a warning when it has one and no
// hardware encoder works, with what to do about it (D225 §3).
func hardwareFinding(node string, hardware nodeproto.Hardware) FindingBody {
	finding := FindingBody{Check: "hardware encoding on node " + node, Verdict: verdictOK}
	switch {
	case hardware.ChosenEncoder != "" && hardware.ChosenEncoder != "x264":
		finding.Detail = "encodes use " + hardware.ChosenEncoder
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
			if gpu.NoAccessReason != "" {
				finding.Action = fmt.Sprintf("let the node's user open %s: add it to the group that owns it "+
					"(usually render or video), then restart the node", gpu.DevicePath)
				break
			}
		}
	}
	return finding
}
