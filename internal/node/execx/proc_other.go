// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package execx

// sampleGroup knows nothing of a process group without Linux's /proc: a stall there is judged by
// its position alone, and called stuck.
func sampleGroup(int) groupSample { return groupSample{} }
