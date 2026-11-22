// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// The keeper's bounds (D524 §3, §4).
const (
	// KeptFor is how long a report is kept: it deletes itself after.
	KeptFor = 60 * 24 * time.Hour
	// NodeAnswerWithin is how long a report waits for each node's recent log; one slower is absent.
	NodeAnswerWithin = 5 * time.Second
	// PerPersonPerHour is how many reports one person may make in an hour: a report holds a day's
	// log, and a person pressing the button again and again should not fill the database.
	PerPersonPerHour = 10
	// A report's own salt is saltBytes long.
	saltBytes = 16
)

// ErrTooMany is a report its person may not make yet: they made PerPersonPerHour in the last hour.
var ErrTooMany = errors.New("problem report: too many in an hour")

// ErrNoReport is a report there is not, or no longer.
var ErrNoReport = errors.New("problem report: there is no such report")

// NodeLogs asks a node for its recent log: the core's byte socket client.
type NodeLogs interface {
	RecentLog(ctx context.Context, nodeID int64) ([]slogx.Line, error)
}

// NodeConnections says how the core last heard from a node: the node service.
type NodeConnections interface {
	NodeConnection(ctx context.Context, nodeID int64) (nodes.Connection, time.Time)
}

// Parts are what a Keeper is made of.
type Parts struct {
	Database *store.Database
	Clock    clock.Clock
	// Version is the core's Orpheum version.
	Version string
	// Recent is the core process's own recent log; nil keeps none, which a report says.
	Recent      *slogx.Recent
	NodeLogs    NodeLogs
	Connections NodeConnections
	// House is the household's realm name now, empty for none.
	House  func() string
	Logger *slog.Logger
}

// Keeper makes a household's problem reports, keeps them as gathered for its admin alone, and
// renders each download (D524). Safe for concurrent use.
type Keeper struct {
	parts Parts
}

// NewKeeper is a keeper of parts.
func NewKeeper(parts Parts) *Keeper {
	return &Keeper{parts: parts}
}

// Request is what the person's device sends for a report.
type Request struct {
	// UserID is the person making it; DeviceName the device they make it from, empty for none.
	UserID     int64
	DeviceName string
	// Words are their own; Page the path of the page they made it from.
	Words, Page string
	// Device are the device's facts, in its order.
	Device []Fact
	// Playback is the playback the report was made from, if any.
	Playback *PlaybackRequest
	// SoundOffset is the file the person says needs a sound offset and the one they set for
	// themselves, which the admin may make every person's (D537); nil for none.
	SoundOffset *SoundOffset
}

// SoundOffset is a report's suggestion that a file needs a sound offset (D537): the file, and the
// offset in milliseconds, later when positive, as a device's is.
type SoundOffset struct {
	FileID, OffsetMs int64
}

// PlaybackRequest is the player's state as the device sends it; the keeper adds the file's shape.
type PlaybackRequest struct {
	PlaybackID, ItemID, FileID int64
	Position                   time.Duration
	Quality, State             string
}

// Summary is a kept report as the admin's list names it.
type Summary struct {
	ID int64
	// UserName is who made it, empty once their account is deleted.
	UserName string
	MadeAt   time.Time
	Version  string
	Words    string
	// SoundOffset is the sound offset the report suggests for a file (D537); nil for none, or once
	// the file is deleted.
	SoundOffset *SoundOffset
}

// Make gathers a report, the core's recent log and each node's, and keeps it; it says the report's
// id. A node that does not answer within NodeAnswerWithin is kept absent, with why.
func (keeper *Keeper) Make(ctx context.Context, request Request) (int64, error) {
	now := keeper.parts.Clock.Now()
	made, err := keeper.parts.Database.Read().CountProblemReportsByAfter(ctx,
		sqlcgen.CountProblemReportsByAfterParams{UserID: &request.UserID, After: now.Add(-time.Hour).UnixMilli()})
	if err != nil {
		return 0, fmt.Errorf("count the person's problem reports: %w", err)
	}
	if made >= PerPersonPerHour {
		return 0, ErrTooMany
	}
	gathered := gatheredReport{
		Salt: NewSalt(), DeviceName: request.DeviceName, Page: request.Page, Components: keeper.components(ctx, now),
	}
	for _, fact := range request.Device {
		gathered.Device = append(gathered.Device, gatheredFact(fact))
	}
	if request.Playback != nil {
		gathered.Playback = keeper.playback(ctx, *request.Playback)
	}
	encoded, err := json.Marshal(gathered)
	if err != nil {
		return 0, fmt.Errorf("encode the problem report: %w", err)
	}
	var reportID int64
	err = keeper.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var added error
		params := sqlcgen.AddProblemReportParams{
			UserID: &request.UserID, MadeAt: now.UnixMilli(), Version: keeper.parts.Version, Words: request.Words,
			GatheredJSON: string(encoded),
		}
		if suggested := request.SoundOffset; suggested != nil {
			params.SoundFileID, params.SoundOffsetMs = &suggested.FileID, &suggested.OffsetMs
		}
		reportID, added = queries.AddProblemReport(ctx, params)
		return added
	})
	if err != nil {
		return 0, fmt.Errorf("keep the problem report: %w", err)
	}
	keeper.parts.Logger.InfoContext(ctx, "made a problem report", "reportId", reportID, "userId", request.UserID)
	return reportID, nil
}

// components are the core's recent log as of now and each live node's, asked at once.
func (keeper *Keeper) components(ctx context.Context, now time.Time) []gatheredComponent {
	core := gatheredComponent{Name: "core", Version: keeper.parts.Version}
	if keeper.parts.Recent == nil {
		core.Absent = "this core keeps no recent log"
	} else {
		var log nodeproto.RecentLog
		log.Add(keeper.parts.Recent.Lines(now)...)
		core.Lines = log.Lines
	}
	liveNodes, err := keeper.parts.Database.Read().ListLiveNodes(ctx)
	if err != nil {
		return []gatheredComponent{core, {Name: "every node", Node: true, Absent: "the nodes could not be listed: " +
			err.Error()}}
	}
	components := make([]gatheredComponent, 1+len(liveNodes))
	components[0] = core
	var asked sync.WaitGroup
	for index, node := range liveNodes {
		asked.Go(func() { components[1+index] = keeper.nodeComponent(ctx, now, node.ID, node.Name) })
	}
	asked.Wait()
	return components
}

// nodeComponent is one node's recent log, or why it gave none.
func (keeper *Keeper) nodeComponent(ctx context.Context, now time.Time, nodeID int64, name string) gatheredComponent {
	component := gatheredComponent{Name: name, Node: true}
	connection, heard := keeper.parts.Connections.NodeConnection(ctx, nodeID)
	if connection != nodes.Connected {
		component.Absent = absentReason(fmt.Sprintf("not connected (%s)", connection), now, heard)
		return component
	}
	asking, cancel := context.WithTimeout(ctx, NodeAnswerWithin)
	defer cancel()
	lines, err := keeper.parts.NodeLogs.RecentLog(asking, nodeID)
	if err != nil {
		component.Absent = absentReason(fmt.Sprintf("no recent log within %s: %v", NodeAnswerWithin, err), now, heard)
		return component
	}
	var log nodeproto.RecentLog
	log.Add(lines...)
	component.Lines = log.Lines
	return component
}

// absentReason is why a node gave no log, with when it was last heard.
func absentReason(why string, now, heard time.Time) string {
	if heard.IsZero() {
		return why + ", not heard since the core started"
	}
	return fmt.Sprintf("%s, last heard %s ago", why, now.Sub(heard).Round(time.Second))
}

// playback is the player's state with its file's shape, where the file is known and probed.
func (keeper *Keeper) playback(ctx context.Context, request PlaybackRequest) *gatheredPlayback {
	kept := &gatheredPlayback{
		PlaybackID: request.PlaybackID, ItemID: request.ItemID, FileID: request.FileID,
		PositionMs: request.Position.Milliseconds(), Quality: request.Quality, State: request.State,
	}
	if request.FileID <= 0 {
		return kept
	}
	file, err := keeper.parts.Database.Read().GetPlayableFile(ctx, request.FileID)
	if err != nil {
		return kept
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return kept
	}
	kept.File = &gatheredFile{Path: string(file.ReportedPathRaw), Probe: probe}
	if file.SizeBytes != nil {
		kept.File.SizeBytes = *file.SizeBytes
	}
	return kept
}

// List is every kept report, the newest first; one past KeptFor is not, though Expire has yet to
// delete it.
func (keeper *Keeper) List(ctx context.Context) ([]Summary, error) {
	rows, err := keeper.parts.Database.Read().ListProblemReports(ctx, keeper.keptSince())
	if err != nil {
		return nil, fmt.Errorf("list the problem reports: %w", err)
	}
	summaries := make([]Summary, 0, len(rows))
	for _, row := range rows {
		summary := Summary{ID: row.ID, MadeAt: time.UnixMilli(row.MadeAt).UTC(), Version: row.Version, Words: row.Words}
		if row.UserName != nil {
			summary.UserName = *row.UserName
		}
		if row.SoundFileID != nil && row.SoundOffsetMs != nil {
			summary.SoundOffset = &SoundOffset{FileID: *row.SoundFileID, OffsetMs: *row.SoundOffsetMs}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// Download renders one kept report in the form options name, the names the database holds now looked
// for in its free text; one past KeptFor is not there.
func (keeper *Keeper) Download(ctx context.Context, reportID int64, options Options) (Rendered, error) {
	row, err := store.OrNotFound(keeper.parts.Database.Read().GetProblemReport(ctx,
		sqlcgen.GetProblemReportParams{ID: reportID, Since: keeper.keptSince()}))
	if errors.Is(err, store.ErrNotFound) {
		return Rendered{}, ErrNoReport
	}
	if err != nil {
		return Rendered{}, fmt.Errorf("read problem report %d: %w", reportID, err)
	}
	var gathered gatheredReport
	if err = strictjson.Unmarshal([]byte(row.GatheredJSON), &gathered); err != nil {
		return Rendered{}, fmt.Errorf("read problem report %d's gathering: %w", reportID, err)
	}
	report := gathered.report(time.UnixMilli(row.MadeAt).UTC(), row.Version, row.Words)
	if row.UserName != nil {
		report.Reporter.UserName = *row.UserName
	}
	house := ""
	if keeper.parts.House != nil {
		house = keeper.parts.House()
	}
	if knownErr := AddKnown(ctx, keeper.parts.Database, house, &report); knownErr != nil {
		return Rendered{}, knownErr
	}
	return Render(report, options)
}

// NewSalt is a new report's own salt, random: every report has one (Report.Salt).
func NewSalt() []byte {
	salt := make([]byte, saltBytes)
	_, _ = rand.Read(salt) // crypto/rand.Read never fails (Go 1.24)
	return salt
}

// AddKnown sets report's Known to every name database holds that can say whose the report is, and
// house, the household's name, when not empty, and adds the other libraries' roots to its
// OtherRoots. A download from the System page and a report made on the command line (D524 §5) both
// look the names up by it.
func AddKnown(ctx context.Context, database *store.Database, house string, report *Report) error {
	names, err := database.Read().ListProblemReportKnownNames(ctx)
	if err != nil {
		return fmt.Errorf("list the names a problem report hides: %w", err)
	}
	report.Known = make([]Known, 0, len(names)+1)
	for _, name := range names {
		if name.Value == "" {
			continue
		}
		known := Known{Kind: Kind(name.Kind), Value: name.Value, Other: name.IsOther != 0}
		report.Known = append(report.Known, known)
		if known.Kind == KindPath && known.Other {
			report.OtherRoots = append(report.OtherRoots, known.Value)
		}
	}
	if house != "" {
		report.Known = append(report.Known, Known{Kind: KindHouse, Value: house})
	}
	return nil
}

// Delete deletes one kept report; false when there was none.
func (keeper *Keeper) Delete(ctx context.Context, reportID int64) (bool, error) {
	var deleted int64
	err := keeper.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deleting error
		deleted, deleting = queries.DeleteProblemReport(ctx, reportID)
		return deleting
	})
	if err != nil {
		return false, fmt.Errorf("delete problem report %d: %w", reportID, err)
	}
	return deleted > 0, nil
}

// keptSince is when the oldest report still kept was made, in Unix milliseconds.
func (keeper *Keeper) keptSince() int64 {
	return keeper.parts.Clock.Now().Add(-KeptFor).UnixMilli()
}

// EventExpired is the log's event of the reports Expire deleted.
const EventExpired = "problemReportsExpired"

// Expire deletes the reports made more than KeptFor ago (D524 §4).
func (keeper *Keeper) Expire(ctx context.Context) error {
	before := keeper.keptSince()
	var deleted int64
	err := keeper.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deleting error
		deleted, deleting = queries.DeleteProblemReportsMadeBefore(ctx, before)
		return deleting
	})
	if err != nil {
		return fmt.Errorf("delete the problem reports past their %s: %w", KeptFor, err)
	}
	if deleted > 0 {
		keeper.parts.Logger.InfoContext(ctx, "deleted problem reports past their keeping",
			slogx.EventKey, EventExpired, "count", deleted)
	}
	return nil
}

// Expiring deletes the reports past their keeping at each tick, until ctx ends, so a report no one
// lists still goes when its time is up.
func (keeper *Keeper) Expiring(ctx context.Context, tick <-chan time.Time) {
	for {
		if err := keeper.Expire(ctx); err != nil && ctx.Err() == nil {
			keeper.parts.Logger.WarnContext(ctx, "could not delete problem reports past their keeping", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}
