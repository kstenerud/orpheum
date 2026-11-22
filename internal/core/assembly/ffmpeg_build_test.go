// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// sayHello posts a node's hello, in this build's protocol, to the channel, and returns the
// node's id.
func sayHello(t *testing.T, channel *httptest.Server, name, ffmpegSha256 string) int64 {
	t.Helper()
	encoded, err := json.Marshal(nodeproto.Hello{
		Name: name, ProtocolVersion: nodeproto.ProtocolVersion, ByteSocketPath: "/run/orpheum-test/node.sock",
		FFmpegSha256: ffmpegSha256,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, channel.URL+nodeproto.PathHello,
		bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	response, err := channel.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only
	var reply nodeproto.HelloReply
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s's hello answered %d", name, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	return reply.NodeID
}

// givenUpAndMade are the given-up and successful rows a test keeps of one file's bytes: each is named
// "<kind> failed" or "<kind> made".
func givenUpAndMade(payload string) []string {
	return []string{
		`INSERT INTO loudness (payload_hash, stream_index, measured_mix, recipe, failure, measured_at)
			VALUES (` + payload + `, 1, 'stereo', 1, 'ffmpeg exited 1', 1)`,
		`INSERT INTO loudness (payload_hash, stream_index, measured_mix, recipe, integrated_lufs, range_lu,
			true_peak_dbtp, measured_at) VALUES (` + payload + `, 2, 'stereo', 1, -23, 7, -1, 1)`,
		`INSERT INTO video_timeline (payload_hash, video_stream_index, failure, made_at)
			VALUES (` + payload + `, 0, 'ffmpeg exited 1', 1)`,
		`INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, made_at, read_failure)
			VALUES (` + payload + `, 1, '[0,2000]', 1, 'the read stopped')`,
		`INSERT INTO repeat_measurement (payload_hash, version, outcome, failure, measured_at)
			VALUES (` + payload + `, 1, 'failed', 'ffmpeg exited 1', 1)`,
		`INSERT INTO repeat_measurement (payload_hash, version, outcome, measured_at)
			VALUES (` + payload + `, 2, 'none', 1)`,
		`INSERT INTO subtitle_text (source_hash, stream_index, failure, made_at)
			VALUES (` + payload + `, 3, 'ffmpeg exited 1', 1)`,
	}
}

// keptOf names what is kept of a file: each FFmpeg failure, what succeeded, and its whole-file hash's
// failure, which is not FFmpeg's.
func keptOf(t *testing.T, path string, fileID int64) []string {
	t.Helper()
	id := strconv.FormatInt(fileID, 10)
	payload := `(SELECT payload_hash FROM file WHERE id = ` + id + `)`
	kept := storetest.QueryRaw(t, path, `
		SELECT 'loudness failed' FROM loudness WHERE failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'loudness made' FROM loudness WHERE failure IS NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'timeline failed' FROM video_timeline
			WHERE failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'timeline read failed' FROM video_timeline
			WHERE read_failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'keyframes made' FROM video_timeline
			WHERE keyframes_ms_json = '[0,2000]' AND payload_hash = `+payload+`
		UNION ALL SELECT 'repeats failed' FROM repeat_measurement
			WHERE outcome = 'failed' AND payload_hash = `+payload+`
		UNION ALL SELECT 'repeats made' FROM repeat_measurement WHERE outcome = 'none' AND payload_hash = `+payload+`
		UNION ALL SELECT 'subtitle text failed' FROM subtitle_text
			WHERE failure IS NOT NULL AND source_hash = `+payload+`
		UNION ALL SELECT 'whole hash failed' FROM whole_file_hash WHERE failure IS NOT NULL AND file_id = `+id)
	slices.Sort(kept)
	return kept
}

// D535 §9, as the server is built: a node that says hello with another FFmpeg build forgets, by each
// table's own package, the failures FFmpeg made of its files (loudness, the picture timeline, repeat
// measurements, subtitle text), and keeps what succeeded, a whole-file hash's failure and every
// failure of another node's files.
func TestBuild_ANodesNewFFmpegForgetsWhatFFmpegFailedAtOfItsFiles(t *testing.T) {
	t.Parallel()
	parts := testParts(t)
	services, err := Build(t.Context(), parts)
	if err != nil {
		t.Fatal(err)
	}
	channel := httptest.NewServer(services.NodeChannel.Handler())
	t.Cleanup(channel.Close)
	nas := sayHello(t, channel, "nas", "build-a")
	attic := sayHello(t, channel, "attic", "build-a")
	path := parts.Database.Path()
	storetest.ExecRaw(t, path,
		rowstest.File(rowstest.FileSpec{ID: 11, NodeID: nas, Path: "/m/a.mkv", PayloadHash: []byte{1}}),
		rowstest.File(rowstest.FileSpec{ID: 21, NodeID: attic, Path: "/m/b.mkv", PayloadHash: []byte{2}}),
		`INSERT INTO whole_file_hash (file_id, failure, hashed_at) VALUES (11, 'the read stopped', 1)`,
		`INSERT INTO whole_file_hash (file_id, failure, hashed_at) VALUES (21, 'the read stopped', 1)`)
	storetest.ExecRaw(t, path, givenUpAndMade("X'01'")...)
	storetest.ExecRaw(t, path, givenUpAndMade("X'02'")...)
	everything := keptOf(t, path, 21)
	sayHello(t, channel, "nas", "build-b")
	want := []string{"keyframes made", "loudness made", "repeats made", "whole hash failed"}
	if kept := keptOf(t, path, 11); !slices.Equal(kept, want) {
		t.Errorf("of the node's file, its new FFmpeg kept %v; want %v", kept, want)
	}
	if kept := keptOf(t, path, 21); len(kept) != 9 || !slices.Equal(kept, everything) {
		t.Errorf("of another node's file, its new FFmpeg kept %v; want all of %v", kept, everything)
	}
}

// D535 §9: once a node's new FFmpeg forgot the failures FFmpeg made of its files, it asks for the
// sweeps that make each again: the read-throughs (loudness and the picture timeline) and the seasons'
// repeats.
func TestAskForTheFFmpegSweeps_AsksForTheReadThroughsAndTheRepeats(t *testing.T) {
	t.Parallel()
	sweeps := &requestedSweeps{}
	askForTheFFmpegSweeps(sweeps)()
	if want := []string{sweepReadThroughs, sweepRepeats}; !slices.Equal(sweeps.names, want) {
		t.Errorf("a node's new FFmpeg asked for the sweeps %v; want %v", sweeps.names, want)
	}
}
