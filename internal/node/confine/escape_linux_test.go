// SPDX-License-Identifier: GPL-3.0-or-later

package confine_test

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The escape test (D112, D190 §3; cut-a §7): this test binary stands in for FFmpeg, and for the
// shim that confines it, so each attempt runs as a tool would, in its own process. Every attempt
// is first shown to succeed without our confinement, so a refusal is ours and not the machine's.

// The words after -- that make this binary one of its own helpers.
const (
	actAsShim = "act-as-shim"
	attempt   = "attempt"
)

// helperArguments are what makes the binary run only the helper test, with words after --.
func helperArguments(words ...string) []string {
	return append([]string{"-test.run=^TestEscapeHelper$", "--"}, words...)
}

// TestEscapeHelper is the helper: as the shim it confines itself and becomes the attempt; as the
// attempt it tries one thing and prints how it went. Run as a plain test it does nothing.
func TestEscapeHelper(_ *testing.T) {
	words := flag.Args()
	if len(words) < 2 {
		return
	}
	switch words[0] {
	case actAsShim:
		err := confine.Shim(words[1])
		_, _ = fmt.Printf("the shim could not run the tool: %v\n", err) //nolint:forbidigo // the helper's answer
	case attempt:
		_, _ = fmt.Println(try(words[1], words[2:])) //nolint:forbidigo // the helper's answer
	}
}

// try is one attempt, answered "allowed" or "refused: why".
func try(what string, arguments []string) string {
	var err error
	switch what {
	case "read":
		_, err = os.ReadFile(arguments[0])
	case "write":
		err = os.WriteFile(arguments[0], []byte("escaped"), 0o600)
	case "inet-socket":
		var socket int
		socket, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		if err == nil {
			_ = syscall.Close(socket) //nolint:errcheck // closing a probe
		}
	}
	if err != nil {
		return "refused: " + err.Error()
	}
	return "allowed"
}

// run runs this binary with arguments and returns what it printed.
func run(t *testing.T, arguments []string) string {
	t.Helper()
	output, err := testutil.RunSelf(t, arguments...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(lastLine(string(output)))
}

// lastLine is the helper's answer, after anything the test framework printed.
func lastLine(output string) string {
	for _, line := range slices.Backward(strings.Split(strings.TrimSpace(output), "\n")) {
		isAnswer := strings.HasPrefix(line, "allowed") || strings.HasPrefix(line, "refused")
		if isAnswer || strings.HasPrefix(line, "the shim") {
			return line
		}
	}
	return output
}

// confined runs one attempt as a confined tool given access.
func confined(t *testing.T, access confine.Access, what string, arguments ...string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec, err := confine.EncodeSpec(confine.Spec{
		Tool: self, Arguments: helperArguments(append([]string{attempt, what}, arguments...)...), Access: access,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run(t, helperArguments(actAsShim, spec))
}

// unconfined runs one attempt as a plain process.
func unconfined(t *testing.T, what string, arguments ...string) string {
	t.Helper()
	return run(t, helperArguments(append([]string{attempt, what}, arguments...)...))
}

// requireMechanisms fails the test where a mechanism is absent, unless the job lists it (D190 §2).
func requireMechanisms(t *testing.T) {
	t.Helper()
	for _, state := range confine.Probe() {
		var absent error
		if !state.InForce {
			absent = errors.New(state.Reason + "; " + state.Remedy)
		}
		testutil.Require(t, state.Mechanism, absent)
	}
}

func TestEscape_AConfinedToolReadsAndWritesOnlyWhatItWasGivenAndOpensNoNetwork(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	given, elsewhere := t.TempDir(), t.TempDir()
	source := filepath.Join(given, "film.mkv")
	secret := filepath.Join(elsewhere, "secret.txt")
	for _, path := range []string{source, secret} {
		if err := os.WriteFile(path, []byte("bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output := t.TempDir()
	access := confine.Access{Read: []string{source}, Write: []string{output}}
	for _, check := range []struct {
		what      string
		arguments []string
	}{
		{"read", []string{secret}},
		{"write", []string{filepath.Join(elsewhere, "planted.txt")}},
		// Refused by our seccomp filter alone: Landlock limits TCP's connect and bind, not the socket.
		{"inet-socket", nil},
	} {
		if got := unconfined(t, check.what, check.arguments...); got != "allowed" {
			t.Fatalf("unconfined, %s %v: %s; want allowed, so the refusal below is ours (D190 §3)",
				check.what, check.arguments, got)
		}
		if got := confined(t, access, check.what, check.arguments...); !strings.HasPrefix(got, "refused") {
			t.Errorf("confined, %s %v: %s; want refused", check.what, check.arguments, got)
		}
	}
	if got := confined(t, access, "read", source); got != "allowed" {
		t.Errorf("confined, reading the source it was given: %s; want allowed", got)
	}
	if got := confined(t, access, "write", filepath.Join(output, "segment.m4s")); got != "allowed" {
		t.Errorf("confined, writing in its output directory: %s; want allowed", got)
	}
}

func TestEscape_ADynamicallyLinkedToolStartsConfinedAndReadsOnlyWhatItWasGiven(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	// FFmpeg is linked dynamically, so starting it executes its loader too; cat is linked so on
	// every distribution this test runs on, and the helper above, a static Go binary, is not.
	cat, err := lookPath("cat")
	if err != nil {
		t.Fatal(err)
	}
	given, elsewhere := filepath.Join(t.TempDir(), "given.txt"), filepath.Join(t.TempDir(), "secret.txt")
	for _, path := range []string{given, elsewhere} {
		if err := os.WriteFile(path, []byte("bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	access := confine.Access{Read: []string{given}}
	if output, err := catConfined(t, cat, given, access); err != nil || output != "bytes" {
		t.Errorf("cat, confined, of the file it was given: %q, %v; want its bytes", output, err)
	}
	if output, err := catConfined(t, cat, elsewhere, access); err == nil {
		t.Errorf("cat, confined, of a file it was not given: %q; want refused", output)
	}
}

// catConfined runs cat on path as a confined tool given access, and returns what it printed.
func catConfined(t *testing.T, cat, path string, access confine.Access) (string, error) {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(cat)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := confine.EncodeSpec(confine.Spec{Tool: resolved, Arguments: []string{path}, Access: access})
	if err != nil {
		t.Fatal(err)
	}
	output, err := testutil.RunSelf(t, helperArguments(actAsShim, spec)...)
	return string(output), err
}

// lookPath finds a tool on the usual directories, without the environment's PATH, which the shim
// empties.
func lookPath(tool string) (string, error) {
	for _, directory := range []string{"/usr/bin", "/bin"} {
		path := filepath.Join(directory, tool)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s in /usr/bin or /bin", tool)
}
