// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// An admin backs the server up from the admin interface into its backups directory and sees the
// backup listed; one with the key is refused while there is no key to put in it (D242 §5).
func TestBackups_AnAdminMakesOneAndSeesItListed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	anonymous := harness.do(t, http.MethodPost, "/backups", "", map[string]bool{"includesKey": false})
	if anonymous.status != http.StatusUnauthorized {
		t.Errorf("a backup asked for signed out answered %d; want 401", anonymous.status)
	}
	admin := harness.createAdminAndSignIn(t)
	made := harness.do(t, http.MethodPost, "/backups", admin, map[string]bool{"includesKey": false})
	var backup MadeBackupOutput
	if made.status != http.StatusCreated || json.Unmarshal([]byte(made.body), &backup.Body) != nil ||
		!strings.HasPrefix(backup.Body.Name, "backup-") || !strings.HasSuffix(backup.Body.Name, ".tar.gz") {
		t.Fatalf("POST /backups = %d %s; want a tarball made", made.status, made.body)
	}
	listed := harness.do(t, http.MethodGet, "/backups", admin, nil)
	var backups BackupsOutput
	if err := json.Unmarshal([]byte(listed.body), &backups.Body); err != nil || listed.status != http.StatusOK {
		t.Fatalf("GET /backups = %d %s", listed.status, listed.body)
	}
	if len(backups.Body.Backups) == 0 || backups.Body.Backups[0].Name != backup.Body.Name ||
		backups.Body.Backups[0].SizeBytes <= 0 || backups.Body.Directory == "" {
		t.Errorf("the backups listed = %+v; want the one made, newest first", backups.Body)
	}
	withKey := harness.do(t, http.MethodPost, "/backups", admin, map[string]bool{"includesKey": true})
	if withKey.status != http.StatusConflict || !strings.Contains(withKey.body, "no secret.key") {
		t.Errorf("a backup with a key there is not = %d %s; want 409 saying so", withKey.status, withKey.body)
	}
}

// A POST carrying an Idempotency-Key is done once: its repeat, as a retry of the same click sends,
// is given the first one's answer and makes no second backup; the key used for another body is
// refused (D244 §4).
func TestIdempotencyKey_ARepeatIsGivenTheFirstAnswerAndDoesNothingAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	click := http.Header{IdempotencyKeyHeader: []string{"click-1"}}
	first := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click, map[string]bool{"includesKey": false})
	harness.clock.Advance(time.Second) // a second backup would have a name of its own
	again := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click, map[string]bool{"includesKey": false})
	if first.status != http.StatusCreated || again.status != first.status || again.body != first.body {
		t.Errorf("the repeat answered %d %s; want the first's, %d %s", again.status, again.body, first.status,
			first.body)
	}
	var backups BackupsOutput
	listed := harness.do(t, http.MethodGet, "/backups", admin, nil)
	if err := json.Unmarshal([]byte(listed.body), &backups.Body); err != nil || len(backups.Body.Backups) != 1 {
		t.Errorf("the backups made: %s (%v); want one", listed.body, err)
	}
	other := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click, map[string]bool{"includesKey": true})
	if other.status != http.StatusConflict || !strings.Contains(other.body, "Idempotency-Key") {
		t.Errorf("the key used for another body answered %d %s; want 409", other.status, other.body)
	}
	harness.clock.Advance(idempotentAnswerKept + time.Second)
	if later := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click,
		map[string]bool{"includesKey": false}); later.status != http.StatusCreated || later.body == first.body {
		t.Errorf("past the answer's keeping, the key answered %d %s; want a new backup", later.status, later.body)
	}
}
