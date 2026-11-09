// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/platform/host"
)

// NodeReports is what each connected node said about itself: nodes.Service.
type NodeReports interface {
	Facts(name string) (nodes.Facts, bool)
	// NodeConnection is whether a node is connected and when it was last heard (D231 §11).
	NodeConnection(ctx context.Context, nodeID int64) (nodes.Connection, time.Time)
	// DeviceStates are the devices a connected node's last hello listed, with their reader limits.
	DeviceStates(ctx context.Context, nodeID int64) ([]nodes.DeviceState, error)
	// SetReaderLimit sets how many may read one of a node's devices at once, or, nil, gives it back
	// to its kind's setting (D240 §5).
	SetReaderLimit(ctx context.Context, nodeName, deviceKey string, limit *int) error
	UnconfinedLeaves
}

// UnconfinedLeaves are the locked nodes and an admin's leave for each to run unconfined: nodes.Service
// (D450 §1).
type UnconfinedLeaves interface {
	// LockedConfinement is what a locked node said it lacks; false for one not locked (D450 §1).
	LockedConfinement(nodeID int64) ([]nodeproto.Confinement, time.Time, bool)
	// UnconfinedLeave is an admin's leave for a node to run without the confinement its kernel lacks;
	// false when there is none (D450 §1).
	UnconfinedLeave(ctx context.Context, nodeID int64) (nodes.UnconfinedLeave, bool, error)
	// AcceptUnconfined gives that leave, recording and logging who and when; nodes.ErrNoNode for a name
	// no live node has. It is not taken back: an admin who no longer wants the node to run stops it.
	AcceptUnconfined(ctx context.Context, nodeName string, userID int64) error
}

// NodeWork is what the core saw of the work it gave each node: nodes.Exchanges.
type NodeWork interface {
	// BrokenInvariants is how many times since the core started a node held work it had not given
	// it, or had ended (D234 §4).
	BrokenInvariants(nodeID int64) int
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
	// PersistentDirectory and CacheDirectory are the server's own, whose disks the page shows.
	PersistentDirectory string
	CacheDirectory      string
	// PersistentDatabase and CacheDatabase are persistent.db and cache.db, measured when the page asks.
	PersistentDatabase Sized
	CacheDatabase      Sized
	// PublicName is the name served over HTTPS, and Certificate its certificate's service; "" and
	// nil for a server with none (D243 §6).
	PublicName  string
	Certificate CertificateExpiry
	// MultipathTCPSetting is the kernel's MPTCP switch the page reads, host.MultipathTCPSetting on the
	// host's network, where it is the host's own; "" reads none (D454 §4).
	MultipathTCPSetting string
}

// CertificateExpiry says when the public name's certificate expires, and false while there is none.
type CertificateExpiry interface {
	ExpiresAt() (time.Time, bool)
}

// CertificateBody is the public name's HTTPS certificate: absent from the page when the server has
// no public name.
type CertificateBody struct {
	Name string `json:"name"`
	// ExpiresAt is when it expires, in Unix milliseconds; absent while none has been obtained. It is
	// renewed a month before (D243 §6).
	ExpiresAt *int64 `json:"expiresAt,omitempty"`
}

// SystemOutput is the System page: the server's facts, what doctor finds, and each node's facts
// (D225).
type SystemOutput struct {
	Body struct {
		Server   ServerBody       `json:"server"`
		Findings []FindingBody    `json:"findings"`
		Nodes    []SystemNodeBody `json:"nodes"`
		// Destinations are what work needs that is down now, each with why and when it is looked
		// at next (D231 §12); Providers each outside host's provider service (D231 §5).
		Destinations []DestinationBody `json:"destinations"`
		Providers    []ProviderBody    `json:"providers"`
		// Offline are the roots and disks scans have found missing, the longest offline first:
		// offline never ends without an admin, who retires one, or a scan that finds it (D41,
		// D231 §7).
		Offline []OfflineBody `json:"offline"`
		// Waiting are the library folders whose scans wait for the admin before removing files, the
		// longest waiting first; Unread what scans could not read (D277).
		Waiting []WaitingBody `json:"waiting"`
		Unread  []UnreadBody  `json:"unread"`
		// Queue is how each kind of job is doing: queued, the oldest waiting, and failed in the last
		// day (D233 §8).
		Queue []QueueKindBody `json:"queue"`
	}
}

// QueueKindBody is how one kind of job is doing.
type QueueKindBody struct {
	Kind        string `json:"kind"`
	QueuedCount int64  `json:"queuedCount"`
	// OldestQueuedAt is when the oldest of them was queued, in Unix milliseconds; absent when none
	// is queued.
	OldestQueuedAt *int64 `json:"oldestQueuedAt,omitempty"`
	// FailedLastDayCount is how many failed in the last 24 hours.
	FailedLastDayCount int64 `json:"failedLastDayCount"`
}

// NodeWorkBody is what a node is doing, as its last status said (D233 §8): how many of its slots
// run work, how many waits for one, and when its running work last moved.
type NodeWorkBody struct {
	RunningCount int `json:"runningCount"`
	// SlotCount is how many it runs at once, all its slots together.
	SlotCount    int `json:"slotCount"`
	WaitingCount int `json:"waitingCount"`
	// LastMovedAt is when its running work last moved, in Unix milliseconds; absent while nothing
	// runs.
	LastMovedAt *int64 `json:"lastMovedAt,omitempty"`
	// BrokenInvariantCount is how many times since the server started the node held work the core
	// had not given it, or had ended: each a fault, in the node or the core (D234 §4).
	BrokenInvariantCount int `json:"brokenInvariantCount"`
}

// OfflineBody is a root, or a disk mounted inside one, that scans have found missing, and since
// when.
type OfflineBody struct {
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	NodeName    string `json:"nodeName"`
	RootPath    string `json:"rootPath"`
	MountPath   string `doc:"The missing disk's mount point inside the root; absent when the root itself is offline" json:"mountPath,omitempty"` //nolint:lll // one struct tag
	// OfflineAt is when the first scan that found it missing ran, in Unix milliseconds.
	OfflineAt int64 `json:"offlineAt"`
}

// ServerBody is the server's own facts.
type ServerBody struct {
	Build     BuildBody      `json:"build"`
	Host      HostBody       `json:"host"`
	GoVersion string         `json:"goVersion"`
	StartedAt int64          `doc:"Unix milliseconds" json:"startedAt"`
	Disks     []DiskBody     `json:"disks"`
	Databases []DatabaseBody `json:"databases"`
	// Certificate is the public name's HTTPS certificate; absent for a server with no public name.
	Certificate *CertificateBody `json:"certificate,omitempty"`
	// IsMultipathTCPOn says the machine's kernel has MPTCP on. Orpheum opens no MPTCP socket, so it is
	// a note on what the machine's other programs may open, not a fault of the server's (D454 §4).
	IsMultipathTCPOn bool `json:"isMultipathTcpOn"`
	// AddressAtHome is the server's address in the house, which devices at home reach it at (D452 §3).
	AddressAtHome AddressAtHomeBody `json:"addressAtHome"`
}

// BuildBody is what a binary was built from (D225).
type BuildBody struct {
	// Name is the build in a word: its tag, or dev and its short commit, and -modified.
	Name string `json:"name"`
	// Version is the release tag its commit carries, or dev.
	Version               string `json:"version"`
	Commit                string `doc:"Empty when the build recorded none" json:"commit,omitempty"`
	HasUncommittedChanges bool   `doc:"Built from a tree with uncommitted changes" json:"hasUncommittedChanges"`
	CommittedAt           int64  `doc:"Unix milliseconds; absent when unrecorded" json:"committedAt,omitempty"`
	// BinarySha256 is the binary's hash, which tells two builds of one modified tree apart.
	BinarySha256 string `json:"binarySha256,omitempty"`
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
		Name: info.String(), Version: info.Version, Commit: info.Commit,
		HasUncommittedChanges: info.HasUncommittedChanges,
		CommittedAt:           info.CommittedAt, BinarySha256: info.BinarySha256,
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
	Name      string `enum:"persistent.db,cache.db" json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

// SystemNodeBody is one node's facts, as it last said them since the server started, and whether
// it is connected now (D231 §11); only its name and connection before it has said hello.
type SystemNodeBody struct {
	Name string `json:"name"`
	// Connection is connected (polling for its commands, or heard within
	// nodeSilentAfterSeconds), silent (heard since the server started, not lately),
	// awaiting_hello (not heard since the server started), needs_update (it speaks another version
	// of the node protocol than the server, so its work waits, D233 §9) or locked (its kernel lacks
	// confinement its config requires, so it runs no FFmpeg until an admin lets it, D450 §1).
	Connection  string `enum:"connected,silent,awaitingHello,needsUpdate,locked" json:"connection"`
	IsConnected bool   `doc:"Whether its connection is connected" json:"isConnected"`
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
	// Confinement is how its FFmpeg runs are confined, per mechanism (D112); a locked node's, what it
	// would get.
	Confinement []ConfinementBody `json:"confinement"`
	// UnconfinedLeave is an admin's leave for it to run FFmpeg without the confinement its kernel lacks,
	// absent when none was given (D450 §1).
	UnconfinedLeave *UnconfinedLeaveBody `json:"unconfinedLeave,omitempty"`
	// Hardware is what it found for hardware encoding; absent while it is not connected.
	Hardware *HardwareBody `json:"hardware,omitempty"`
	// PreparedUsage is what its prepared directory holds against its budget (D217 §5).
	PreparedUsage *PreparedBody `json:"preparedUsage,omitempty"`
	// Disks are the filesystems its roots and prepared directory are on.
	Disks []DiskBody `json:"disks"`
	// Devices are what its roots and prepared directory are on, with how many may read each at once
	// (D240 §5); none while it is not connected.
	Devices []NodeDeviceBody `json:"devices"`
	// Stuck is the work it gave up on that has not ended, waiting on a disk that does not answer
	// (D233 §10): a program the kernel will not stop until the disk answers, which holds no slot.
	Stuck []StuckBody `json:"stuck"`
	// Work is what it is doing; absent before its first status.
	Work *NodeWorkBody `json:"work,omitempty"`
}

// StuckBody is one piece of work a node gave up on that waits on a disk.
type StuckBody struct {
	// Program is the program waiting: ffmpeg, ffprobe, or the node's own reading of a folder.
	Program string `enum:"ffmpeg,ffprobe,node" json:"program"`
	// Paths are where it was reading: the disk under one of them is not answering.
	Paths []string `json:"paths"`
	// GaveUpAt is when the node gave up on it, in Unix milliseconds.
	GaveUpAt int64 `json:"gaveUpAt"`
}

// NodeDeviceBody is one of a node's devices: a disk, or a pool, array or volume over several.
type NodeDeviceBody struct {
	Key        string   `doc:"The node's name for it: zfs:tank, disk:sdc" json:"key"`
	Kind       string   `doc:"How it reads, as the node found it" enum:"spinning,ssd,network,unknown" json:"kind"`
	Filesystem string   `json:"filesystem"`
	Disks      []string `doc:"The whole disks it reads from; none when the node cannot see them" json:"disks"`
	// MountPaths are where it is mounted at or inside the node's roots and prepared directory.
	MountPaths []string `json:"mountPaths"`
	// ReaderLimit is how many may read it at once: the limit an admin set, else its kind's setting.
	ReaderLimit      int  `json:"readerLimit"`
	IsReaderLimitSet bool `doc:"Whether an admin set its reader limit, over its kind's" json:"isReaderLimitSet"`
}

// ConfinementBody is one mechanism on a node: in force, or not, why, and what would put it there.
type ConfinementBody struct {
	Mechanism string `enum:"landlock,seccomp" json:"mechanism"`
	IsInForce bool   `json:"isInForce"`
	// CauseCode names why it is not in force, for a client to say in its viewer's words.
	CauseCode string `enum:"disabledAtBoot,kernelWithout,blockedByRuntime,notLinux,architectureUnwritten,unexplained" json:"causeCode,omitempty"` //nolint:lll // one struct tag
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
	ChosenEncoder string `enum:"h264_qsv,h264_nvenc,h264_vaapi,x264" json:"chosenEncoder"`
	// EncodeSpeedPercent is how fast the chosen encoder made the lowest quality at the node's start,
	// in percent of real time; absent when its trial measured none (D457).
	EncodeSpeedPercent *int `json:"encodeSpeedPercent,omitempty"`
	// IsTooSlowToConvert is whether that is below chunkRunSlowestSpeedPercent, so the node's films
	// play only as they are or with their own picture copied, and a device that needs one converted
	// is refused (D457).
	IsTooSlowToConvert bool `json:"isTooSlowToConvert"`
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
	Encoder string `enum:"h264_qsv,h264_nvenc,h264_vaapi" json:"encoder"`
	Failure string `json:"failure,omitempty"`
}

// PreparedBody is a node's prepared directory against its budget and its disk's limit.
type PreparedBody struct {
	UsedBytes            int64 `json:"usedBytes"`
	BudgetBytes          int64 `json:"budgetBytes"`
	DiskUsedPercent      int   `json:"diskUsedPercent"`
	DiskUsedLimitPercent int   `json:"diskUsedLimitPercent"`
}

func (server *Server) registerSystem() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSystem", Method: http.MethodGet, Path: "/system", Tags: []string{tagRunningIt},
		Summary:    "Every fact about the server and each node, with doctor's findings: the System page (D225)",
		Extensions: callableBy(capabilityAdmin),
	}, server.getSystem)
	server.registerAddressAtHome()
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setReaderLimit", Method: http.MethodPut, Path: "/nodes/{name}/reader-limit",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary:    "Set how many may read one of a node's devices at once (D240)",
		Extensions: callableBy(capabilityAdmin),
	}, server.setReaderLimit)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "clearReaderLimit", Method: http.MethodDelete, Path: "/nodes/{name}/reader-limit",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary:    "Give one of a node's devices back to its kind's reader limit (D240)",
		Extensions: callableBy(capabilityAdmin),
	}, server.clearReaderLimit)
	server.registerResourceUse()
	server.registerUnconfined()
}

// DeviceReaderLimitInput sets one of a node's devices' reader limit.
type DeviceReaderLimitInput struct {
	AuthorizationHeader
	Name string `doc:"The node's configured name" minLength:"1" path:"name"`
	Body struct {
		// DeviceKey is the node's name for the device, as the System page lists it.
		DeviceKey string `json:"deviceKey" minLength:"1"`
		// ReaderLimit is how many may read it at once.
		ReaderLimit int `json:"readerLimit" maximum:"64" minimum:"1"`
	}
}

// ClearReaderLimitInput gives one of a node's devices back to its kind's reader limit.
type ClearReaderLimitInput struct {
	AuthorizationHeader
	Name      string `doc:"The node's configured name" minLength:"1" path:"name"`
	DeviceKey string `doc:"The node's name for the device, as the System page lists it" minLength:"1" query:"deviceKey" required:"true"` //nolint:lll // one struct tag
}

func (server *Server) setReaderLimit(ctx context.Context, input *DeviceReaderLimitInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	limit := input.Body.ReaderLimit
	return server.changeReaderLimit(ctx, dependencies, input.Name, input.Body.DeviceKey, &limit)
}

func (server *Server) clearReaderLimit(ctx context.Context, input *ClearReaderLimitInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.changeReaderLimit(ctx, dependencies, input.Name, input.DeviceKey, nil)
}

// changeReaderLimit sets a device's reader limit, or, nil, gives it back to its kind's.
func (server *Server) changeReaderLimit(
	ctx context.Context, dependencies *Dependencies, nodeName, deviceKey string, limit *int,
) (*struct{}, error) {
	err := dependencies.NodeReports.SetReaderLimit(ctx, nodeName, deviceKey, limit)
	if errors.Is(err, nodes.ErrNoNode) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such node")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "set a device's reader limit", err)
	}
	return &struct{}{}, nil
}

func (server *Server) getSystem(ctx context.Context, input *AuthorizationHeader) (*SystemOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &SystemOutput{}
	output.Body.Server, err = serverFacts(ctx, dependencies.System)
	if err != nil {
		return nil, server.logInternalError(ctx, "measure the databases", err)
	}
	output.Body.Server.AddressAtHome = server.addressAtHomeBody()
	output.Body.Findings, err = server.collectFindings(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	listed, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list nodes", err)
	}
	nodeName := server.nodeNamer(ctx, dependencies)
	output.Body.Destinations = make([]DestinationBody, 0)
	for _, state := range dependencies.Jobs.ParkedDestinations() {
		output.Body.Destinations = append(output.Body.Destinations, destinationBody(state, nodeName))
	}
	offline, err := dependencies.Libraries.Offline(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list what is offline", err)
	}
	output.Body.Offline = make([]OfflineBody, 0, len(offline))
	for _, missing := range offline {
		output.Body.Offline = append(output.Body.Offline, OfflineBody{
			LibraryID: missing.LibraryID, LibraryName: missing.LibraryName, NodeName: missing.NodeName,
			RootPath: missing.RootPath, MountPath: missing.MountPath, OfflineAt: missing.Since.UnixMilli(),
		})
	}
	if waitsErr := server.fillWhatWaits(ctx, dependencies, output); waitsErr != nil {
		return nil, waitsErr
	}
	health, err := dependencies.Jobs.Health(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "read the queue's health", err)
	}
	output.Body.Queue = queueBodies(health)
	output.Body.Providers = make([]ProviderBody, 0, len(dependencies.Providers))
	for _, provider := range dependencies.Providers {
		output.Body.Providers = append(output.Body.Providers, providerBody(provider.State(ctx), nodeName))
	}
	output.Body.Nodes = make([]SystemNodeBody, 0, len(listed))
	for _, node := range listed {
		body, nodeErr := server.systemNode(ctx, dependencies, node)
		if nodeErr != nil {
			return nil, nodeErr
		}
		output.Body.Nodes = append(output.Body.Nodes, body)
	}
	return output, nil
}

// systemNode is everything the System page shows of one node.
func (server *Server) systemNode(
	ctx context.Context, dependencies *Dependencies, node library.NodeRoots,
) (SystemNodeBody, error) {
	body := SystemNodeBody{
		Name: node.Name, Confinement: []ConfinementBody{}, Disks: []DiskBody{},
		Devices: []NodeDeviceBody{}, Stuck: []StuckBody{},
	}
	devices, err := dependencies.NodeReports.DeviceStates(ctx, node.ID)
	if err != nil {
		return body, server.logInternalError(ctx, "list a node's devices", err)
	}
	for _, device := range devices {
		body.Devices = append(body.Devices, NodeDeviceBody{
			Key: device.Key, Kind: string(device.Kind), Filesystem: device.Filesystem,
			Disks: append([]string{}, device.Disks...), MountPaths: append([]string{}, device.MountPaths...),
			ReaderLimit: device.ReaderLimit, IsReaderLimitSet: device.IsReaderLimitSet,
		})
	}
	connection, lastHeard := dependencies.NodeReports.NodeConnection(ctx, node.ID)
	body.Connection, body.IsConnected = string(connection), connection == nodes.Connected
	if !lastHeard.IsZero() {
		body.LastHeardAt = new(lastHeard.UnixMilli())
	}
	leave, isGiven, err := dependencies.NodeReports.UnconfinedLeave(ctx, node.ID)
	if err != nil {
		return body, server.logInternalError(ctx, "read a node's leave to run unconfined", err)
	}
	if isGiven {
		body.UnconfinedLeave = &UnconfinedLeaveBody{
			AcceptedBy: leave.AcceptedBy, AcceptedAt: leave.AcceptedAt.UnixMilli(),
		}
	}
	// A locked node says only what it lacks: what it was before is not what it is (D450 §1).
	if lacking, _, isLocked := dependencies.NodeReports.LockedConfinement(node.ID); isLocked &&
		connection == nodes.Locked {
		body.Confinement = confinementBodies(lacking)
		return body, nil
	}
	facts, found := dependencies.NodeReports.Facts(node.Name)
	if !found {
		return body, nil
	}
	fillNodeFacts(&body, facts, dependencies.System.Build)
	if body.Hardware != nil && dependencies.Chunks != nil {
		body.Hardware.IsTooSlowToConvert, err = dependencies.Chunks.IsTooSlowToEncode(ctx, facts.NodeID)
		if err != nil {
			return body, server.logInternalError(ctx, "judge a node's encode speed", err)
		}
	}
	if body.Work != nil && dependencies.NodeWork != nil {
		body.Work.BrokenInvariantCount = dependencies.NodeWork.BrokenInvariants(facts.NodeID)
	}
	return body, nil
}

// serverFacts are the server's version, platform, disks and database sizes.
func serverFacts(ctx context.Context, system System) (ServerBody, error) {
	facts := ServerBody{
		Build: buildBody(system.Build), Host: hostBody(host.Describe()), GoVersion: runtime.Version(),
		StartedAt: system.StartedAt.UnixMilli(),
		Disks:     []DiskBody{measureDisk(system.PersistentDirectory), measureDisk(system.CacheDirectory)},
	}
	if system.MultipathTCPSetting != "" {
		facts.IsMultipathTCPOn = host.IsMultipathTCPOn(system.MultipathTCPSetting)
	}
	if system.PublicName != "" && system.Certificate != nil {
		facts.Certificate = &CertificateBody{Name: system.PublicName}
		if expiresAt, isObtained := system.Certificate.ExpiresAt(); isObtained {
			facts.Certificate.ExpiresAt = new(expiresAt.UnixMilli())
		}
	}
	for _, database := range []struct {
		name     string
		measurer Sized
	}{{"persistent.db", system.PersistentDatabase}, {"cache.db", system.CacheDatabase}} {
		size, err := database.measurer.SizeBytes(ctx)
		if err != nil {
			return ServerBody{}, fmt.Errorf("measure %s: %w", database.name, err)
		}
		facts.Databases = append(facts.Databases, DatabaseBody{
			Name: database.name, SizeBytes: int64(size), //nolint:gosec // a database's size fits
		})
	}
	return facts, nil
}

// measureDisk measures the disk that holds path, or says why it could not be measured.
func measureDisk(path string) DiskBody {
	space, err := diskspace.Measure(path)
	if err != nil {
		return DiskBody{Path: path, Failure: err.Error()}
	}
	return DiskBody{Path: path, TotalBytes: space.TotalBytes, AvailableBytes: space.AvailableBytes}
}

// fillNodeFacts fills in what a connected node last said about itself.
func fillNodeFacts(body *SystemNodeBody, facts nodes.Facts, serverBuild build.Provenance) {
	hello := facts.Hello
	nodeBuild, nodeHost := buildBody(hello.Build), hostBody(hello.Host)
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
	for _, stuck := range facts.Status.Stuck {
		body.Stuck = append(body.Stuck, StuckBody{Program: stuck.Tool, Paths: stuck.Paths, GaveUpAt: stuck.Since})
	}
	body.Work = nodeWorkBody(hello.Capacity, *facts.Status)
}

// nodeWorkBody is what a node of capacity is doing, as its status says.
func nodeWorkBody(capacity nodeproto.Capacity, status nodeproto.Status) *NodeWorkBody {
	work := &NodeWorkBody{}
	for _, slot := range capacity.Slots() {
		work.SlotCount += capacity.SlotCount(slot)
	}
	var leastSinceMoved *int64
	for _, held := range status.Held {
		switch held.State {
		case nodeproto.HeldRunning:
			work.RunningCount++
			if leastSinceMoved == nil || held.SinceMovedMs < *leastSinceMoved {
				leastSinceMoved = new(held.SinceMovedMs)
			}
		case nodeproto.HeldWaiting, nodeproto.HeldPaused:
			work.WaitingCount++
		case nodeproto.HeldEnded:
		}
	}
	if leastSinceMoved != nil {
		work.LastMovedAt = new(status.SentAt - *leastSinceMoved)
	}
	return work
}

// queueBodies are the queue's health as the System page shows it.
func queueBodies(health []jobs.KindHealth) []QueueKindBody {
	bodies := make([]QueueKindBody, 0, len(health))
	for _, kind := range health {
		body := QueueKindBody{
			Kind: string(kind.Kind), QueuedCount: kind.QueuedCount, FailedLastDayCount: kind.FailedCount,
		}
		if !kind.OldestQueuedAt.IsZero() {
			body.OldestQueuedAt = new(kind.OldestQueuedAt.UnixMilli())
		}
		bodies = append(bodies, body)
	}
	return bodies
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
	if hardware.EncodeSpeedPercent > 0 {
		body.EncodeSpeedPercent = new(hardware.EncodeSpeedPercent)
	}
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

// nodeBuildFinding is whether a node runs the server's own build: a node on another release, or a
// build of other changes, may not speak the same protocol or behave the same (D225).
func nodeBuildFinding(nodeName string, nodeBuild, serverBuild build.Provenance) FindingBody {
	finding := FindingBody{
		Check: "build on node " + nodeName, Verdict: VerdictOK,
		Detail: "the server's own build, " + serverBuild.String(),
	}
	if !nodeBuild.IsSame(serverBuild) {
		finding.Verdict = VerdictWarning
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
func hardwareFinding(nodeName string, hardware nodeproto.Hardware) FindingBody {
	finding := FindingBody{Check: "hardware encoding on node " + nodeName, Verdict: VerdictOK}
	switch {
	case hardware.ChosenEncoder != "" && hardware.ChosenEncoder != "x264":
		finding.Detail = "encodes use " + hardware.ChosenEncoder
	case len(hardware.GPUs) == 0:
		finding.Detail = "no GPU found; encodes use x264, on the processor"
	default:
		finding.Verdict = VerdictWarning
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
