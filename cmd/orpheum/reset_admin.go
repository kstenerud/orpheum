// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// commandResetAdmin is the operator's way back in from the server's own machine (D294): everyone
// who administers is demoted, and the person --name names becomes the admin with the password read
// from standard input, asked twice without echo at a terminal, or its first line when piped. No
// account is removed; a name no one has is made. It runs beside a running server.
func commandResetAdmin(ctx context.Context, call invocation) int {
	var name string
	config, configReport, err := call.loadConfigFromFlags("reset-admin", kindCore, func(flags *flag.FlagSet) {
		flags.StringVar(&name, "name", "", "the person to make the admin: an account's name, made if no one has it")
	})
	if err != nil {
		return call.fail(err)
	}
	if strings.TrimSpace(name) == "" {
		return call.fail(errors.New("--name is required: the person to make the admin"))
	}
	call.printConfigNotes(configReport)
	password, err := call.newPassword(name)
	if err != nil {
		return call.fail(err)
	}
	logger, closeLog, err := call.logger(config)
	if err != nil {
		return call.fail(err)
	}
	defer closeLog() //nolint:errcheck // the log of a command that has written its answer
	_, database, err := openExistingDatabase(ctx, config)
	if err != nil {
		return call.fail(err)
	}
	authService := auth.NewService(database, clock.System{}, rand.Reader,
		settings.NewService(database, clock.System{}, logger))
	reset, err := authService.ResetAdmin(ctx, name, password)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.fail(err)
	}
	logger.WarnContext(ctx, "the operator reset who administers", "admin", name, "demoted", reset.Demoted,
		"isCreated", reset.IsCreated)
	return write(call.stdout, resetSummary(name, reset), exitSuccess)
}

// resetSummary says what a reset did.
func resetSummary(name string, reset auth.AdminReset) string {
	made := ""
	if reset.IsCreated {
		made = ", a new account"
	}
	demoted := "no one else administered"
	if len(reset.Demoted) > 0 {
		demoted = "no longer admins: " + strings.Join(reset.Demoted, ", ")
	}
	return fmt.Sprintf("%s is now the only admin%s, signed out everywhere; %s\n", name, made, demoted)
}

// newPassword reads the new admin's password: at a terminal asked twice with echo off, otherwise
// the first line of standard input.
func (call invocation) newPassword(name string) (string, error) {
	terminal, isFile := call.stdin.(*os.File)
	if !isFile || !isTerminal(terminal) {
		line, err := bufio.NewReader(call.stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read the password: %w", err)
		}
		if password := strings.TrimRight(line, "\r\n"); password != "" {
			return password, nil
		}
		return "", errors.New("give the password on standard input")
	}
	first, err := readHidden(terminal, call.stderr, "New password for "+name+": ")
	if err != nil {
		return "", err
	}
	second, err := readHidden(terminal, call.stderr, "The same again: ")
	switch {
	case err != nil:
		return "", err
	case first != second:
		return "", errors.New("the two passwords are not the same")
	case first == "":
		return "", errors.New("the password is empty")
	}
	return first, nil
}
