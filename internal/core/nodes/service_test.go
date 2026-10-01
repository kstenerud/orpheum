// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingSink keeps every batch it was handed, and says when one arrives.
type recordingSink struct {
	mutex   sync.Mutex
	batches [][]nodeproto.Record
	arrived chan struct{}
}

func (sink *recordingSink) IngestRecords(_ context.Context, _ int64, records []nodeproto.Record) error {
	sink.mutex.Lock()
	sink.batches = append(sink.batches, append([]nodeproto.Record(nil), records...))
	sink.mutex.Unlock()
	select {
	case sink.arrived <- struct{}{}:
	default:
	}
	return nil
}

func (sink *recordingSink) all() []nodeproto.Record {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()
	var records []nodeproto.Record
	for _, batch := range sink.batches {
		records = append(records, batch...)
	}
	return records
}

// recordingDestinations are the job queue's parked destinations, keeping each one unparked.
type recordingDestinations struct {
	mutex    sync.Mutex
	unparked []jobs.Destination
}

func (destinations *recordingDestinations) Unpark(_ context.Context, destination jobs.Destination) {
	destinations.mutex.Lock()
	defer destinations.mutex.Unlock()
	destinations.unparked = append(destinations.unparked, destination)
}

func (destinations *recordingDestinations) all() []jobs.Destination {
	destinations.mutex.Lock()
	defer destinations.mutex.Unlock()
	return slices.Clone(destinations.unparked)
}

type fixture struct {
	database     *store.Database
	settings     *settings.Service
	service      *Service
	sink         *recordingSink
	destinations *recordingDestinations
	server       *httptest.Server
	clock        *clocktest.Fake
}

func newFixture(t *testing.T, pollHold time.Duration) *fixture {
	t.Helper()
	database := storetest.OpenMigrated(t)
	settingsService := settingstest.Seeded(t, database)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	sink := &recordingSink{arrived: make(chan struct{}, 1)}
	destinations := &recordingDestinations{}
	service := NewService(ServiceParts{
		Database: database, Clock: fakeClock, Settings: settingsService, Sink: sink, Destinations: destinations,
		Logger: slog.New(slog.DiscardHandler), PollHold: pollHold,
	})
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	return &fixture{
		database: database, settings: settingsService, service: service, sink: sink, destinations: destinations,
		server: server, clock: fakeClock,
	}
}

// httpAnswer is a response, read and closed.
type httpAnswer struct {
	status int
	body   []byte
}

func (harness *fixture) post(t *testing.T, path string, nodeID int64, body io.Reader) httpAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, harness.server.URL+path, body)
	if err != nil {
		t.Error(err)
		return httpAnswer{}
	}
	if nodeID != 0 {
		request.Header.Set(NodeIDHeader, strconv.FormatInt(nodeID, 10))
	}
	response, err := harness.server.Client().Do(request)
	if err != nil {
		t.Error(err)
		return httpAnswer{}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Error(err)
	}
	return httpAnswer{status: response.StatusCode, body: read}
}

func (harness *fixture) hello(t *testing.T, hello nodeproto.Hello) nodeproto.HelloReply {
	t.Helper()
	encoded, err := json.Marshal(hello)
	if err != nil {
		t.Fatal(err)
	}
	response := harness.post(t, nodeproto.PathHello, 0, bytes.NewReader(encoded))
	if response.status != http.StatusOK {
		t.Fatalf("hello answered %d %s", response.status, response.body)
	}
	var reply nodeproto.HelloReply
	if err := json.Unmarshal(response.body, &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func (harness *fixture) poll(t *testing.T, nodeID, after int64) (nodeproto.Commands, int) {
	t.Helper()
	url := fmt.Sprintf("%s%s?%s=%d", harness.server.URL, nodeproto.PathCommands, nodeproto.QueryAfterCommandSeq, after)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(NodeIDHeader, strconv.FormatInt(nodeID, 10))
	response, err := harness.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only
	var commands nodeproto.Commands
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&commands); err != nil {
			t.Fatal(err)
		}
	}
	return commands, response.StatusCode
}

func TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRoot(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	first := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	again := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	other := harness.hello(t, nodeproto.Hello{Name: "attic", RootPaths: nil})
	if first.NodeID == 0 || again.NodeID != first.NodeID || other.NodeID == first.NodeID {
		t.Errorf("node ids %d, %d, %d: want one id per name (D207)", first.NodeID, again.NodeID, other.NodeID)
	}

	commands, status := harness.poll(t, first.NodeID, 0)
	var roots []string
	for _, command := range commands.Commands {
		if command.Type != nodeproto.CommandScan {
			t.Errorf("command %+v, want a scan", command)
			continue
		}
		roots = append(roots, command.Scan.RootPath)
	}
	if status != http.StatusOK || strings.Join(roots, ",") != "/media/films,/media/shows,/media/films" {
		t.Errorf("status %d, scans of %q; want one scan per root named in each hello", status, roots)
	}
	if seqs := []int64{commands.Commands[0].CommandSeq, commands.Commands[2].CommandSeq}; seqs[0] != 1 || seqs[1] != 3 {
		t.Errorf("command numbers %v, want 1 then 3", seqs)
	}
}

// What a node's machine can do is its config's, said in its hello: the core adds up the prepare
// slots of the nodes that said hello, and knows how fast each reads.
func TestHello_TellsTheCoreEachNodesSlotsAndHowFastItReads(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	var told []int
	harness.service.SetPrepareSlotsSink(func(prepareSlotCount int) { told = append(told, prepareSlotCount) })
	local := harness.hello(t, nodeproto.Hello{Name: "local", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 1, RemuxSlotCount: 2, SlowestReadBytesPerSecond: 20 << 20,
	}})
	harness.hello(t, nodeproto.Hello{Name: "attic", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 2, RemuxSlotCount: 4, SlowestReadBytesPerSecond: 5 << 20,
	}})
	harness.hello(t, nodeproto.Hello{Name: "local", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 1, RemuxSlotCount: 3, SlowestReadBytesPerSecond: 40 << 20,
	}})
	if !slices.Equal(told, []int{3, 9, 10}) {
		t.Errorf("the prepare slots told after each hello: %v, want 3, 9, then 10 when local came back with more", told)
	}
	if rate := harness.service.SlowestReadBytesPerSecond(local.NodeID); rate != 40<<20 {
		t.Errorf("local reads at %d bytes a second, want its last hello's 40 MiB", rate)
	}
	if rate := harness.service.SlowestReadBytesPerSecond(99); rate != 0 {
		t.Errorf("a node that has not said hello reads at %d, want 0: not known", rate)
	}
}

func TestHello_ScansNothingWhenTheSettingSaysNot(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	if commands, _ := harness.poll(t, reply.NodeID, 0); len(commands.Commands) != 0 {
		t.Errorf("commands %+v, want none", commands.Commands)
	}
}

func scanOf(root string) nodeproto.Command {
	return nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{RootPath: root}}
}

func TestCommands_ALongPollWaitsForACommandAndAnAcknowledgedOneIsDropped(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})

	got := make(chan nodeproto.Commands)
	go func() {
		commands, _ := harness.poll(t, reply.NodeID, 0)
		got <- commands
	}()
	// The poll is held until a command is queued; Enqueue before the poll starts is also
	// answered at once, so the order of these two does not matter.
	harness.service.Enqueue(reply.NodeID, scanOf("/a"))
	first := testutil.Receive(t, got, "the long-poll's answer")
	if len(first.Commands) != 1 || first.Commands[0].Scan.RootPath != "/a" {
		t.Fatalf("the long-poll answered %+v, want the scan of /a", first)
	}
	harness.service.Enqueue(reply.NodeID, scanOf("/b"))
	commands, _ := harness.poll(t, reply.NodeID, 1)
	if len(commands.Commands) != 1 || commands.Commands[0].CommandSeq != 2 {
		t.Errorf("after acknowledging 1: %+v, want only command 2", commands.Commands)
	}
}

func TestCommands_ALongPollWithNothingToSayAnswersEmpty(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	commands, status := harness.poll(t, reply.NodeID, 0)
	if status != http.StatusOK || commands.Commands == nil || len(commands.Commands) != 0 {
		t.Errorf("status %d, %+v; want 200 and an empty list", status, commands)
	}
}

func TestConnectedNode_ANodeMustSayHelloFirst(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, status := harness.poll(t, 99, 0); status != http.StatusConflict {
		t.Errorf("a poll from an unknown node: %d, want 409", status)
	}
	response := harness.post(t, nodeproto.PathReport, 0, strings.NewReader(""))
	if response.status != http.StatusBadRequest {
		t.Errorf("a report without the node header: %d, want 400", response.status)
	}
}

func jsonl(t *testing.T, records []nodeproto.Record) io.Reader {
	t.Helper()
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	return &body
}

func TestServeReport_HandsRecordsToTheSinkInOrderAndRecordsHowFarItGot(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	records := make([]nodeproto.Record, 0, 300)
	for index := range 299 {
		records = append(records, nodeproto.Record{
			ReportSeq: reply.LastIngestedReportSeq + int64(index) + 1, Type: nodeproto.RecordSkip,
			Skip: &nodeproto.Skip{Path: nodeproto.Path{Text: fmt.Sprintf("f%d", index)}, Rule: nodeproto.SkipExtension},
		})
	}
	records = append(records, nodeproto.Record{
		ReportSeq: 300, Type: nodeproto.RecordStatus,
		Status: &nodeproto.Status{SentAt: 1, Activity: "idle"},
	})
	response := harness.post(t, nodeproto.PathReport, reply.NodeID, jsonl(t, records))
	if response.status != http.StatusNoContent {
		t.Fatalf("the report answered %d %s", response.status, response.body)
	}
	got := harness.sink.all()
	if len(got) != 300 || got[0].ReportSeq != 1 || got[299].ReportSeq != 300 {
		t.Fatalf("the sink got %d records", len(got))
	}
	// How the body is split in transit decides where quiet moments fall, so assert the bounds:
	// batched, and never more than batchRecords at once.
	if len(harness.sink.batches) > 30 {
		t.Errorf("%d batches for 300 records; the ingest is not batched", len(harness.sink.batches))
	}
	for _, batch := range harness.sink.batches {
		if len(batch) > maxBatchRecords {
			t.Errorf("a batch of %d, over %d", len(batch), maxBatchRecords)
		}
	}
	node, err := harness.database.Read().GetNodeStatus(t.Context(), reply.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if node.LastStatusJSON == nil || !strings.Contains(*node.LastStatusJSON, `"activity":"idle"`) {
		t.Errorf("the node's status is %v, want the last status record", node.LastStatusJSON)
	}
	if again := harness.hello(t, nodeproto.Hello{Name: "local"}); again.LastIngestedReportSeq != 300 {
		t.Errorf("a reconnecting node is told %d, want 300: it continues numbering from there",
			again.LastIngestedReportSeq)
	}
	if heard, ever := harness.service.LastHeard("local"); !ever || !heard.Equal(harness.clock.Now()) {
		t.Errorf("LastHeard = %v, %v", heard, ever)
	}
}

func TestServeReport_RefusesARecordOutOfSequence(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	records := []nodeproto.Record{
		{ReportSeq: 5, Type: nodeproto.RecordStatus, Status: &nodeproto.Status{Activity: "idle"}},
		{ReportSeq: 5, Type: nodeproto.RecordStatus, Status: &nodeproto.Status{Activity: "idle"}},
	}
	response := harness.post(t, nodeproto.PathReport, reply.NodeID, jsonl(t, records))
	if response.status != http.StatusBadRequest || !strings.Contains(string(response.body), "only rises") {
		t.Errorf("a repeated sequence number: %d %q, want a 400 saying why", response.status, response.body)
	}
}

func TestServeReport_ARecordReachesTheIngestWhileTheReportIsStillOpen(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	reader, writer := io.Pipe()
	done := make(chan httpAnswer, 1)
	go func() { done <- harness.post(t, nodeproto.PathReport, reply.NodeID, reader) }()
	record := nodeproto.Record{
		ReportSeq: 1, Type: nodeproto.RecordScanBegin,
		ScanBegin: &nodeproto.ScanBegin{RootPath: "/m"},
	}
	if err := json.NewEncoder(writer).Encode(record); err != nil {
		t.Fatal(err)
	}
	// The stream stays open and nothing else is written: the record must still arrive, because a
	// quiet stream is flushed rather than held for a full batch.
	testutil.Receive(t, harness.sink.arrived, "the record to reach the ingest")
	if got := harness.sink.all(); len(got) != 1 || got[0].Type != nodeproto.RecordScanBegin {
		t.Errorf("the ingest has %+v, want the scan_begin", got)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if response := testutil.Receive(t, done, "the report's answer"); response.status != http.StatusNoContent {
		t.Errorf("the report answered %d %s", response.status, response.body)
	}
}

func TestConnectedNodeRoots_AreWhatTheNodesLastHelloNamed(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	if _, _, connected := harness.service.ConnectedNodeRoots(t.Context(), "local"); connected {
		t.Error("a node that never said hello has roots")
	}
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	nodeID, roots, connected := harness.service.ConnectedNodeRoots(t.Context(), "local")
	if !connected || nodeID != reply.NodeID || !slices.Equal(roots, []string{"/media/films"}) {
		t.Errorf("Roots = %d, %v, %v; want the last hello's", nodeID, roots, connected)
	}
}

// recordingArtworkSink records the artwork answers it is handed.
type recordingArtworkSink struct {
	answers []nodeproto.ArtworkResult
	from    []int64
}

func (sink *recordingArtworkSink) TakeArtworkAnswer(_ context.Context, nodeID int64, result nodeproto.ArtworkResult) {
	sink.answers = append(sink.answers, result)
	sink.from = append(sink.from, nodeID)
}

func TestArtwork_AnAnswerReachesTheArtworkSinkFromAKnownNodeOnly(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	answerJSON := func(result nodeproto.ArtworkResult) io.Reader {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(encoded)
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	result := nodeproto.ArtworkResult{
		JobID: 4, Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: []byte("RIFF")}},
	}
	if refused := harness.post(t, nodeproto.PathArtwork, reply.NodeID, answerJSON(result)); refused.status !=
		http.StatusServiceUnavailable {
		t.Errorf("with no artwork sink: %d, want 503", refused.status)
	}
	sink := &recordingArtworkSink{}
	harness.service.SetArtworkSink(sink)
	if stranger := harness.post(t, nodeproto.PathArtwork, reply.NodeID+1, answerJSON(result)); stranger.status !=
		http.StatusConflict {
		t.Errorf("from a node that never said hello: %d, want 409", stranger.status)
	}
	taken := harness.post(t, nodeproto.PathArtwork, reply.NodeID, answerJSON(result))
	if taken.status != http.StatusOK || len(sink.answers) != 1 || sink.from[0] != reply.NodeID ||
		sink.answers[0].JobID != 4 || string(sink.answers[0].Images[0].WebP) != "RIFF" {
		t.Errorf("answered %d %s; sink has %+v", taken.status, taken.body, sink.answers)
	}
	if connected, found := harness.service.ConnectedNodeID(t.Context()); !found || connected != reply.NodeID {
		t.Errorf("Connected = %d, %v; want the node", connected, found)
	}
	ctx := t.Context()
	if !harness.service.IsConnected(ctx, reply.NodeID) || harness.service.IsConnected(ctx, reply.NodeID+1) {
		t.Error("IsConnected does not tell the node that said hello from one that did not")
	}
}

func TestHello_KeepsWhatTheNodeSaidAboutItsConfinement(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that never said hello has a confinement")
	}
	said := []nodeproto.Confinement{
		{Mechanism: "landlock", Reason: "not enabled at boot", Remedy: "add landlock to lsm="},
		{Mechanism: "seccomp", IsInForce: true},
	}
	harness.hello(t, nodeproto.Hello{Name: "local", Confinement: said})
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Confinement
	if !found || len(kept) != 2 || kept[0] != said[0] || kept[1] != said[1] {
		t.Errorf("the kept confinement = %+v, %v; want what the hello said (D112)", kept, found)
	}
}

func TestStatus_KeepsWhatTheNodeSaysItsPreparedDirectoryHolds(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	if facts, _ := harness.service.Facts("local"); facts.Status != nil {
		t.Error("a node that has sent no status has a prepared usage")
	}
	usage := nodeproto.PreparedUsage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	status := []nodeproto.Record{{
		ReportSeq: reply.LastIngestedReportSeq + 1, Type: nodeproto.RecordStatus,
		Status: &nodeproto.Status{Activity: "idle", PreparedUsage: &usage},
	}}
	if response := harness.post(t, nodeproto.PathReport, reply.NodeID, jsonl(t, status)); response.status !=
		http.StatusNoContent {
		t.Fatalf("the report answered %d %s", response.status, response.body)
	}
	facts, _ := harness.service.Facts("local")
	if facts.Status == nil || facts.Status.PreparedUsage == nil || *facts.Status.PreparedUsage != usage {
		t.Errorf("the kept status = %+v; want the prepared usage it said (D217 §5)", facts.Status)
	}
}

func TestFacts_KeepTheHellosHardwareAndTheLastStatussDisks(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that has not said hello has facts")
	}
	hardware := nodeproto.Hardware{
		GPUs:          []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128", Driver: "i915"}},
		Trials:        []nodeproto.EncoderTrial{{Encoder: "h264_qsv"}, {Encoder: "h264_nvenc", Failure: "no device"}},
		ChosenEncoder: "h264_qsv",
	}
	reply := harness.hello(t, nodeproto.Hello{
		Name: "local", Host: host.System{Platform: "linux/amd64", Codename: "trixie"}, Hardware: hardware,
	})
	disk := nodeproto.Disk{Path: "/media", TotalBytes: 1000, AvailableBytes: 400}
	status := []nodeproto.Record{{
		ReportSeq: reply.LastIngestedReportSeq + 1, Type: nodeproto.RecordStatus,
		Status: &nodeproto.Status{Activity: "idle", Disks: []nodeproto.Disk{disk}},
	}}
	if response := harness.post(t, nodeproto.PathReport, reply.NodeID, jsonl(t, status)); response.status !=
		http.StatusNoContent {
		t.Fatalf("the report answered %d %s", response.status, response.body)
	}
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Hardware
	if !found || facts.NodeID != reply.NodeID || facts.Hello.Host.Codename != "trixie" ||
		kept.ChosenEncoder != "h264_qsv" || len(kept.Trials) != 2 || kept.GPUs[0].Driver != "i915" {
		t.Errorf("the facts' hello is %+v, %v; want the host and hardware the hello said (D225)",
			facts.Hello, found)
	}
	if facts.Status == nil || len(facts.Status.Disks) != 1 || facts.Status.Disks[0] != disk {
		t.Errorf("the facts' status is %+v; want the disks the status said (D225)", facts.Status)
	}
}
