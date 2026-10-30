// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// The filesystem rights Landlock knows, by the ABI that added them. A ruleset handles every right
// its kernel knows, so anything no rule grants is refused.
const (
	fsRightsABI1 = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK | unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	fsRightsABI2 = fsRightsABI1 | unix.LANDLOCK_ACCESS_FS_REFER
	fsRightsABI3 = fsRightsABI2 | unix.LANDLOCK_ACCESS_FS_TRUNCATE
	fsRightsABI5 = fsRightsABI3 | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
)

// The rights a rule grants.
const (
	readTree = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	runTool  = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_EXECUTE
	// Libraries are read, and executed as the kernel executes a tool's dynamic loader.
	libraryRights = readTree | unix.LANDLOCK_ACCESS_FS_EXECUTE
	// The rights a rule on a file, not a directory, may grant.
	fileRights = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	useDevice = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE
	// A GPU's device nodes are opened read-write and driven by ioctl.
	useGPU    = readTree | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	writeTree = readTree | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR
)

// systemReads are where the dynamic loader and FFmpeg's drivers read from: the system's own
// libraries, never a user's files, and nothing a run could have written. Executing is granted
// too, because the kernel executes a dynamically linked tool's loader from here. Each is granted
// when it exists.
func systemReads() []string {
	return []string{"/lib", "/lib64", "/usr/lib", "/usr/lib64", "/usr/local/lib", "/etc/ld.so.cache"}
}

// gpuDevices are the device nodes a hardware encode opens: Intel's and AMD's render nodes, and
// NVIDIA's, every GPU that detection finds (NVIDIADevices). Each is granted when it exists.
func gpuDevices() []string { return gpuDevicesUnder("/dev") }

// gpuDevicesUnder is gpuDevices with the device directory named, so a test can build it.
func gpuDevicesUnder(devDirectory string) []string {
	devices := []string{
		filepath.Join(devDirectory, "dri"), filepath.Join(devDirectory, "nvidiactl"),
		filepath.Join(devDirectory, "nvidia-uvm"), filepath.Join(devDirectory, "nvidia-modeset"),
	}
	return append(devices, NVIDIADevices(devDirectory)...)
}

// gpuDescriptions are what a GPU's drivers read to find their device: never a user's files.
func gpuDescriptions() []string {
	return []string{"/sys/devices", "/sys/class/drm", "/sys/bus/pci", "/proc/driver/nvidia", "/etc/OpenCL"}
}

// gpuDriverReads are the files a GPU's driver reads as it starts, CUDA's among them: the machine's
// device numbers, its processor, the lowest address a mapping may take, and the run's own memory map
// and status, none of them a user's (D331 §3). Without /proc/devices, CUDA starts nvidia-modprobe to
// make its device nodes, which no run may. /proc/self names the shim, whose process the tool becomes.
func gpuDriverReads() []string {
	return []string{
		"/proc/devices", "/proc/cpuinfo", "/proc/sys/vm/mmap_min_addr", "/proc/self/maps",
		"/proc/self/status",
	}
}

// gpuThreadNames is where a GPU's driver names its threads, its own process's: CUDA fails to start
// when it cannot (D331 §3).
const gpuThreadNames = "/proc/self/task"

// nameThreads are the rights to name a thread: its comm file is opened to write, and truncated.
const nameThreads = unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE

// devicesAlways are the device nodes any run may read and write.
func devicesAlways() []string {
	return []string{"/dev/null", "/dev/zero", "/dev/urandom", "/dev/random"}
}

// landlockABI is the Landlock ABI the kernel offers, or the errno that says why it offers none.
func landlockABI() (int, error) {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, errno
	}
	return int(abi), nil
}

// landlockState is Landlock's state on this machine, with the errno translated into a cause and a
// remedy (D112).
func landlockState() State {
	_, err := landlockABI()
	return landlockStateOf(err)
}

// landlockStateOf is Landlock's state given what asking for its ABI answered.
func landlockStateOf(err error) State {
	state := State{Mechanism: MechanismLandlock}
	switch {
	case err == nil:
		state.IsInForce = true
	case errors.Is(err, unix.EOPNOTSUPP):
		state.CauseCode = CauseDisabledAtBoot
		state.Reason = "the kernel has Landlock, but it is not enabled at boot"
		state.Remedy = "add landlock to the lsm= list on the kernel command line, for example " +
			"lsm=landlock,lockdown,yama,integrity,apparmor, and reboot"
	case errors.Is(err, unix.ENOSYS):
		state.CauseCode = CauseKernelWithout
		state.Reason = "the kernel is too old for Landlock, or was built without it"
		state.Remedy = "run a kernel of 5.13 or newer built with Landlock"
	case errors.Is(err, unix.EPERM):
		state.CauseCode = CauseBlockedByRuntime
		state.Reason = "the container runtime blocks Landlock's system calls"
		state.Remedy = "upgrade the container runtime (Docker allows them from 23), " +
			"or allow landlock_* in its seccomp profile"
	default:
		state.CauseCode = CauseUnexplained
		state.Reason = fmt.Sprintf("Landlock answered %v", err)
		state.Remedy = "report it; the node cannot say what would fix it"
	}
	return state
}

// rulesetFor is the ruleset a kernel of this Landlock ABI is asked for: every filesystem right it
// knows handled, so what no rule grants is refused; TCP's bind and connect handled, so no run may
// do either; and, where it can, signals and abstract sockets kept inside the run.
func rulesetFor(abi int) unix.LandlockRulesetAttr {
	handled := uint64(fsRightsABI1)
	switch {
	case abi >= 5:
		handled = fsRightsABI5
	case abi >= 3:
		handled = fsRightsABI3
	case abi >= 2:
		handled = fsRightsABI2
	}
	attributes := unix.LandlockRulesetAttr{Handled_access_fs: handled}
	if abi >= 4 {
		attributes.Handled_access_net = unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
	}
	if abi >= 6 {
		attributes.Scoped = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL
	}
	return attributes
}

// grant is one rule: a path, the rights beneath it, and whether the run needs it to exist.
type grant struct {
	path       string
	rights     uint64
	isRequired bool
}

// grantsFor are the rules a spec's run gets: the tool, the system's libraries, a few device nodes,
// and what the spec reads and writes. Only a run that uses the GPU is granted a device's ioctl,
// and only on the GPU's device nodes.
func grantsFor(spec Spec) []grant {
	grants := []grant{{path: spec.Tool, rights: runTool, isRequired: true}}
	for _, path := range systemReads() {
		grants = append(grants, grant{path: path, rights: libraryRights})
	}
	for _, path := range devicesAlways() {
		grants = append(grants, grant{path: path, rights: useDevice})
	}
	if spec.Access.UsesGPU {
		for _, path := range gpuDevices() {
			grants = append(grants, grant{path: path, rights: useGPU})
		}
		for _, path := range gpuDescriptions() {
			grants = append(grants, grant{path: path, rights: readTree})
		}
		for _, path := range gpuDriverReads() {
			grants = append(grants, grant{path: path, rights: unix.LANDLOCK_ACCESS_FS_READ_FILE})
		}
		grants = append(grants, grant{path: gpuThreadNames, rights: nameThreads})
	}
	for _, path := range spec.Access.Read {
		grants = append(grants, grant{path: path, rights: readTree, isRequired: true})
	}
	for _, path := range spec.Access.Write {
		grants = append(grants, grant{path: path, rights: writeTree, isRequired: true})
	}
	return grants
}

// restrictLandlock confines the calling thread, and what it executes, to the spec's grants.
func restrictLandlock(abi int, spec Spec) error {
	attributes := rulesetFor(abi)
	// Passed up to Scoped, the last field rulesetFor sets: x/sys v0.49 added the quiet fields after
	// it, which a kernel without them takes only when they are zero.
	//nolint:gosec // G103: the ruleset's attributes, passed as the syscall defines them
	ruleset, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attributes)), unsafe.Offsetof(attributes.Quiet_access_fs), 0)
	if errno != 0 {
		return fmt.Errorf("create a Landlock ruleset: %w", errno)
	}
	defer func() { _ = unix.Close(int(ruleset)) }() //nolint:errcheck // the thread keeps the domain
	for _, rule := range grantsFor(spec) {
		err := addLandlockRule(int(ruleset), rule.path, rule.rights&attributes.Handled_access_fs)
		if err != nil && (rule.isRequired || !errors.Is(err, os.ErrNotExist)) {
			return err
		}
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, ruleset, 0, 0); errno != 0 {
		return fmt.Errorf("restrict to the Landlock ruleset: %w", errno)
	}
	return nil
}

// addLandlockRule adds a rule granting rights beneath path; a file is granted only the rights a file can
// have.
func addLandlockRule(ruleset int, path string, rights uint64) error {
	file, err := os.Open(path) //nolint:gosec // a path the run names, opened to name it to Landlock
	if err != nil {
		return fmt.Errorf("name %s to Landlock: %w", path, err)
	}
	defer readonly.Close(file)
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("name %s to Landlock: %w", path, err)
	}
	if !info.IsDir() {
		rights &= fileRights
	}
	//nolint:gosec // G115: a descriptor fits in an int32
	rule := unix.LandlockPathBeneathAttr{Allowed_access: rights, Parent_fd: int32(file.Fd())}
	//nolint:gosec // G103: the rule, passed as the syscall defines it
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("grant %s under Landlock: %w", path, errno)
	}
	return nil
}
