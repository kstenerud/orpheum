// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/problemreport"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A running core keeps its problem reports: a report holds the lines the process's log kept before
// it, and one past its 60 days goes as the core starts, though no one lists it (D524 §3, §4).
func TestServe_AProblemReportHoldsTheCoresRecentLogAndAnOldOneGoes(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	seedPersistentDatabase(t, config)
	databasePath := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	storetest.ExecRaw(t, databasePath, `INSERT INTO problem_report (user_id, made_at, version, words, gathered_json)
		VALUES (NULL, 1, 'v0.8.4', 'long ago', '{"components":[]}')`)
	recent := slogx.NewRecent(slogx.ProcessRecentLimits())
	processLogger, err := slogx.NewKeeping(&strings.Builder{}, slogx.FormatJSON, slog.LevelWarn, recent)
	if err != nil {
		t.Fatal(err)
	}
	processLogger.Debug("the process's own line, below the level it writes at")
	captured := newCapturedLog("ready", problemreport.EventExpired)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := serveWithOn(t, listener, config, captured, "ready", serveAlongside{recent: recent})
	testutil.Receive(t, captured.signals[problemreport.EventExpired], "the old report to go")

	owner := signedInOwner(t, server, configPath)
	made := requestAPI(t, http.MethodPost, server.baseURL+"/problem-reports", owner,
		map[string]any{"words": "it stalls", "page": "/", "device": []any{}})
	var report struct {
		ID int64 `json:"id"`
	}
	if err = json.Unmarshal([]byte(made.body), &report); err != nil || made.status != http.StatusCreated {
		t.Fatalf("POST /problem-reports = %d %s", made.status, made.body)
	}
	path := server.baseURL + "/problem-reports/" + strconv.FormatInt(report.ID, 10) + "?form=full"
	downloaded := requestAPI(t, http.MethodGet, path, owner, nil)
	if downloaded.status != http.StatusOK || !strings.Contains(downloaded.body, "below the level it writes at") {
		t.Errorf("GET %s = %d; want the process's recent line in it:\n%s", path, downloaded.status, downloaded.body)
	}
	if rows := storetest.QueryRaw(t, databasePath, "SELECT words FROM problem_report"); len(rows) != 1 {
		t.Errorf("the reports kept are %v; want the new one alone", rows)
	}
}
