// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// errInputEnded is the owner's answers running out (a closed terminal, or a test's script) before the
// guided command finished. The command keeps what it made, so running it again carries on.
var errInputEnded = errors.New("the answers ended before the ceremony did: " +
	"run the same command again to carry on from where it stopped")

// prompter asks the owner plain questions: it writes them to out and reads the answers from in, line by
// line, so a test drives a whole ceremony with a script. A default is shown in brackets and Enter
// accepts it.
type prompter struct {
	in  *bufio.Reader
	out io.Writer
	// wrote is the first failure to write to out, which say keeps for the command to report.
	wrote error
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{in: bufio.NewReader(in), out: out}
}

// say writes a line of explanation.
func (asker *prompter) sayf(format string, arguments ...any) {
	if _, err := fmt.Fprintf(asker.out, format+"\n", arguments...); err != nil && asker.wrote == nil {
		asker.wrote = err
	}
}

// line reads one answer, trimmed.
func (asker *prompter) line() (string, error) {
	text, err := asker.in.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || text == "") {
		return "", errInputEnded
	}
	return strings.TrimSpace(text), nil
}

// ask asks question and returns the answer, fallback when the owner just presses Enter.
func (asker *prompter) ask(question, fallback string) (string, error) {
	if fallback != "" {
		asker.sayf("%s [%s]", question, fallback)
	} else {
		asker.sayf("%s", question)
	}
	if _, err := io.WriteString(asker.out, "> "); err != nil && asker.wrote == nil {
		asker.wrote = err
	}
	answer, err := asker.line()
	if err != nil {
		return "", err
	}
	if answer == "" {
		return fallback, nil
	}
	return answer, nil
}

// number asks for a whole number of at least least.
func (asker *prompter) number(question string, fallback, least int) (int, error) {
	for {
		answer, err := asker.ask(question, strconv.Itoa(fallback))
		if err != nil {
			return 0, err
		}
		if count, parseErr := strconv.Atoi(answer); parseErr == nil && count >= least {
			return count, nil
		}
		asker.sayf("Please type a whole number, %d or more.", least)
	}
}

// yes asks a yes or no question.
func (asker *prompter) yes(question string, fallback bool) (bool, error) {
	shown := "yes"
	if !fallback {
		shown = "no"
	}
	for {
		answer, err := asker.ask(question+" (yes or no)", shown)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		asker.sayf("Please answer yes or no.")
	}
}

// choose asks for one of options, by its name, and returns it.
func (asker *prompter) choose(question string, options []string, fallback string) (string, error) {
	for {
		answer, err := asker.ask(question+" ("+strings.Join(options, ", ")+")", fallback)
		if err != nil {
			return "", err
		}
		for _, option := range options {
			if strings.EqualFold(answer, option) {
				return option, nil
			}
		}
		asker.sayf("Please type one of: %s.", strings.Join(options, ", "))
	}
}

// press waits for Enter.
func (asker *prompter) press(message string) error {
	asker.sayf("%s", message)
	if _, err := io.WriteString(asker.out, "Press Enter when ready. "); err != nil && asker.wrote == nil {
		asker.wrote = err
	}
	_, err := asker.line()
	return err
}
