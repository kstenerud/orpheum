// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// chromeMKV is a start-playback body for fileID that a browser playing Matroska directly sends.
func chromeMKV(fileID int64) map[string]any {
	return map[string]any{
		"fileId": fileID, "playableFormats": map[string]any{
			"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"}, "subtitleFormats": []string{},
			"containers": []string{"mkv"},
		},
	}
}

// A play of a film still being read is refused as still reading, and has the server read it before
// what else waits, so the film plays soon after: the server wires the play to the scans (D304 §4).
func TestServe_APlayOfAFilmStillBeingReadHasItReadAndThenPlays(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	server, token, filmFileID := playableFilm(t, shelved)
	database := openDatabase(t, shelved.config)
	storetest.ExecRaw(t, database.Path(), "UPDATE file SET payload_hash = NULL WHERE path LIKE '%.mkv'")
	refused := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, chromeMKV(filmFileID))
	if refused.status != http.StatusServiceUnavailable || !strings.Contains(refused.body, "still_reading") {
		t.Fatalf("a play of a film still being read = %d %s; want refused, still reading", refused.status, refused.body)
	}
	pollUntil(t, "the film read and playable", func() bool {
		played := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, chromeMKV(filmFileID))
		return played.status == http.StatusOK
	})
}

// A film a person confirmed, copied to another folder and deleted where it was, is listed at a new
// inode, stored unread and filed from its new path; its read shows the film stored before, which
// keeps its row, its confirmed title and its history, and the title the unread row was filed in is
// merged into it rather than left live with no file. The server wires the ingest to identification
// for it (D304 §5).
func TestServe_AConfirmedFilmCopiedElsewhereAndDeletedKeepsItsTitleAndNoOther(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	server, token, filmFileID := playableFilm(t, shelved)
	database := openDatabase(t, shelved.config)
	libraries, err := database.Read().ListLiveLibraryIDs(t.Context())
	if err != nil || len(libraries) != 1 {
		t.Fatalf("libraries %v, %v; want the one", libraries, err)
	}
	library := libraries[0]
	storetest.ExecRaw(t, database.Path(), "UPDATE title SET identification_state = 'confirmed'")
	copied := filepath.Join(shelved.media, "Another Name (2021)", "Another Name (2021).mkv")
	if err = os.MkdirAll(filepath.Dir(copied), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(copied, []byte("film"), 0o600); err != nil { // a copy: a new inode
		t.Fatal(err)
	}
	//nolint:forbidigo // the test's own temporary media, deleted as a person would delete a film
	if err = os.RemoveAll(filepath.Join(shelved.media, "Film (2020)")); err != nil {
		t.Fatal(err)
	}
	scanned := requestAPI(t, http.MethodPost, server.baseURL+"/libraries/"+strconv.FormatInt(library, 10)+"/scan",
		token, nil)
	if scanned.status != http.StatusAccepted {
		t.Fatalf("POST scan = %d %s", scanned.status, scanned.body)
	}
	pollUntil(t, "the copy read where it is, its film's row kept", func() bool {
		moved := storetest.QueryRaw(t, database.Path(), "SELECT path FROM file WHERE id = "+
			strconv.FormatInt(filmFileID, 10)+" AND removed_at IS NULL")
		return len(moved) == 1 && strings.HasSuffix(moved[0], "Another Name (2021).mkv")
	})
	pollUntil(t, "one live title, the confirmed one", func() bool {
		live := storetest.QueryRaw(t, database.Path(),
			"SELECT identification_state FROM title JOIN item ON item.id = title.id WHERE item.removed_at IS NULL")
		return len(live) == 1 && live[0] == "confirmed"
	})
}

// Opening the page of a film still being read has the server read it before what else waits, so
// Play works by the time it is pressed, with no play asked (the owner, 2026-10-17: Nosferatu's page said
// still reading, and could not be played). The server wires the page to the scans (D304 §4).
func TestServe_OpeningTheFilmStillBeingReadsPageHasItRead(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	server, token, filmFileID := playableFilm(t, shelved)
	database := openDatabase(t, shelved.config)
	storetest.ExecRaw(t, database.Path(), "UPDATE file SET payload_hash = NULL WHERE path LIKE '%.mkv'")
	titles := storetest.QueryRaw(t, database.Path(), "SELECT version.title_id FROM file JOIN version ON "+
		"version.id = file.version_id WHERE file.id = "+strconv.FormatInt(filmFileID, 10))
	if len(titles) != 1 {
		t.Fatalf("the film's title: %v", titles)
	}
	page := requestAPI(t, http.MethodGet, server.baseURL+"/items/"+titles[0], token, nil)
	if page.status != http.StatusOK || !strings.Contains(page.body, "still_reading") {
		t.Fatalf("the film's page = %d %s; want it, still reading", page.status, page.body)
	}
	pollUntil(t, "the film read, its page only opened", func() bool {
		read := storetest.QueryRaw(t, database.Path(), "SELECT payload_hash IS NOT NULL FROM file WHERE id = "+
			strconv.FormatInt(filmFileID, 10))
		return len(read) == 1 && read[0] == "1"
	})
}
