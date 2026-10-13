// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// isTerminal is whether file is a terminal.
func isTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	return err == nil
}

// readHidden asks for a line at the terminal with its echo off, and puts the echo back.
func readHidden(terminal *os.File, prompt io.Writer, asking string) (string, error) {
	descriptor := int(terminal.Fd())
	was, err := unix.IoctlGetTermios(descriptor, unix.TCGETS)
	if err != nil {
		return "", fmt.Errorf("read the terminal: %w", err)
	}
	hidden := *was
	hidden.Lflag &^= unix.ECHO
	if err = unix.IoctlSetTermios(descriptor, unix.TCSETS, &hidden); err != nil {
		return "", fmt.Errorf("turn the terminal's echo off: %w", err)
	}
	defer func() { _ = unix.IoctlSetTermios(descriptor, unix.TCSETS, was) }() //nolint:errcheck // put back as it was
	write(prompt, asking, exitSuccess)
	line, err := bufio.NewReader(terminal).ReadString('\n')
	write(prompt, "\n", exitSuccess)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
