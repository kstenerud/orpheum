// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// startedOnce runs the server until it is ready and stops it, leaving a state directory with a
// migrated database, a secret.key and a setup code, as a first start does.
func startedOnce(t *testing.T) (Config, string) {
	t.Helper()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready")
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
	server := startServing(t, config, newCapturedLog("ready"), "ready")
	code := runCommand(t, "setup-code", "--config", path)
	if code.status != exitSuccess || len(strings.TrimSpace(code.stdout)) != len("AAAA-BBBB-CCCC") {
		t.Fatalf("setup-code = %+v", code)
	}
	created := call(t, http.MethodPost, server.baseURL+"/setup", "",
		map[string]string{"setupCode": strings.TrimSpace(code.stdout), "name": "owner", "password": "pw"})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	after := runCommand(t, "setup-code", "--config", path)
	if after.status == exitSuccess || !strings.Contains(after.stdout, "setup is complete") {
		t.Errorf("after setup: %+v, want it to say setup is complete, and not succeed", after)
	}
}

func TestBackupAndRestore_FromTheCommandLine(t *testing.T) {
	t.Parallel()
	_, sourcePath := startedOnce(t)
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
	config, path := startedOnce(t)
	if err := os.WriteFile(filepath.Join(config.Server.DataDir, "secret.key"), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	damaged := runCommand(t, "doctor", "--config", path)
	if damaged.status != exitFailure || !strings.Contains(damaged.stdout, "FAIL  secret.key is damaged") {
		t.Errorf("doctor with a damaged key = %+v, want a failure naming it", damaged)
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

func TestSeedSettings_RefillsARemovedSettingAndDoctorSaysWhatIsMissing(t *testing.T) {
	t.Parallel()
	config, path := startedOnce(t)
	storetest.ExecRaw(t, filepath.Join(config.Server.DataDir, "library.db"),
		"DELETE FROM setting WHERE key = 'backup_age_warning_h'")
	doctor := runCommand(t, "doctor", "--config", path)
	if !strings.Contains(doctor.stdout, "backup_age_warning_h has no value; its starting value is 168") ||
		!strings.Contains(doctor.stdout, "orpheum seed-settings") {
		t.Errorf("doctor with a setting removed does not name it, its starting value and the fix:\n%s", doctor.stdout)
	}
	storetest.ExecRaw(t, filepath.Join(config.Server.DataDir, "library.db"),
		"INSERT INTO setting (scope, key, value, set_at) VALUES ('server', 'backup_age_warn_h', '24', 1)")
	seeded := runCommand(t, "seed-settings", "--config", path)
	for _, want := range []string{
		"config: every key is there; nothing was written",
		"settings: wrote backup_age_warning_h = 168, its starting value",
		"settings: backup_age_warn_h is in the database, but nothing reads it",
	} {
		if seeded.status != exitSuccess || !strings.Contains(seeded.stdout, want) {
			t.Errorf("seed-settings = %+v, want it to say %q", seeded, want)
		}
	}
	again := runCommand(t, "seed-settings", "--config", path)
	if again.status != exitSuccess || !strings.Contains(again.stdout, "settings: every setting already has a value") ||
		!strings.Contains(again.stdout, "backup_age_warn_h is in the database") {
		t.Errorf("seed-settings again = %+v, want nothing written and the unknown row still reported", again)
	}
	doctorAfter := runCommand(t, "doctor", "--config", path)
	if !strings.Contains(doctorAfter.stdout, "setting backup_age_warn_h is in the database, but nothing reads it") {
		t.Errorf("doctor does not report the unknown setting row:\n%s", doctorAfter.stdout)
	}
}

func TestSeedSettings_FillsTheConfigFileWhenTheServerCannotStart(t *testing.T) {
	t.Parallel()
	config, path := startedOnce(t)
	text := strings.Replace(string(readFile(t, path)), "shutdown_grace_s = 10\n", "", 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := runCommand(t, "doctor", "--config", path)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr,
		"server.shutdown_grace_s is required; its starting value is 10") {
		t.Errorf("with a key removed, doctor = %+v; want a refusal naming the key and its starting value", refused)
	}
	seeded := runCommand(t, "seed-settings", "--config", path)
	if seeded.status != exitSuccess || !strings.Contains(seeded.stdout, "config: wrote server.shutdown_grace_s = 10") {
		t.Errorf("seed-settings = %+v, want the missing key written", seeded)
	}
	if _, _, err := loadConfig(path); err != nil {
		t.Errorf("after seeding, the config does not load: %v", err)
	}
	if config.Server.DataDir == "" {
		t.Fatal("the test config has no data directory")
	}
}
