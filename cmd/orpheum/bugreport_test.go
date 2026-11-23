// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// bugreportWords are a person's words naming a person and a library the database holds.
const bugreportWords = "danielle says grandmas shelf will not scan"

// bugreportInstance is a core instance whose database holds a person and a library bugreportWords
// name; with logFile it writes its log, in JSON, to the file it names.
func bugreportInstance(t *testing.T, logFile string) (Config, string) {
	t.Helper()
	config, configPath := testConfig(t)
	if logFile != "" {
		text, err := os.ReadFile(config.Instance.fullConfigPath())
		if err != nil {
			t.Fatal(err)
		}
		written := strings.Replace(string(text), `destination = "stderr"`, `destination = "`+logFile+`"`, 1)
		//nolint:gosec // G703: the test's own file
		if err = os.WriteFile(config.Instance.fullConfigPath(), []byte(written), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seedPersistentDatabase(t, config)
	storetest.ExecRaw(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		`INSERT INTO user (name, role_id, created_at) VALUES ('danielle', 1, 1)`,
		`INSERT INTO node (name, enrolled_at) VALUES ('garagebox', 1)`,
		`INSERT INTO library (name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
		 VALUES ('grandmas shelf', 'movie', 'tmdb', 'manual', 6, 0, 1)`)
	return config, configPath
}

// The command line's safe report hides every name the database holds, and the instance's paths;
// its unsafe report names them, with the table of what each tag stands for. Either says the
// command that made it, what the database says, and why the core and the nodes gave no log
// (D524 §4, §5).
func TestBugreport_SafeHidesTheNamesTheDatabaseHoldsAndUnsafeNamesThemWithTheirTags(t *testing.T) {
	t.Parallel()
	config, configPath := bugreportInstance(t, "")

	safe := runCommand(t, "bugreport", "safe", "--instance", configPath, "--words", bugreportWords)
	if safe.status != exitSuccess {
		t.Fatalf("bugreport safe = %d:\n%s", safe.status, safe.stderr)
	}
	for _, name := range []string{"danielle", "grandmas shelf", "garagebox", config.Instance.PersistentDir} {
		if strings.Contains(safe.stdout, name) {
			t.Errorf("the safe report names %q:\n%s", name, safe.stdout)
		}
	}
	for _, said := range []string{
		"Made on the command line, by `orpheum bugreport safe`", "| Database | opens, at schema step",
		"| Doctor: ok | config: every key is in the file", "### core: absent", "standard error",
		"a report made on the command line asks no node", "Review before sharing", "is writable",
	} {
		if !strings.Contains(safe.stdout, said) {
			t.Errorf("the safe report does not say %q:\n%s", said, safe.stdout)
		}
	}

	unsafe := runCommand(t, "bugreport", "unsafe", "--instance", configPath, "--words", bugreportWords)
	if unsafe.status != exitSuccess {
		t.Fatalf("bugreport unsafe = %d:\n%s", unsafe.status, unsafe.stderr)
	}
	for _, said := range []string{bugreportWords, config.Instance.PersistentDir, "DO NOT SHARE PUBLICLY"} {
		if !strings.Contains(unsafe.stdout, said) {
			t.Errorf("the unsafe report does not say %q:\n%s", said, unsafe.stdout)
		}
	}
	for _, row := range []string{
		`\| user:[0-9a-f]+ \| danielle \|`,
		`\| text:[0-9a-f]+ \| grandmas shelf \|`,
		`\| node:[0-9a-f]+ \| garagebox \|`,
	} {
		if !regexp.MustCompile(row).MatchString(unsafe.stdout) {
			t.Errorf("the unsafe report's table has no row %s:\n%s", row, unsafe.stdout)
		}
	}
}

// A report has no default form: without its word, or with another, the command says how it is used
// and makes none (D74, D524 §4).
func TestBugreport_RefusesAMissingOrWrongFormWord(t *testing.T) {
	t.Parallel()
	_, configPath := testConfig(t)
	for _, arguments := range [][]string{
		{"bugreport"},
		{"bugreport", "share", "--instance", configPath},
		{"bugreport", "--instance", configPath},
		{"bugreport", "safe"},
	} {
		refused := runCommand(t, arguments...)
		if refused.status != exitUsageError || refused.stdout != "" ||
			!strings.Contains(refused.stderr, "Usage: orpheum bugreport safe|unsafe") {
			t.Errorf("%v = %d, stdout %q, stderr %q; want the usage error, its usage and no report", arguments,
				refused.status, refused.stdout, refused.stderr)
		}
	}
}

// A database that does not open still gives a report, which says why, and that it could not look
// for the database's names (D524 §5).
func TestBugreport_ADatabaseThatDoesNotOpenStillGivesAReportSayingWhy(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	if err := os.WriteFile(filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		[]byte(strings.Repeat("not a database ", 512)), 0o600); err != nil {
		t.Fatal(err)
	}
	report := runCommand(t, "bugreport", "unsafe", "--instance", configPath)
	if report.status != exitSuccess {
		t.Fatalf("bugreport unsafe = %d:\n%s", report.status, report.stderr)
	}
	for _, said := range []string{
		"| Database | does not open: ", "| Names hidden | only those this report finds itself",
		"### every node: absent", "the database did not open",
	} {
		if !strings.Contains(report.stdout, said) {
			t.Errorf("the report does not say %q:\n%s", said, report.stdout)
		}
	}
}

// A core that writes its log to a file gives the report its recent log from it, a day's lines at
// info before the newest, a name in them hidden in the safe report (D524 §3, §5).
func TestBugreport_HoldsTheCoresRecentLogFromItsLogFile(t *testing.T) {
	t.Parallel()
	logFile := filepath.Join(t.TempDir(), "core.log")
	_, configPath := bugreportInstance(t, logFile)
	file, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logger, err := slogx.New(file, slogx.FormatJSON, slog.LevelDebug)
	if err != nil {
		t.Fatal(err)
	}
	newest := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	for _, each := range []struct {
		at      time.Time
		message string
	}{
		{newest.Add(-25 * time.Hour), "a line from the day before"},
		{newest, "the library could not be scanned"},
	} {
		record := slog.NewRecord(each.at, slog.LevelWarn, each.message, 0)
		record.AddAttrs(slog.String("err", "grandmas shelf: permission denied"))
		if err = logger.Handler().Handle(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	safe := runCommand(t, "bugreport", "safe", "--instance", configPath)
	if safe.status != exitSuccess {
		t.Fatalf("bugreport safe = %d:\n%s", safe.status, safe.stderr)
	}
	if !strings.Contains(safe.stdout, "All 1 of its recent lines.") ||
		!strings.Contains(safe.stdout, "the library could not be scanned") ||
		strings.Contains(safe.stdout, "the day before") || strings.Contains(safe.stdout, "grandmas shelf") {
		t.Errorf("the safe report does not hold the core's newest line alone, its library's name hidden:\n%s",
			safe.stdout)
	}
	if unsafe := runCommand(t, "bugreport", "unsafe", "--instance", configPath); !strings.Contains(unsafe.stdout,
		"err=\"grandmas shelf: permission denied\"") {
		t.Errorf("the unsafe report does not hold the core's line as logged:\n%s", unsafe.stdout)
	}
}
