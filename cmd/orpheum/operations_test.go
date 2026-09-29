// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// startServerOnce runs the server until it is ready and stops it, leaving a state directory with a
// migrated database, a secret.key and a setup code, as a first start does.
func startServerOnce(t *testing.T) (Config, string) {
	t.Helper()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	return config, path
}

func TestSetupCode_PrintsTheCodeUntilSetupIsDone(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	if got := runCommand(t, "setup-code", "--config", path); got.status == exitSuccess {
		t.Errorf("before the first start: %+v, want a failure saying to start the server", got)
	}
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	code := runCommand(t, "setup-code", "--config", path)
	if code.status != exitSuccess || len(strings.TrimSpace(code.stdout)) != len("AAAA-BBBB-CCCC") {
		t.Fatalf("setup-code = %+v", code)
	}
	setUpOwner(t, server, path, "owner")
	after := runCommand(t, "setup-code", "--config", path)
	if after.status == exitSuccess || !strings.Contains(after.stdout, "setup is complete") {
		t.Errorf("after setup: %+v, want it to say setup is complete, and not succeed", after)
	}
}

func TestBackupAndRestore_FromTheCommandLine(t *testing.T) {
	t.Parallel()
	_, sourcePath := startServerOnce(t)
	written := runCommand(t, "backup", "--config", sourcePath, "--include-key")
	if written.status != exitSuccess || !strings.Contains(written.stdout, "secret.key") {
		t.Fatalf("backup --include-key = %+v", written)
	}
	fields := strings.Fields(written.stdout)
	backupFile, keyFile := fields[1], filepath.Join(filepath.Dir(fields[1]),
		strings.TrimSuffix(filepath.Base(fields[1]), ".sqlite")+".secret.key")

	target, targetPath := testConfig(t)
	dryRun := runCommand(t, "restore", "--config", targetPath, "--from", backupFile, "--key", keyFile)
	if dryRun.status != exitUsageError || !strings.Contains(dryRun.stdout, "Nothing was changed") {
		t.Errorf("restore without --yes = %+v, want exit 2 and nothing changed", dryRun)
	}
	if _, err := os.Stat(filepath.Join(target.Server.DataDir, "library.db")); err == nil {
		t.Error("restore without --yes wrote the database")
	}
	restored := runCommand(t, "restore", "--config", targetPath, "--from", backupFile, "--key", keyFile, "--yes")
	if restored.status != exitSuccess {
		t.Fatalf("restore --yes = %+v", restored)
	}
	doctor := runCommand(t, "doctor", "--config", targetPath)
	if doctor.status != exitSuccess || !strings.Contains(doctor.stdout, "every sealed secret opens") {
		t.Errorf("doctor after a restore with the key = %+v", doctor)
	}
	missing := runCommand(t, "restore", "--config", targetPath, "--from", backupFile+".typo", "--yes")
	if missing.status == exitSuccess {
		t.Errorf("restore from a missing file = %+v, want a failure", missing)
	}
}

func TestMigrate_FromTheCommandLine(t *testing.T) {
	t.Parallel()
	_, path := testConfig(t)
	first := runCommand(t, "migrate", "--config", path)
	if first.status != exitSuccess || !strings.Contains(first.stdout, "migrating: step 1 of") {
		t.Errorf("the first migrate = %+v, want it to report each step", first)
	}
	again := runCommand(t, "migrate", "--config", path)
	if again.status != exitSuccess || strings.Contains(again.stdout, "migrating:") {
		t.Errorf("migrating a current database = %+v, want no steps", again)
	}
}

func TestDoctor_SaysWhatIsMissingAndHowToFixIt(t *testing.T) {
	t.Parallel()
	_, path := testConfig(t)
	fresh := runCommand(t, "doctor", "--config", path)
	for _, want := range []string{"there is no library.db yet", "secret.key is missing", "there is no backup yet"} {
		if !strings.Contains(fresh.stdout, want) {
			t.Errorf("doctor on a fresh directory does not say %q:\n%s", want, fresh.stdout)
		}
	}
	config, path := startServerOnce(t)
	if err := os.WriteFile(filepath.Join(config.Server.DataDir, "secret.key"), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	damaged := runCommand(t, "doctor", "--config", path)
	if damaged.status != exitFailure || !strings.Contains(damaged.stdout, "FAIL  secret.key is damaged") {
		t.Errorf("doctor with a damaged key = %+v, want a failure naming it", damaged)
	}
}

func TestDoctor_FailsADatabaseBelowTheBaselineRatherThanPromisingAMigration(t *testing.T) {
	t.Parallel()
	config, path := startServerOnce(t)
	steps, err := store.Steps()
	if err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, filepath.Join(config.Server.DataDir, "library.db"),
		"PRAGMA user_version = "+strconv.Itoa(steps[0].Version-1))
	doctor := runCommand(t, "doctor", "--config", path)
	if doctor.status != exitFailure || !strings.Contains(doctor.stdout, "below the baseline") {
		t.Errorf("doctor on a database the release would refuse = %+v, want a failure saying why", doctor)
	}
}

func TestOpenAPI_PrintsTheDocument(t *testing.T) {
	t.Parallel()
	document := runCommand(t, "openapi")
	if document.status != exitSuccess || !strings.Contains(document.stdout, `"url": "/api/v1"`) ||
		!strings.Contains(document.stdout, `"/health"`) {
		t.Errorf("openapi = status %d, %.200s", document.status, document.stdout)
	}
}

func TestDoctor_ListsWhatShouldBeFixedAndChangesNothingAPersonChose(t *testing.T) {
	t.Parallel()
	config, path := startServerOnce(t)
	database := filepath.Join(config.Server.DataDir, "library.db")
	storetest.ExecRaw(t, database,
		"INSERT INTO setting (scope, key, value_json, set_at) VALUES ('server', 'backup_age_warn_h', '24', 1)")
	text := strings.Replace(string(readFile(t, path)), "shutdown_grace_s = 10\n", "shutdwn_grace_s = 5\n", 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	doctor := runCommand(t, "doctor", "--config", path)
	for _, want := range []string{
		"config: server.shutdown_grace_s is not in the file; the server runs on its starting value, 10",
		"config: server.shutdwn_grace_s is in the file, but nothing reads it",
		"setting backup_age_warn_h is unused; no release has used this key: probably a typo",
		"DELETE /api/v1/config/unused/backup_age_warn_h",
	} {
		if !strings.Contains(doctor.stdout, want) {
			t.Errorf("doctor does not say %q:\n%s", want, doctor.stdout)
		}
	}
	if got := string(readFile(t, path)); got != text {
		t.Errorf("doctor changed the config file:\n%s", got)
	}
}

func TestLoadConfig_RefusesOnlyWhatHasNoStartingValue(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	text := strings.Replace(string(readFile(t, path)), "data_dir = ", "# data_dir = ", 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := runCommand(t, "doctor", "--config", path)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr,
		"server.data_dir is required and has no starting value") {
		t.Errorf("with data_dir removed, doctor = %+v; want a refusal: it has no starting value", refused)
	}
	if config.Server.DataDir == "" {
		t.Fatal("the test config has no data directory")
	}
}

func TestConfinementFindings_SayEachMechanismAndWhatWouldPutAMissingOneInForce(t *testing.T) {
	t.Parallel()
	got := confinementFindings([]confine.State{
		{Mechanism: confine.MechanismLandlock, Reason: "not enabled at boot", Remedy: "add landlock to lsm="},
		{Mechanism: confine.MechanismSeccomp, IsInForce: true},
	})
	if len(got) != 2 || got[0].verdict != verdictWarning ||
		!strings.Contains(got[0].description, "not enabled at boot") ||
		got[0].remedy != "add landlock to lsm=" || got[1].verdict != verdictOK {
		t.Errorf("findings = %+v; want Landlock warned with its remedy, seccomp ok (D112)", got)
	}
}
