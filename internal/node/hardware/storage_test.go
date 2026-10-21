// SPDX-License-Identifier: AGPL-3.0-or-later

package hardware

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSystem is a made-up /proc, /sys and /run/udev under a test's directory.
type fakeSystem struct {
	t    *testing.T
	root string
}

func (system fakeSystem) write(path, content string) {
	system.t.Helper()
	full := filepath.Join(system.root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		system.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		system.t.Fatal(err)
	}
}

func (system fakeSystem) link(path, target string) {
	system.t.Helper()
	full := filepath.Join(system.root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		system.t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(system.root, target), full); err != nil {
		system.t.Fatal(err)
	}
}

// disk makes a whole disk, its partitions and their device numbers, as sysfs shows them.
func (system fakeSystem) disk(name, majorMinor, rotational string, partitions map[string]string) {
	system.t.Helper()
	system.write("sys/devices/pci/block/"+name+"/queue/rotational", rotational+"\n")
	system.link("sys/block/"+name, "sys/devices/pci/block/"+name)
	system.link("sys/class/block/"+name, "sys/devices/pci/block/"+name)
	system.link("sys/dev/block/"+majorMinor, "sys/devices/pci/block/"+name)
	for partition, partitionMajorMinor := range partitions {
		target := "sys/devices/pci/block/" + name + "/" + partition
		system.write(target+"/partition", "1\n")
		system.link("sys/class/block/"+partition, target)
		system.link("sys/dev/block/"+partitionMajorMinor, target)
	}
}

// A path is on the device of the mount it is under, a mount inside it on its own, and a pool,
// array or volume is followed to its member disks to say how it reads (D240 §5): two roots of one
// ZFS pool share it, an md array of a spinning and a solid-state disk spins, and a network mount is
// one.
func TestDevices_FollowsPoolsArraysAndPartitionsToTheirDisksToTellHowEachReads(t *testing.T) {
	t.Parallel()
	system := fakeSystem{t: t, root: t.TempDir()}
	system.write("proc/self/mountinfo", `20 1 0:30 / / rw - overlay rootfs rw
30 20 0:60 / /media/films rw - zfs tank/media/films rw
31 30 8:17 / /media/films/Extra\040Disk rw - ext4 /dev/sdb1 rw
32 20 0:61 / /media/shows rw - zfs tank/media/shows rw
33 20 0:62 / /media/nas rw - nfs4 server:/export rw
34 20 9:0 / /media/raid rw - ext4 /dev/md0 rw
35 20 8:1 / /prepared rw - ext4 /dev/sda1 rw
`)
	system.disk("sda", "8:0", "0", map[string]string{"sda1": "8:1"})
	system.disk("sdb", "8:16", "0", map[string]string{"sdb1": "8:17"})
	system.disk("sdc", "8:32", "1", map[string]string{"sdc1": "8:33"})
	system.disk("sdd", "8:48", "0", nil)
	system.disk("sde", "8:64", "1", map[string]string{"sde1": "8:65"})
	system.disk("md0", "9:0", "0", nil)
	system.write("sys/devices/pci/block/md0/slaves/sdc1/.keep", "")
	system.write("sys/devices/pci/block/md0/slaves/sdd/.keep", "")
	system.write("run/udev/data/b8:65", "N:sde1\nE:ID_FS_TYPE=zfs_member\nE:ID_FS_LABEL=tank\n")
	system.write("run/udev/data/b8:48", "N:sdd\nE:ID_FS_TYPE=linux_raid_member\n")

	devices, err := devicesUnder(system.root,
		[]string{"/media/films", "/media/shows", "/media/nas", "/media/raid", "/prepared"})
	if err != nil {
		t.Fatal(err)
	}
	want := []nodeproto.Device{
		{
			Key: "zfs:tank", Kind: nodeproto.DeviceSpinning, Filesystem: "zfs", Disks: []string{"sde"},
			MountPaths: []string{"/media/films", "/media/shows"},
		},
		{
			Key: "disk:sdb", Kind: nodeproto.DeviceSSD, Filesystem: "ext4", Disks: []string{"sdb"},
			MountPaths: []string{"/media/films/Extra Disk"},
		},
		{
			Key: "network:server:/export", Kind: nodeproto.DeviceNetwork, Filesystem: "nfs4",
			MountPaths: []string{"/media/nas"},
		},
		{
			Key: "disk:sdc+sdd", Kind: nodeproto.DeviceSpinning, Filesystem: "ext4", Disks: []string{"sdc", "sdd"},
			MountPaths: []string{"/media/raid"},
		},
		{
			Key: "disk:sda", Kind: nodeproto.DeviceSSD, Filesystem: "ext4", Disks: []string{"sda"},
			MountPaths: []string{"/prepared"},
		},
	}
	if len(devices) != len(want) {
		t.Fatalf("found %+v, want %+v", devices, want)
	}
	for index, device := range devices {
		if device.Key != want[index].Key || device.Kind != want[index].Kind ||
			device.Filesystem != want[index].Filesystem || !slices.Equal(device.Disks, want[index].Disks) ||
			!slices.Equal(device.MountPaths, want[index].MountPaths) {
			t.Errorf("device %d is %+v, want %+v", index, device, want[index])
		}
	}
}

// A pool whose disks the node cannot see, in a container without the host's /run/udev, and a path
// on an overlay, are of unknown kind: one reader at a time, as for a spinning disk.
func TestDevices_APoolOrOverlayWhoseDisksCannotBeSeenIsUnknown(t *testing.T) {
	t.Parallel()
	system := fakeSystem{t: t, root: t.TempDir()}
	system.write("proc/self/mountinfo", `20 1 0:30 / / rw - overlay rootfs rw
30 20 0:60 / /media/films rw - zfs tank/media/films rw
`)
	devices, err := devicesUnder(system.root, []string{"/media/films", "/srv/prepared"})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].Key != "zfs:tank" || devices[0].Kind != nodeproto.DeviceUnknown ||
		devices[1].Key != "unknown:0:30" || devices[1].Kind != nodeproto.DeviceUnknown ||
		!slices.Equal(devices[1].MountPaths, []string{"/srv/prepared"}) {
		t.Errorf("found %+v, want the pool and the overlay, both of unknown kind", devices)
	}
}

// usbDisk makes a whole disk behind a USB bridge, with its device number, as sysfs shows it.
func (system fakeSystem) usbDisk(name, majorMinor, rotational string) {
	system.t.Helper()
	target := "sys/devices/pci/usb2/2-1/host0/block/" + name
	system.write(target+"/queue/rotational", rotational+"\n")
	system.write(target+"/dev", majorMinor+"\n")
	system.link("sys/block/"+name, target)
	system.link("sys/class/block/"+name, target)
	system.link("sys/dev/block/"+majorMinor, target)
}

// The rotation rate udev read from a drive says best how it reads; without one, a disk behind a USB
// bridge that the kernel says spins is unknown, since most bridges hide the drive from the kernel
// (the owner's USB SSD on devnas, 2026-10-03); a disk on its own bus that spins, spins.
func TestDevices_ADiskBehindUSBIsKnownByItsDrivesRotationRateOrElseUnknown(t *testing.T) {
	t.Parallel()
	system := fakeSystem{t: t, root: t.TempDir()}
	system.write("proc/self/mountinfo", `30 1 8:32 / /usb-ssd rw - ext4 /dev/sdc rw
31 1 8:48 / /usb-told rw - ext4 /dev/sdd rw
32 1 8:64 / /sata rw - ext4 /dev/sde rw
`)
	system.usbDisk("sdc", "8:32", "1")
	system.usbDisk("sdd", "8:48", "1")
	system.write("run/udev/data/b8:48", "E:ID_BUS=usb\nE:ID_ATA_ROTATION_RATE_RPM=0\n")
	system.disk("sde", "8:64", "1", nil)
	devices, err := devicesUnder(system.root, []string{"/usb-ssd", "/usb-told", "/sata"})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]nodeproto.DeviceKind{}
	for _, device := range devices {
		kinds[device.Key] = device.Kind
	}
	want := map[string]nodeproto.DeviceKind{
		"disk:sdc": nodeproto.DeviceUnknown, "disk:sdd": nodeproto.DeviceSSD, "disk:sde": nodeproto.DeviceSpinning,
	}
	if !maps.Equal(kinds, want) {
		t.Errorf("the disks read as %v, want %v", kinds, want)
	}
}

// A path is on the mount with the longest mount point it is under, and the root mount holds a path
// under no other; a mount point that is a prefix of a name is not one it is under.
func TestMountHolding_IsTheLongestMountPointThePathIsUnderTheRootHoldingTheRest(t *testing.T) {
	t.Parallel()
	mounts := []mount{
		{majorMinor: "8:1", mountPoint: "/"},
		{majorMinor: "8:17", mountPoint: "/media"},
		{majorMinor: "8:33", mountPoint: "/media/films"},
	}
	for path, want := range map[string]string{
		"/media/films/Wing (1979)/Wing.mkv": "8:33",
		"/media/films-old/Wing.mkv":          "8:17",
		"/media":                              "8:17",
		"/srv/shows":                          "8:1",
	} {
		holding, found := mountHolding(mounts, path)
		if !found || holding.majorMinor != want {
			t.Errorf("%s is on %+v (found %v), want %s", path, holding, found, want)
		}
	}
}

// How a share's reads behave when its server goes away is its mount's: NFS waits for good unless
// mounted soft, SMB only when mounted hard, virtiofs and FUSE may wait as what they pass on does,
// and a local disk has no server (D233 §10; staging, 2026-10-21).
func TestDevices_SayWhetherAShareWaitsForeverWhenItsServerGoes(t *testing.T) {
	t.Parallel()
	system := fakeSystem{t: t, root: t.TempDir()}
	system.write("proc/self/mountinfo", `20 1 0:30 / / rw - overlay rootfs rw
31 20 0:41 / /hard rw - nfs4 nas:/films rw,vers=4.2,hard,proto=tcp,timeo=600,retrans=2
32 20 0:42 / /soft rw - nfs nas:/shows rw,vers=3,soft,proto=tcp,timeo=100,retrans=3
33 20 0:43 / /smb rw - cifs //nas/music rw,vers=3.1.1,soft
34 20 0:44 / /smb-hard rw - cifs //nas/books rw,vers=3.1.1,hard
35 20 0:45 / /passed rw - virtiofs media rw
`)
	paths := []string{"/hard", "/soft", "/smb", "/smb-hard", "/passed", "/local"}
	devices, err := devicesUnder(system.root, paths)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]nodeproto.ShareWhenGone{
		"/hard": nodeproto.ShareWaitsForever, "/soft": "", "/smb": "", "/smb-hard": nodeproto.ShareWaitsForever,
		"/passed": nodeproto.ShareMayWaitForever, "/local": "",
	}
	for _, path := range paths {
		key := nodeproto.Devices(devices).KeyOf(path)
		index := slices.IndexFunc(devices, func(device nodeproto.Device) bool { return device.Key == key })
		if index < 0 || devices[index].WhenGone != want[path] {
			t.Errorf("%s is on %+v; want its share to be %q when gone", path, devices, want[path])
		}
	}
}

// A path's mount point is the deepest mount over it in the mount table, read without touching the
// filesystem: what the disk gate keys a disk mounted inside a root by (D328 §1).
func TestMountPointOf_IsTheDeepestMountHoldingThePath(t *testing.T) {
	t.Parallel()
	system := fakeSystem{t: t, root: t.TempDir()}
	system.write("proc/self/mountinfo", `20 1 0:30 / / rw - overlay rootfs rw
30 20 0:60 / /media rw - nfs nas:/media rw
31 30 8:32 / /media/disk1 rw - ext4 /dev/sdc rw
`)
	for path, want := range map[string]string{
		"/media/disk1/Wing.mkv": "/media/disk1", "/media/Metropolis.mkv": "/media", "/media/disk10/x": "/media",
		"/srv/prepared": "/",
	} {
		if got, found := mountPointUnder(system.root, path); !found || got != want {
			t.Errorf("%s: %s, %t; want %s", path, got, found, want)
		}
	}
	if _, found := mountPointUnder(t.TempDir(), "/media"); found {
		t.Error("with no mount table, a mount point was found")
	}
}
