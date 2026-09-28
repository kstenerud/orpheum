// SPDX-License-Identifier: GPL-3.0-or-later

// Package hardware is what the node's machine has, as the System page shows it (D225): the GPU
// devices it can see, with the driver behind each and whether the node's user may open it. How
// full a disk is, is internal/platform/diskspace's.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - A render node and an NVIDIA device are each reported with their driver, and one the node's
//     user may not open says why. Enforced by TestGPUs_ReportsEachDeviceItsDriverAndWhyItCannotBeOpened.
//
// # Read before changing
//
//   - D225: the System page, and what M3's shows of hardware encoding.
//
// # Where to change
//
//   - gpu.go: the GPU devices.
package hardware
