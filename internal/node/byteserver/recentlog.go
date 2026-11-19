// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// ServingRecentLog has the server answer nodeproto.PathBytesRecentLog from the node process's recent
// log, its lines as of clockSource's now (D524 §3); without it the route answers 404, as a node of
// protocol 64 would.
func (server *Server) ServingRecentLog(recent *slogx.Recent, clockSource clock.Clock) *Server {
	server.recent, server.recentClock = recent, clockSource
	return server
}

// serveRecentLog answers nodeproto.PathBytesRecentLog: every line the node kept, outside every queue
// and without touching a disk, so a node whose disks do not answer still says what it logged.
func (server *Server) serveRecentLog(writer http.ResponseWriter, _ *http.Request) {
	if server.recent == nil {
		http.Error(writer, "this node keeps no recent log", http.StatusNotFound)
		return
	}
	answer := nodeproto.RecentLog{Lines: logLinesOf(server.recent.Lines(server.recentClock.Now()))}
	encoded, err := json.Marshal(answer)
	if err != nil {
		http.Error(writer, "the answer could not be written", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	// The node's own JSON, on its private socket, to the core: no page renders it.
	_, _ = writer.Write(encoded) //nolint:errcheck // a core that hung up asks again
}

// logLinesOf are lines as the node sends them.
func logLinesOf(lines []slogx.Line) []nodeproto.LogLine {
	sent := make([]nodeproto.LogLine, 0, len(lines))
	for _, line := range lines {
		fields := make([]nodeproto.LogField, 0, len(line.Fields))
		for _, field := range line.Fields {
			fields = append(fields, logFieldOf(field))
		}
		sent = append(sent, nodeproto.LogLine{
			Time: line.Time, Level: int(line.Level), Message: line.Message, Fields: fields,
		})
	}
	return sent
}

// logFieldOf is field written out with its kind.
func logFieldOf(field slogx.Field) nodeproto.LogField {
	value := field.Value
	kind, text := nodeproto.LogValueString, value.String()
	switch value.Kind() {
	case slog.KindInt64:
		kind, text = nodeproto.LogValueInt, strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		kind, text = nodeproto.LogValueUint, strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		kind, text = nodeproto.LogValueFloat, strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindBool:
		kind, text = nodeproto.LogValueBool, strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		kind, text = nodeproto.LogValueDuration, strconv.FormatInt(int64(value.Duration()), 10)
	case slog.KindTime:
		kind, text = nodeproto.LogValueTime, value.Time().Format(time.RFC3339Nano)
	case slog.KindString, slog.KindAny, slog.KindGroup, slog.KindLogValuer:
	}
	return nodeproto.LogField{Key: field.Key, Kind: kind, Value: text}
}
