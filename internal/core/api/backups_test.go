// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
	if made.status != http.StatusAccepted {
		t.Fatalf("POST /backups = %d %s; want it queued", made.status, made.body)
	}
	if queued := harness.do(t, http.MethodGet, "/jobs", admin, nil); !strings.Contains(queued.body, `"kind":"backup"`) {
		t.Errorf("the job view after a backup was asked for: %s; want the backup's job (D243 §11)", queued.body)
	}
	harness.runBackups(t)
	listed := harness.do(t, http.MethodGet, "/backups", admin, nil)
	var backups BackupsOutput
	if err := json.Unmarshal([]byte(listed.body), &backups.Body); err != nil || listed.status != http.StatusOK {
		t.Fatalf("GET /backups = %d %s", listed.status, listed.body)
	}
	if len(backups.Body.Backups) != 1 || !strings.HasPrefix(backups.Body.Backups[0].Name, "backup-") ||
		!strings.HasSuffix(backups.Body.Backups[0].Name, ".tar.gz") || backups.Body.Backups[0].SizeBytes <= 0 ||
		backups.Body.Directory == "" {
		t.Errorf("the backups listed = %+v; want the one its job made", backups.Body)
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
	harness.runBackups(t)
	harness.clock.Advance(time.Second) // a second backup would have a name of its own
	again := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click, map[string]bool{"includesKey": false})
	harness.runBackups(t)
	if first.status != http.StatusAccepted || again.status != first.status || again.body != first.body {
		t.Errorf("the repeat answered %d %s; want the first's, %d %s", again.status, again.body, first.status,
			first.body)
	}
	if count := harness.backupCount(t, admin); count != 1 {
		t.Errorf("a click and its repeat made %d backups; want one", count)
	}
	other := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click, map[string]bool{"includesKey": true})
	if other.status != http.StatusConflict || !strings.Contains(other.body, "Idempotency-Key") {
		t.Errorf("the key used for another body answered %d %s; want 409", other.status, other.body)
	}
	harness.clock.Advance(idempotentAnswerKept + time.Second)
	if later := harness.doWithHeader(t, http.MethodPost, "/backups", admin, click,
		map[string]bool{"includesKey": false}); later.status != http.StatusAccepted {
		t.Errorf("past the answer's keeping, the key answered %d %s; want a new backup asked for", later.status,
			later.body)
	}
	harness.runBackups(t)
	if count := harness.backupCount(t, admin); count != 2 {
		t.Errorf("past the answer's keeping, the key made %d backups in all; want a second", count)
	}
}

// runBackups runs the backup jobs that are due, as the server's runner would.
func (harness testHarness) runBackups(t *testing.T) {
	t.Helper()
	if err := harness.queue.NewRunner(jobs.KindBackup, harness.backups.HandleBackup).RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// backupCount is how many backups GET /backups lists.
func (harness testHarness) backupCount(t *testing.T, admin string) int {
	t.Helper()
	var backups BackupsOutput
	listed := harness.do(t, http.MethodGet, "/backups", admin, nil)
	if err := json.Unmarshal([]byte(listed.body), &backups.Body); err != nil {
		t.Fatalf("GET /backups = %d %s", listed.status, listed.body)
	}
	return len(backups.Body.Backups)
}
