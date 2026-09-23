// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !dev

package main

// isDevBuild is false in every release build; see build_dev.go.
const isDevBuild = false
