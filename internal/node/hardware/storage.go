// SPDX-License-Identifier: AGPL-3.0-or-later

package hardware

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

// isNetworkFilesystem is whether a filesystem type is mounted over the network.
func isNetworkFilesystem(filesystem string) bool {
	switch filesystem {
	case "nfs", "nfs4", "cifs", "smb3", "smbfs", "9p", "ceph", "glusterfs", "fuse.sshfs", "fuse.rclone", "davfs":
		return true
	}
	return false
}

// whenGone is how a read of a mount behaves when what it is shared from goes away (D233 §10): NFS
// waits for good unless mounted soft, SMB only when mounted hard; virtiofs, 9p and FUSE pass on
// another's filesystem, whose mounts the node cannot see; a local disk has no server to lose.
func whenGone(each mount) nodeproto.ShareWhenGone {
	has := func(option string) bool { return slices.Contains(each.superOptions, option) }
	switch {
	case each.filesystem == "nfs" || each.filesystem == "nfs4":
		if has("soft") || has("softerr") {
			return ""
		}
		return nodeproto.ShareWaitsForever
	case each.filesystem == "cifs" || each.filesystem == "smb3":
		if has("hard") {
			return nodeproto.ShareWaitsForever
		}
		return ""
	case each.filesystem == "virtiofs" || each.filesystem == "9p" || strings.HasPrefix(each.filesystem, "fuse"):
		return nodeproto.ShareMayWaitForever
	}
	return ""
}

// maxSlaveDepth bounds how far a volume is followed through the devices beneath it: an LVM volume
// on an md array is two.
const maxSlaveDepth = 4

// Devices are what paths, absolute and with their links resolved, are on, and how each reads
// (D240 §5): the device each is on and every one mounted inside it, a pool, array or volume
// followed to its member disks. It reads the system's mount table, /sys, and udev's database for
// the disks of a ZFS pool.
func Devices(paths []string) ([]nodeproto.Device, error) {
	return devicesUnder(sysTree{root: "/", branchesOf: mergerfsBranches}, paths)
}

// devicesUnder is Devices with /proc, /sys and /run/udev read under the tree's root: "/" on a node,
// a made-up tree in a test.
func devicesUnder(system sysTree, paths []string) ([]nodeproto.Device, error) {
	mounts, err := readMounts(system.path("proc", "self", "mountinfo"))
	if err != nil {
		return nil, err
	}
	byKey := map[string]*nodeproto.Device{}
	var keys []string
	add := func(each mount, mountPath string) {
		described := system.describe(each, mounts)
		device, found := byKey[described.Key]
		if !found {
			device = described
			byKey[device.Key] = device
			keys = append(keys, device.Key)
		}
		if !slices.Contains(device.MountPaths, mountPath) {
			device.MountPaths = append(device.MountPaths, mountPath)
		}
	}
	for _, path := range paths {
		if holding, found := mountHolding(mounts, path); found {
			add(holding, path)
		}
		for _, inside := range mountsInside(mounts, path) {
			add(inside, inside.mountPoint)
		}
	}
	devices := make([]nodeproto.Device, 0, len(keys))
	for _, key := range keys {
		devices = append(devices, *byKey[key])
	}
	return devices, nil
}

// mount is one line of the mount table.
type mount struct {
	majorMinor string // the device number, "<major>:<minor>"; major 0 is no block device
	mountPoint string
	filesystem string
	source     string
	// superOptions are the filesystem's own options, as the mount table's last field says them.
	superOptions []string
}

// readMounts reads a mountinfo file (proc(5)). A mount point mounted over again is the later one.
func readMounts(path string) ([]mount, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the system's own mount table
	if err != nil {
		return nil, fmt.Errorf("read the mount table: %w", err)
	}
	return parseMounts(content), nil
}

// parseMounts is the mounts a mountinfo file's content lists, in the order mounted; a mount point
// mounted over again is the later one, in the place of the later.
func parseMounts(content []byte) []mount {
	var mounts []mount
	latest := map[string]int{} // the index in mounts of each mount point's latest mount
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		before, after, found := strings.Cut(scanner.Text(), " - ")
		fields, tail := strings.Fields(before), strings.Fields(after)
		if !found || len(fields) < 5 || len(tail) < 2 {
			continue
		}
		next := mount{
			majorMinor: fields[2], mountPoint: unescapeMountField(fields[4]), filesystem: tail[0],
			source: unescapeMountField(tail[1]),
		}
		if len(tail) > 2 {
			next.superOptions = strings.Split(tail[2], ",")
		}
		if earlier, isMountedOver := latest[next.mountPoint]; isMountedOver {
			mounts[earlier].mountPoint = "" // mounted over: dropped below
		}
		latest[next.mountPoint] = len(mounts)
		mounts = append(mounts, next)
	}
	return slices.DeleteFunc(mounts, func(each mount) bool { return each.mountPoint == "" })
}

// unescapeMountField undoes the mount table's octal escapes: "\040" is a space.
func unescapeMountField(field string) string {
	var unescaped strings.Builder
	for index := 0; index < len(field); index++ {
		if field[index] == '\\' && index+3 < len(field) {
			if code, err := strconv.ParseUint(field[index+1:index+4], 8, 8); err == nil {
				unescaped.WriteByte(byte(code))
				index += 3
				continue
			}
		}
		unescaped.WriteByte(field[index])
	}
	return unescaped.String()
}

// mountHolding is the mount path is on: the one with the longest mount point it is under.
func mountHolding(mounts []mount, path string) (mount, bool) {
	var holding mount
	found := false
	for _, each := range mounts {
		if pathtree.IsAtOrUnder(path, each.mountPoint) && (!found || len(each.mountPoint) > len(holding.mountPoint)) {
			holding, found = each, true
		}
	}
	return holding, found
}

// mountsInside are the mounts whose mount points are under path, not at it.
func mountsInside(mounts []mount, path string) []mount {
	var inside []mount
	for _, each := range mounts {
		if each.mountPoint != path && pathtree.IsAtOrUnder(each.mountPoint, path) {
			inside = append(inside, each)
		}
	}
	return inside
}

// zfsPool is the pool a ZFS mount's dataset is in, and whether it is ZFS.
func (each mount) zfsPool() (string, bool) {
	if each.filesystem != "zfs" {
		return "", false
	}
	pool, _, _ := strings.Cut(each.source, "/")
	return pool, true
}

// key names the device a mount is on (nodeproto.Device.Key).
func (each mount) key(system sysTree) string {
	if pool, isZFS := each.zfsPool(); isZFS {
		return "zfs:" + pool
	}
	if isNetworkFilesystem(each.filesystem) {
		return "network:" + each.source
	}
	if disks := system.disksOfMount(each); len(disks) > 0 {
		return "disk:" + strings.Join(disks, "+")
	}
	return "unknown:" + each.majorMinor
}

// sysTree reads /sys and udev's database under root.
type sysTree struct {
	root string
	// branchesOf are the folders a mergerfs pool's folder is made of, in its branches, as the pool
	// says them; none when it does not say.
	branchesOf func(folder string) []string
}

func (system sysTree) path(parts ...string) string {
	return filepath.Join(append([]string{system.root}, parts...)...)
}

// describe is the device a mount is on, without its mount paths: a mergerfs pool's, where the node
// sees every branch of it, the devices its branches are on together.
func (system sysTree) describe(each mount, mounts []mount) *nodeproto.Device {
	if each.filesystem == mergerfs {
		if pool, isSeen := system.describePool(each, mounts); isSeen {
			return pool
		}
	}
	device := &nodeproto.Device{
		Key: each.key(system), Kind: nodeproto.DeviceUnknown, Filesystem: each.filesystem, WhenGone: whenGone(each),
	}
	if isNetworkFilesystem(each.filesystem) {
		device.Kind = nodeproto.DeviceNetwork
		return device
	}
	if pool, isZFS := each.zfsPool(); isZFS {
		device.Disks = system.zfsDisks(pool)
	} else {
		device.Disks = system.disksOfMount(each)
	}
	device.Kind = system.kindOf(device.Disks)
	return device
}

// mergerfs is a mergerfs pool's filesystem type, as the mount table names it.
const mergerfs = "fuse.mergerfs"

// describePool is a mergerfs pool as the devices of its branches together, and whether the node
// sees each branch: in a container the pool's folder is bound in and none of its branches' disks are
// (the owner's friend's OpenMediaVault, 2026-11-19), and the pool stays a FUSE mount of unknown kind.
// The devices together spin when one spins, are a network share's when one is, and wait for a server
// as the most waiting of them does; two roots on one pool share it.
func (system sysTree) describePool(pool mount, mounts []mount) (*nodeproto.Device, bool) {
	if system.branchesOf == nil {
		return nil, false
	}
	branches := system.branchesOf(pool.mountPoint)
	if len(branches) == 0 {
		return nil, false
	}
	var members []*nodeproto.Device
	for _, branch := range branches {
		if _, err := os.Stat(system.path(branch)); err != nil {
			return nil, false
		}
		holding, found := mountHolding(mounts, branch)
		if !found || holding.filesystem == mergerfs {
			return nil, false
		}
		members = append(members, system.describe(holding, nil))
	}
	device := &nodeproto.Device{Filesystem: pool.filesystem}
	var keys []string
	var kinds []nodeproto.DeviceKind
	for _, member := range members {
		keys = append(keys, member.Key)
		kinds = append(kinds, member.Kind)
		device.Disks = append(device.Disks, member.Disks...)
		device.WhenGone = mostWaiting(device.WhenGone, member.WhenGone)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	device.Key = keys[0]
	if len(keys) > 1 {
		device.Key = "mergerfs:" + strings.Join(keys, "+")
	}
	slices.Sort(device.Disks)
	device.Disks = slices.Compact(device.Disks)
	device.Kind = kindTogether(kinds)
	return device, true
}

// kindTogether is how devices read together, as kindOf has disks: a network share's when one is one,
// else spinning when one spins, unknown when one cannot be told, and SSD when all are.
func kindTogether(kinds []nodeproto.DeviceKind) nodeproto.DeviceKind {
	for _, kind := range []nodeproto.DeviceKind{
		nodeproto.DeviceNetwork, nodeproto.DeviceSpinning, nodeproto.DeviceUnknown,
	} {
		if slices.Contains(kinds, kind) {
			return kind
		}
	}
	return nodeproto.DeviceSSD
}

// mostWaiting is the one of two ways a read behaves when a share goes away that waits the more.
func mostWaiting(one, other nodeproto.ShareWhenGone) nodeproto.ShareWhenGone {
	for _, waiting := range []nodeproto.ShareWhenGone{nodeproto.ShareWaitsForever, nodeproto.ShareMayWaitForever} {
		if one == waiting || other == waiting {
			return waiting
		}
	}
	return ""
}

// mergerfsBranches are the folders a mergerfs pool's folder is in, one in each branch that holds
// it, as the pool's user.mergerfs.allpaths attribute says them (mergerfs(1)); none when it says
// nothing, on a pool mounted without extended attributes or not mergerfs at all.
func mergerfsBranches(folder string) []string {
	size, err := unix.Getxattr(folder, "user.mergerfs.allpaths", nil)
	if err != nil || size <= 0 {
		return nil
	}
	value := make([]byte, size)
	if size, err = unix.Getxattr(folder, "user.mergerfs.allpaths", value); err != nil {
		return nil
	}
	var folders []string
	for path := range strings.SplitSeq(string(value[:size]), "\x00") {
		if path != "" {
			folders = append(folders, filepath.Clean(path)) // a pool's root says each branch's with a "/" after
		}
	}
	return folders
}

// kindOf is how disks read together: spinning when one of them spins, SSD when none does, unknown
// when any cannot be told or there are none.
func (system sysTree) kindOf(disks []string) nodeproto.DeviceKind {
	if len(disks) == 0 {
		return nodeproto.DeviceUnknown
	}
	kind := nodeproto.DeviceSSD
	for _, disk := range disks {
		switch system.diskKind(disk) {
		case nodeproto.DeviceSpinning:
			return nodeproto.DeviceSpinning
		case nodeproto.DeviceUnknown:
			kind = nodeproto.DeviceUnknown
		case nodeproto.DeviceSSD, nodeproto.DeviceNetwork:
		}
	}
	return kind
}

// diskKind is how one whole disk reads. The rotation rate udev read from the drive, when it could,
// says best: 0 is a solid-state disk. Else the kernel's rotational flag, except that behind a USB
// bridge, which most often hides the drive from the kernel, a flag that says it spins is a guess, and
// the disk is unknown (the owner's USB SSD on devnas, 2026-10-03).
func (system sysTree) diskKind(disk string) nodeproto.DeviceKind {
	if rpm, isKnown := system.udevRotationRPM(disk); isKnown {
		if rpm == "0" {
			return nodeproto.DeviceSSD
		}
		return nodeproto.DeviceSpinning
	}
	flag, err := os.ReadFile(system.path("sys/block", disk, "queue/rotational"))
	switch {
	case err != nil:
		return nodeproto.DeviceUnknown
	case strings.TrimSpace(string(flag)) == "0":
		return nodeproto.DeviceSSD
	case system.isBehindUSB(disk):
		return nodeproto.DeviceUnknown
	}
	return nodeproto.DeviceSpinning
}

// udevRotationRPM is the rotation rate udev's database says a disk's drive told it, and whether it
// says one.
func (system sysTree) udevRotationRPM(disk string) (string, bool) {
	majorMinor, err := os.ReadFile(system.path("sys/block", disk, "dev"))
	if err != nil {
		return "", false
	}
	entry := system.path("run/udev/data", "b"+strings.TrimSpace(string(majorMinor)))
	content, err := os.ReadFile(entry) //nolint:gosec // udev's own database, named by the kernel's number
	if err != nil {
		return "", false
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		if rpm, isRate := strings.CutPrefix(line, "E:ID_ATA_ROTATION_RATE_RPM="); isRate {
			return rpm, true
		}
	}
	return "", false
}

// isBehindUSB is whether a disk hangs off a USB bus, as its place among the system's devices says.
func (system sysTree) isBehindUSB(disk string) bool {
	target, err := os.Readlink(system.path("sys/class/block", disk))
	return err == nil && slices.ContainsFunc(strings.Split(target, "/"), func(part string) bool {
		return strings.HasPrefix(part, "usb")
	})
}

// disksOfMount are the whole disks a block filesystem is on: through its device number, or, where
// that names no block device (btrfs), the device its source names, with every other device of a
// btrfs filesystem.
func (system sysTree) disksOfMount(each mount) []string {
	name := system.blockName(each.majorMinor)
	if name == "" && strings.HasPrefix(each.source, "/dev/") {
		name = filepath.Base(each.source)
	}
	if name == "" {
		return nil
	}
	names := []string{name}
	if each.filesystem == "btrfs" {
		names = system.btrfsDevices(name)
	}
	var disks []string
	for _, member := range names {
		disks = append(disks, system.disksOf(member, 0)...)
	}
	slices.Sort(disks)
	return slices.Compact(disks)
}

// blockName is the kernel name of the block device numbered majorMinor, or "" when it is none.
func (system sysTree) blockName(majorMinor string) string {
	if strings.HasPrefix(majorMinor, "0:") {
		return ""
	}
	target, err := os.Readlink(system.path("sys/dev/block", majorMinor))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// disksOf are the whole disks beneath a block device: itself when it is one, its disk when it is a
// partition, and those beneath each device an array or volume is made of.
func (system sysTree) disksOf(name string, depth int) []string {
	if target, err := os.Readlink(system.path("sys/class/block", name)); err == nil {
		partition := system.path("sys/class/block", name, "partition")
		if _, statErr := os.Stat(partition); statErr == nil { //nolint:gosec // the kernel's own /sys
			name = filepath.Base(filepath.Dir(target))
		}
	}
	slaves, err := os.ReadDir(system.path("sys/block", name, "slaves"))
	if err != nil || len(slaves) == 0 || depth >= maxSlaveDepth {
		return []string{name}
	}
	var disks []string
	for _, slave := range slaves {
		disks = append(disks, system.disksOf(slave.Name(), depth+1)...)
	}
	return disks
}

// btrfsDevices are every device of the btrfs filesystem one of whose devices is name, or name
// alone when /sys/fs/btrfs does not say.
func (system sysTree) btrfsDevices(name string) []string {
	filesystems, err := os.ReadDir(system.path("sys/fs/btrfs"))
	if err != nil {
		return []string{name}
	}
	for _, filesystem := range filesystems {
		entries, readErr := os.ReadDir(system.path("sys/fs/btrfs", filesystem.Name(), "devices"))
		if readErr != nil {
			continue
		}
		var devices []string
		for _, entry := range entries {
			devices = append(devices, entry.Name())
		}
		if slices.Contains(devices, name) {
			return devices
		}
	}
	return []string{name}
}

// zfsDisks are the whole disks of a ZFS pool, as udev's database labels its members; none when the
// database is not there, as in a container that was not given the host's /run/udev.
func (system sysTree) zfsDisks(pool string) []string {
	entries, err := os.ReadDir(system.path("run/udev/data"))
	if err != nil {
		return nil
	}
	var disks []string
	for _, entry := range entries {
		majorMinor, isBlock := strings.CutPrefix(entry.Name(), "b")
		if !isBlock {
			continue
		}
		content, readErr := os.ReadFile(system.path("run/udev/data", entry.Name()))
		if readErr != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		if !slices.Contains(lines, "E:ID_FS_TYPE=zfs_member") || !slices.Contains(lines, "E:ID_FS_LABEL="+pool) {
			continue
		}
		name := system.blockName(majorMinor)
		for _, line := range lines {
			if node, isName := strings.CutPrefix(line, "N:"); isName && name == "" {
				name = node
			}
		}
		if name != "" {
			disks = append(disks, system.disksOf(name, 0)...)
		}
	}
	slices.Sort(disks)
	return slices.Compact(disks)
}
