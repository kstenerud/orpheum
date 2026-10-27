// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// startServerOnce runs the server until it is ready and stops it, leaving an instance with a
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
	if got := runCommand(t, "setup-code", "--instance", path); got.status == exitSuccess {
		t.Errorf("before the first start: %+v, want a failure saying to start the server", got)
	}
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	code := runCommand(t, "setup-code", "--instance", path)
	if code.status != exitSuccess || len(strings.TrimSpace(code.stdout)) != len("AAAA-BBBB-CCCC") {
		t.Fatalf("setup-code = %+v", code)
	}
	// It is the running server's, so it is kept in the run directory, which no backup holds (D242 §3).
	if kept := readFile(t, filepath.Join(config.Instance.RunDir, "setup-code")); string(kept) !=
		strings.TrimSpace(code.stdout) {
		t.Errorf("the run directory keeps %q as the setup code; want %q", kept, code.stdout)
	}
	setUpOwner(t, server, path, "owner")
	after := runCommand(t, "setup-code", "--instance", path)
	if after.status == exitSuccess || !strings.Contains(after.stdout, "setup is complete") {
		t.Errorf("after setup: %+v, want it to say setup is complete, and not succeed", after)
	}
}

func TestBackupAndRestore_FromTheCommandLine(t *testing.T) {
	t.Parallel()
	source, sourcePath := startServerOnce(t)
	written := runCommand(t, "backup", "--instance", sourcePath, "--include-key")
	if written.status != exitSuccess || !strings.Contains(written.stdout, "it holds secret.key") {
		t.Fatalf("backup --include-key = %+v", written)
	}
	backupFile := strings.Fields(written.stdout)[1]
	if filepath.Dir(backupFile) != source.Instance.BackupsDir || !strings.HasSuffix(backupFile, ".tar.gz") {
		t.Errorf("the backup is %s; want a tarball in the backups directory (D242 §5)", backupFile)
	}

	target, targetPath := testConfig(t)
	dryRun := runCommand(t, "restore", "--instance", targetPath, "--from", backupFile)
	if dryRun.status != exitUsageError || !strings.Contains(dryRun.stdout, "Nothing was changed") ||
		!strings.Contains(dryRun.stdout, "holds secret.key") {
		t.Errorf("restore without --yes = %+v, want exit 2, the key named and nothing changed", dryRun)
	}
	if _, err := os.Stat(filepath.Join(target.Instance.PersistentDir, "persistent.db")); err == nil {
		t.Error("restore without --yes wrote the database")
	}
	restored := runCommand(t, "restore", "--instance", targetPath, "--from", backupFile, "--yes")
	if restored.status != exitSuccess {
		t.Fatalf("restore --yes = %+v", restored)
	}
	doctor := runCommand(t, "doctor", "--instance", targetPath)
	if doctor.status != exitSuccess || !strings.Contains(doctor.stdout, "every sealed secret opens") {
		t.Errorf("doctor after a restore with the key = %+v", doctor)
	}
	missing := runCommand(t, "restore", "--instance", targetPath, "--from", backupFile+".typo", "--yes")
	if missing.status == exitSuccess {
		t.Errorf("restore from a missing file = %+v, want a failure", missing)
	}
}

func TestMigrate_FromTheCommandLine(t *testing.T) {
	t.Parallel()
	_, path := testConfig(t)
	first := runCommand(t, "migrate", "--instance", path)
	if first.status != exitSuccess || !strings.Contains(first.stdout, "migrating: step 1 of") {
		t.Errorf("the first migrate = %+v, want it to report each step", first)
	}
	again := runCommand(t, "migrate", "--instance", path)
	if again.status != exitSuccess || strings.Contains(again.stdout, "migrating:") {
		t.Errorf("migrating a current database = %+v, want no steps", again)
	}
}

func TestDoctor_SaysWhatIsMissingAndHowToFixIt(t *testing.T) {
	t.Parallel()
	_, path := testConfig(t)
	fresh := runCommand(t, "doctor", "--instance", path)
	for _, want := range []string{
		"there is no persistent.db yet", "start the server with `orpheum core`", "secret.key is missing",
		"there is no backup yet", "persistent directory " + filepath.Join(path, "persistent") + " is writable",
	} {
		if !strings.Contains(fresh.stdout, want) {
			t.Errorf("doctor on a fresh directory does not say %q:\n%s", want, fresh.stdout)
		}
	}
	config, path := startServerOnce(t)
	keyPath := filepath.Join(config.Instance.PersistentDir, "secret.key")
	if err := os.WriteFile(keyPath, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	damaged := runCommand(t, "doctor", "--instance", path)
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
	storetest.ExecRaw(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		"PRAGMA user_version = "+strconv.Itoa(steps[0].Version-1))
	doctor := runCommand(t, "doctor", "--instance", path)
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
	database := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	storetest.ExecRaw(t, database,
		"INSERT INTO setting (key, value_json, set_at) VALUES ('backup_age_warn_h', '24', 1)")
	fullConfig := config.Instance.fullConfigPath()
	text := strings.Replace(string(readFile(t, fullConfig)),
		"shutdown_grace_seconds = 10\n", "shutdwn_grace_seconds = 5\n", 1)
	if err := os.WriteFile(fullConfig, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	doctor := runCommand(t, "doctor", "--instance", path)
	for _, want := range []string{
		"config: server.shutdown_grace_seconds is not in the file; the server runs on its starting value, 10",
		"config: server.shutdwn_grace_seconds is in the file, but nothing reads it",
		"setting backup_age_warn_h is unused; no release has used this key: probably a typo",
		"DELETE /api/v1/config/unused/backup_age_warn_h",
		`delete it on the System page, under "Settings this release does not read"`,
	} {
		if !strings.Contains(doctor.stdout, want) {
			t.Errorf("doctor does not say %q:\n%s", want, doctor.stdout)
		}
	}
	if got := string(readFile(t, fullConfig)); got != text {
		t.Errorf("doctor changed the config file:\n%s", got)
	}
}

func TestLoadConfig_RefusesOnlyWhatHasNoStartingValue(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	fullConfig := config.Instance.fullConfigPath()
	text := strings.Replace(string(readFile(t, fullConfig)), "listen = ", "# listen = ", 1)
	if err := os.WriteFile(fullConfig, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := runCommand(t, "doctor", "--instance", path)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr,
		"server.listen is required and has no starting value") {
		t.Errorf("with listen removed, doctor = %+v; want a refusal: it has no starting value", refused)
	}
}

func TestConfinementFindings_SayEachMechanismAndWhatWouldPutAMissingOneInForce(t *testing.T) {
	t.Parallel()
	// The one judgement the server's /doctor makes of each node's report (api.ConfinementFinding).
	got := confinementOfThisMachine([]confine.State{
		{Mechanism: confine.MechanismLandlock, Reason: "not enabled at boot", Remedy: "add landlock to lsm="},
		{Mechanism: confine.MechanismSeccomp, IsInForce: true},
	})
	if got.verdict != api.VerdictWarning ||
		!strings.Contains(got.description, confine.MechanismLandlock+": not enabled at boot") ||
		strings.Contains(got.description, confine.MechanismSeccomp) ||
		got.remedy != confine.MechanismLandlock+": add landlock to lsm=" {
		t.Errorf("finding = %+v; want Landlock warned with its remedy, seccomp not named (D112)", got)
	}
	confined := confinementOfThisMachine([]confine.State{
		{Mechanism: confine.MechanismLandlock, IsInForce: true}, {Mechanism: confine.MechanismSeccomp, IsInForce: true},
	})
	if confined.verdict != api.VerdictOK || !strings.Contains(confined.description, "this machine") {
		t.Errorf("both in force: %+v, want ok for this machine", confined)
	}
	// A machine that reports no mechanism at all is not confined; the command once said nothing.
	if none := confinementOfThisMachine(nil); none.verdict != api.VerdictWarning {
		t.Errorf("no mechanism reported: %+v, want a warning", none)
	}
}

// An operation on a server that never started says how to start it, with a subcommand that exists:
// `orpheum core` since the node became a service of its own (D235 §4).
func TestOpenExistingDatabase_SaysToStartTheCoreWithASubcommandThatExists(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	_, _, err := openExistingDatabase(t.Context(), config)
	if err == nil || !strings.Contains(err.Error(), "`orpheum core`") {
		t.Fatalf("an operation with no database: %v, want it to say to start `orpheum core`", err)
	}
	if !slices.ContainsFunc(plannedSubcommands(), func(planned subcommand) bool {
		return planned.name == "core" && planned.implementation != nil
	}) {
		t.Error("the message names `orpheum core`, which is not a subcommand that runs")
	}
}

// start-over sets the database aside, with the server stopped, and keeps every other persistent
// file, so a next start makes an empty database under the same identity (D303 §2); without --yes
// it changes nothing, and while the server runs it refuses.
func TestStartOver_SetsTheDatabaseAsideAndKeepsTheRest(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	if running := runCommand(t, "start-over", "--instance", path, "--yes"); running.status == exitSuccess {
		t.Errorf("start-over while the server runs = %+v; want a refusal", running)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	untilTheInstanceIsFree(t, config)
	persistentDB := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	dryRun := runCommand(t, "start-over", "--instance", path)
	if _, err := os.Stat(persistentDB); err != nil || dryRun.status != exitUsageError ||
		!strings.Contains(dryRun.stdout, "Nothing was changed") {
		t.Fatalf("start-over without --yes = %+v (%v); want exit 2 and the database where it was", dryRun, err)
	}
	keptBefore := persistentFilesBut(t, config.Instance.PersistentDir, "persistent.db")
	done := runCommand(t, "start-over", "--instance", path, "--yes")
	if done.status != exitSuccess {
		t.Fatalf("start-over --yes = %+v", done)
	}
	if _, err := os.Stat(persistentDB); !os.IsNotExist(err) {
		t.Errorf("the database is still in the persistent directory (%v)", err)
	}
	aside, err := filepath.Glob(filepath.Join(config.Instance.BackupsDir, "persistent-set-aside-*.db"))
	if err != nil || len(aside) != 1 {
		t.Errorf("set aside in the backups directory: %v (%v); want the one database", aside, err)
	}
	if kept := persistentFilesBut(t, config.Instance.PersistentDir, "persistent.db"); !slices.Equal(kept,
		keptBefore) || len(kept) == 0 {
		t.Errorf("the persistent directory holds %v after; want %v, all but the database", kept, keptBefore)
	}
}

// persistentFilesBut lists the persistent directory's files, but the database and its sidecars.
func persistentFilesBut(t *testing.T, directory, database string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), database) {
			names = append(names, entry.Name())
		}
	}
	return names
}

// The served server keeps its filing reports, which its admin lists (D306 §3).
func TestServe_TheAdminListsTheFilingReports(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, path)
	listed := requestAPI(t, http.MethodGet, server.baseURL+"/filing-reports", token, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"reports":[]`) {
		t.Errorf("GET /filing-reports = %d %s; want none yet", listed.status, listed.body)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}

// untilTheInstanceIsFree waits until the stopped server's lock on the instance can be taken, and lets
// it go. The server ran in this test process, whose other tests start subprocesses: until each child
// execs, it holds a copy of every descriptor the process has open, the lock's among them, and under
// load start-over found the instance still in use after stop returned (2026-10-27).
func untilTheInstanceIsFree(t *testing.T, config Config) {
	t.Helper()
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		t.Fatal(err)
	}
	for tries := 0; ; tries++ {
		lock, lockErr := directories.LockInstance()
		if lockErr == nil {
			if releaseErr := lock.Release(); releaseErr != nil {
				t.Fatal(releaseErr)
			}
			return
		}
		if !errors.Is(lockErr, instancedir.ErrInUse) || tries >= int(testutil.ReceiveLimit/(10*time.Millisecond)) {
			t.Fatalf("the stopped server's instance is still locked: %v", lockErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
