// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
