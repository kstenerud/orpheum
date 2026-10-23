// SPDX-License-Identifier: AGPL-3.0-or-later

package confine_test

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

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
		_, _ = fmt.Println(attemptEscape(words[1], words[2:])) //nolint:forbidigo // the helper's answer
	}
}

// attemptEscape is one attempt, answered "allowed" or "refused: why".
func attemptEscape(what string, arguments []string) string {
	var err error
	switch what {
	case "read":
		_, err = os.ReadFile(arguments[0])
	case "write":
		err = os.WriteFile(arguments[0], []byte("escaped"), 0o600)
	case "oom-score":
		var score []byte
		if score, err = os.ReadFile("/proc/self/oom_score_adj"); err == nil {
			return "allowed " + strings.TrimSpace(string(score))
		}
	case "name-thread":
		// As CUDA names its helper thread: through its own task's comm file.
		runtime.LockOSThread()
		err = os.WriteFile(fmt.Sprintf("/proc/self/task/%d/comm", syscall.Gettid()), []byte("probe"), 0o600)
	case "inet-socket", "unix-socket", "unix-connect", "unix-pair":
		err = attemptSocket(what, arguments)
	case "start-process", "session", "process-group", "clone3", "threads":
		err = attemptProcess(what)
	}
	if err != nil {
		return "refused: " + err.Error()
	}
	return "allowed"
}

// attemptSocket tries to create a network socket, a Unix one, a connected Unix one, or a pair.
func attemptSocket(what string, arguments []string) error {
	var err error
	switch what {
	case "inet-socket":
		var socket int
		socket, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		if err == nil {
			_ = syscall.Close(socket) //nolint:errcheck // closing a probe
		}
	case "unix-socket":
		var socket int
		socket, err = syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
		if err == nil {
			_ = syscall.Close(socket) //nolint:errcheck // closing a probe
		}
	case "unix-connect":
		// The node's and the core's sockets are named Unix sockets, which Landlock does not guard.
		var socket int
		socket, err = syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
		if err == nil {
			err = syscall.Connect(socket, &syscall.SockaddrUnix{Name: arguments[0]})
			_ = syscall.Close(socket) //nolint:errcheck // closing a probe
		}
	case "unix-pair":
		var pair [2]int
		pair, err = syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
		if err == nil {
			_ = syscall.Close(pair[0]) //nolint:errcheck // closing a probe
			_ = syscall.Close(pair[1]) //nolint:errcheck // closing a probe
		}
	}
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// runSelfForAnswer runs this binary with arguments and returns what it printed.
func runSelfForAnswer(t *testing.T, arguments []string) string {
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

// attemptConfined runs one attempt as a confined tool given access.
func attemptConfined(t *testing.T, access confine.Access, what string, arguments ...string) string {
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
	return runSelfForAnswer(t, helperArguments(actAsShim, spec))
}

// attemptUnconfined runs one attempt as a plain process.
func attemptUnconfined(t *testing.T, what string, arguments ...string) string {
	t.Helper()
	return runSelfForAnswer(t, helperArguments(append([]string{attempt, what}, arguments...)...))
}

// requireMechanisms fails the test where a mechanism is absent, unless the job lists it (D190 §2).
func requireMechanisms(t *testing.T) {
	t.Helper()
	for _, state := range confine.MechanismStates() {
		var absent error
		if !state.IsInForce {
			absent = errors.New(state.Reason + "; " + state.Remedy)
		}
		testutil.Require(t, state.Mechanism, absent)
	}
}

// The confined tool is the first the kernel kills when the machine runs out of memory, so a film
// that drives FFmpeg's memory up fails FFmpeg, not the node (D248).
func TestEscape_AConfinedToolIsTheFirstKilledWhenMemoryRunsOut(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	access := confine.Access{Read: []string{"/proc/self/oom_score_adj"}}
	if got := attemptConfined(t, access, "oom-score"); got != "allowed 1000" {
		t.Errorf("the confined tool's oom_score_adj: %s; want 1000", got)
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
	listening := listenUnix(t, filepath.Join(elsewhere, "core.sock"))
	for _, check := range []struct {
		what      string
		arguments []string
	}{
		{"read", []string{secret}},
		{"write", []string{filepath.Join(elsewhere, "planted.txt")}},
		// Refused by our seccomp filter alone: Landlock limits TCP's connect and bind, not the socket,
		// and governs no connect(2) to a named Unix socket, as the core's and the node's are (N1).
		{"inet-socket", nil},
		{"unix-connect", []string{listening}},
		{"unix-pair", nil},
	} {
		if got := attemptUnconfined(t, check.what, check.arguments...); got != "allowed" {
			t.Fatalf("unconfined, %s %v: %s; want allowed, so the refusal below is ours (D190 §3)",
				check.what, check.arguments, got)
		}
		if got := attemptConfined(t, access, check.what, check.arguments...); !strings.HasPrefix(got, "refused") {
			t.Errorf("confined, %s %v: %s; want refused", check.what, check.arguments, got)
		}
	}
	if got := attemptConfined(t, access, "read", source); got != "allowed" {
		t.Errorf("confined, reading the source it was given: %s; want allowed", got)
	}
	if got := attemptConfined(t, access, "write", filepath.Join(output, "segment.m4s")); got != "allowed" {
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

// A run that uses the GPU may do what its driver needs to start, and nothing more: create a Unix
// socket (CUDA asks NVIDIA's daemon over one, and listens on one of its own), read the machine's
// device numbers, and name its threads (D331). Every other run may do none of it, and a GPU run still
// opens no network socket and reads nothing it was not given, the keys to the core's and the node's
// sockets among it.
func TestEscape_OnlyAGPURunOpensAUnixSocketAndNamesItsThreadsAsItsDriverNeeds(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	secret := filepath.Join(t.TempDir(), "core.key")
	if err := os.WriteFile(secret, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	needs := []struct {
		what      string
		arguments []string
	}{{"unix-socket", nil}, {"read", []string{"/proc/devices"}}, {"name-thread", nil}}
	for _, need := range needs {
		if got := attemptUnconfined(t, need.what, need.arguments...); got != "allowed" {
			t.Fatalf("unconfined, %s %v: %s; want allowed, so the refusal below is ours (D190 §3)",
				need.what, need.arguments, got)
		}
		got := attemptConfined(t, confine.Access{}, need.what, need.arguments...)
		if !strings.HasPrefix(got, "refused") {
			t.Errorf("confined without the GPU, %s %v: %s; want refused", need.what, need.arguments, got)
		}
		if got := attemptConfined(t, confine.Access{UsesGPU: true}, need.what, need.arguments...); got != "allowed" {
			t.Errorf("confined with the GPU, %s %v: %s; want allowed", need.what, need.arguments, got)
		}
	}
	for _, refused := range []struct {
		what      string
		arguments []string
	}{{"inet-socket", nil}, {"unix-pair", nil}, {"read", []string{secret}}} {
		got := attemptConfined(t, confine.Access{UsesGPU: true}, refused.what, refused.arguments...)
		if !strings.HasPrefix(got, "refused") {
			t.Errorf("confined with the GPU, %s %v: %s; want refused", refused.what, refused.arguments, got)
		}
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

// listenUnix listens on a Unix socket at path, as the core's node socket does, accepting and
// closing what connects until the test ends, and returns path.
func listenUnix(t *testing.T, path string) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // the test's own listener
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = connection.Close() //nolint:errcheck // a probe's connection
		}
	}()
	return path
}

// attemptProcess tries to start a process, leave its session or process group, call clone3, or
// start threads.
func attemptProcess(what string) error {
	switch what {
	case "start-process":
		//nolint:gosec // G204: it starts this test binary itself, to see whether starting a process is refused
		child, err := syscall.ForkExec("/proc/self/exe", []string{"helper", "-test.run=^$"}, &syscall.ProcAttr{})
		if err == nil {
			_, err = syscall.Wait4(child, nil, 0, nil)
		}
		if err != nil {
			return fmt.Errorf("start a process: %w", err)
		}
		return nil
	case "session":
		if _, err := syscall.Setsid(); err != nil {
			return fmt.Errorf("leave its session: %w", err)
		}
		return nil
	case "process-group":
		if err := syscall.Setpgid(0, 0); err != nil {
			return fmt.Errorf("leave its process group: %w", err)
		}
		return nil
	case "clone3":
		// A zero-sized clone_args: the kernel refuses it with EINVAL; the filter answers ENOSYS first.
		_, _, errno := syscall.RawSyscall(unix.SYS_CLONE3, 0, 0, 0)
		return errno
	case "threads":
		return startThreads()
	}
	return fmt.Errorf("no attempt %q", what)
}

// startThreads starts operating-system threads, as FFmpeg's decoders and encoders do, and has each
// make a system call: Go creates a thread with clone(CLONE_THREAD) for each goroutine locked to one.
func startThreads() error {
	const threads = 8
	var waiting sync.WaitGroup
	var mutex sync.Mutex
	var first error
	for range threads {
		waiting.Go(func() {
			runtime.LockOSThread()
			var facts syscall.Stat_t
			if err := syscall.Fstat(0, &facts); err != nil {
				mutex.Lock()
				if first == nil {
					first = fmt.Errorf("a thread's system call: %w", err)
				}
				mutex.Unlock()
			}
		})
	}
	waiting.Wait()
	return first
}

// A confined run starts no process and leaves neither its session nor its process group, and its
// threads still start: a process it started, or one in another group, would escape the kill that ends
// the run (ffstreamer's specification, Launching). The clone3 call answers ENOSYS, not EPERM, so glibc and Go
// fall back to a clone the filter can read; EPERM there failed FFmpeg's threads (ffstreamer's third
// audit, M3).
func TestEscape_AConfinedToolStartsNoProcessAndStaysInItsGroupButStartsThreads(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	for _, what := range []string{"start-process", "session", "process-group", "clone3", "threads"} {
		if got := attemptUnconfined(t, what); what != "clone3" && got != "allowed" {
			t.Fatalf("unconfined, %s: %s; want allowed, so the refusal below is ours (D190 §3)", what, got)
		}
	}
	for _, what := range []string{"start-process", "session", "process-group"} {
		if got := attemptConfined(t, confine.Access{}, what); !strings.HasPrefix(got, "refused") {
			t.Errorf("confined, %s: %s; want refused", what, got)
		}
	}
	if got := attemptConfined(t, confine.Access{}, "clone3"); got != "refused: "+syscall.ENOSYS.Error() {
		t.Errorf("confined, clone3: %s; want refused with ENOSYS, so the C library falls back to clone", got)
	}
	if got := attemptConfined(t, confine.Access{}, "threads"); got != "allowed" {
		t.Errorf("confined, starting threads: %s; want allowed", got)
	}
}
