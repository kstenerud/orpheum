// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"encoding/json"
	"net/http"

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
	var answer nodeproto.RecentLog
	answer.Add(server.recent.Lines(server.recentClock.Now())...)
	encoded, err := json.Marshal(answer)
	if err != nil {
		http.Error(writer, "the answer could not be written", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	// The node's own JSON, on its private socket, to the core: no page renders it.
	_, _ = writer.Write(encoded) //nolint:errcheck // a core that hung up asks again
}
