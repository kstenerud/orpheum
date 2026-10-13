// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package main

import (
	"errors"
	"io"
	"os"
)

// isTerminal is false where the server is not built to run: the password is read as a line.
func isTerminal(*os.File) bool {
	return false
}

// readHidden is never called where isTerminal is false.
func readHidden(*os.File, io.Writer, string) (string, error) {
	return "", errors.New("reading a password at a terminal is built for Linux only")
}
