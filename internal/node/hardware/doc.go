// SPDX-License-Identifier: GPL-3.0-or-later

// Package hardware is what the node's machine has, as the System page shows it (D225): the GPU
// devices it can see, with the driver behind each and whether the node's user may open it; and the
// devices its roots are on, and how each reads, which the core schedules by (D240 §5). How full a
// disk is, is internal/platform/diskspace's.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - A render node and an NVIDIA device are each reported with their driver, and one the node's
//     user may not open says why. Enforced by TestGPUs_ReportsEachDeviceItsDriverAndWhyItCannotBeOpened.
//   - The NVIDIA devices reported are the ones a confined encode is granted: both come from
//     confine.NVIDIADevices. TestGPUDevices_AreGrantedEveryNVIDIAGPUDetectionFinds.
//   - A path is on the device of the longest mount point it is under, and a mount inside it on its
//     own; a ZFS pool, an md array, an LVM volume and a btrfs filesystem are followed to their whole
//     disks, spinning when one of them spins, and one whose disks cannot be seen is unknown (D240
//     §5). A disk's drive's rotation rate, when udev read one, says how it reads; without one, a disk
//     behind USB that the kernel says spins is unknown, the kernel guessing through most bridges.
//     TestDevices_FollowsPoolsArraysAndPartitionsToTheirDisksToTellHowEachReads,
//     TestDevices_APoolOrOverlayWhoseDisksCannotBeSeenIsUnknown,
//     TestDevices_ADiskBehindUSBIsKnownByItsDrivesRotationRateOrElseUnknown.
//
// # Read before changing
//
//   - D225: the System page, and what M3's shows of hardware encoding.
//   - D240 §5: the devices the core schedules readers on, and how their kinds are told.
//
// # Where to change
//
//   - gpu.go: the GPU devices. storage.go: the devices the roots are on.
package hardware
