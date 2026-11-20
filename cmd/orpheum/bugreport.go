// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/core/problemreport"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// bugreportForm is the download a form word of `orpheum bugreport` asks for, D74's, and false for
// any other word. There is no default: the person says which each time (D524 §4).
func bugreportForm(word string) (problemreport.Form, bool) {
	switch word {
	case "safe":
		return problemreport.FormShareable, true
	case "unsafe":
		return problemreport.FormFull, true
	}
	return "", false
}

// coreComponent is the core's name in a report, as the keeper names it.
const coreComponent = string(kindCore)

// bugreportUsage is what `orpheum bugreport` says when its form word is missing or wrong.
const bugreportUsage = `Usage: orpheum bugreport safe|unsafe --instance <directory> [--words "..."] [--include-other]

Prints a problem report in Markdown, made on this machine for a core that does not start (D524 §5):
safe hides every title, path, name and address it finds behind a tag, to post where others read;
unsafe names everything, with the table of what each tag stands for, for you alone. Say which: there
is no default.
`

// coreLogTailBytes is the most of the core's log file a report reads, its end: the recent log a core
// keeps is far less (D524 §3), and a file appended to for years is not read whole.
const coreLogTailBytes = 64 << 20

// commandBugreport prints a problem report made on the command line, by the builder the System
// page's download is made by, from the config, the database and the core's log file alone: doctor's
// findings, the version, the machine, what the database says, and the core's recent log where it
// writes it to a file (D524 §5). It opens the database without migrating or writing to it, and asks
// no node.
func commandBugreport(ctx context.Context, call invocation) int {
	if len(call.arguments) == 0 {
		return write(call.stderr, bugreportUsage, exitUsageError)
	}
	formWord := call.arguments[0]
	form, ok := bugreportForm(formWord)
	if !ok {
		return write(call.stderr, fmt.Sprintf("orpheum bugreport: %q is not a form\n\n", formWord)+bugreportUsage,
			exitUsageError)
	}
	flags := flag.NewFlagSet("orpheum bugreport "+formWord, flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	instanceDir := flags.String("instance", "", "the instance directory, which holds orpheum.toml (required)")
	words := flags.String("words", "", "your own words: what went wrong, and what you did")
	includeOther := flags.Bool("include-other", false,
		"with unsafe, name an other library's videos too, and put them in the table (D349 §10)")
	if err := flags.Parse(call.arguments[1:]); err != nil {
		return exitUsageError
	}
	if *instanceDir == "" || flags.NArg() > 0 {
		return write(call.stderr, "orpheum bugreport: --instance is required, and nothing after the flags\n\n"+
			bugreportUsage, exitUsageError)
	}
	report := gatherBugreport(ctx, call, *instanceDir, clock.System{}.Now())
	report.Command, report.Words = "orpheum bugreport "+formWord, *words
	rendered, err := problemreport.Render(report, problemreport.Options{Form: form, IncludeOther: *includeOther})
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, rendered.Whole, exitSuccess)
}

// gatherBugreport is the report of the core whose instance is instanceDir, as of now: what of it
// cannot be read is said, with why, and the rest gathered still.
func gatherBugreport(ctx context.Context, call invocation, instanceDir string, now time.Time) problemreport.Report {
	built := thisBuild()
	report := problemreport.Report{
		Made: now, Version: built.Version, Salt: problemreport.NewSalt(),
		Device: []problemreport.Fact{{Name: "System", Value: runtime.GOOS + "/" + runtime.GOARCH + ", " +
			runtime.Version() + ", " + strconv.Itoa(runtime.NumCPU()) + " CPUs"}},
	}
	core := problemreport.Component{Name: coreComponent, Version: built.Version}
	config, configReport, err := loadInstanceConfig(instanceDir, kindCore)
	if err != nil {
		report.Device = append(report.Device,
			problemreport.Fact{Name: "Config", Value: "does not load: " + err.Error()})
		core.Absent = "the config did not load, so where the core writes its log is not known"
		report.Components = []problemreport.Component{core, nodesAbsent("the config did not load")}
		return report
	}
	report.Device = append(report.Device, problemreport.Fact{Name: "Config", Value: "loads from " + instanceDir})
	logger := slog.New(slog.NewTextHandler(call.stderr, nil))
	for _, found := range append(configFindings(configReport), diagnose(ctx, config, logger, now)...) {
		value := found.description
		if found.remedy != "" {
			value += " — " + found.remedy
		}
		report.Device = append(report.Device, problemreport.Fact{
			Name:  "Doctor: " + verdictWord(found.verdict),
			Value: value,
		})
	}
	nodes := bugreportDatabase(ctx, config, &report)
	// The instance's own paths are known too, so doctor's words around them stay words.
	for _, path := range []string{
		instanceDir, config.Instance.PersistentDir, config.Instance.CacheDir, config.Instance.BackupsDir,
		config.Instance.RunDir, config.Log.Destination,
	} {
		if filepath.IsAbs(path) {
			report.Known = append(report.Known, problemreport.Known{Kind: problemreport.KindPath, Value: path})
		}
	}
	core, unread := coreLogComponent(config, built.Version)
	if unread != "" {
		report.Device = append(report.Device, problemreport.Fact{Name: "The core's log", Value: unread})
	}
	report.Components = append([]problemreport.Component{core}, nodes...)
	return report
}

// bugreportDatabase adds to report what the database says (whether it opens and why not, its schema
// step and this binary's) and every name it holds that says whose the report is, and is each node it
// knows, named absent: the command line asks no node.
func bugreportDatabase(ctx context.Context, config Config, report *problemreport.Report) []problemreport.Component {
	say := func(name, value string) {
		report.Device = append(report.Device, problemreport.Fact{Name: name, Value: value})
	}
	_, database, err := openExistingDatabase(ctx, config)
	if err != nil {
		say("Database", "does not open: "+err.Error())
		say("Names hidden", "only those this report finds itself: the database did not open, so its titles, "+
			"libraries, people, devices and nodes are not looked for; review the report the more carefully")
		return []problemreport.Component{nodesAbsent("the database did not open")}
	}
	defer func() { _ = database.Close() }() //nolint:errcheck // a report; a close error changes nothing in it
	step := "its schema step cannot be read"
	if state, stateErr := database.State(ctx); stateErr == nil {
		step = "at schema step " + strconv.Itoa(state.Version)
	}
	binaryStep := "this binary's cannot be read"
	if version, versionErr := store.SchemaVersion(); versionErr == nil {
		binaryStep = "this binary's is " + strconv.Itoa(version)
	}
	say("Database", "opens, "+step+"; "+binaryStep)
	if err = problemreport.AddKnown(ctx, database, "", report); err != nil {
		say("Names hidden", "only those this report finds itself: "+err.Error()+"; review the report the more "+
			"carefully")
	}
	liveNodes, err := database.Read().ListLiveNodes(ctx)
	if err != nil {
		return []problemreport.Component{nodesAbsent("the nodes could not be listed: " + err.Error())}
	}
	nodes := make([]problemreport.Component, 0, len(liveNodes))
	for _, node := range liveNodes {
		nodes = append(nodes, problemreport.Component{Name: node.Name, Node: true, Absent: nodeNotAsked})
	}
	return nodes
}

// nodeNotAsked is why a report made on the command line has no node's log.
const nodeNotAsked = "not asked: a report made on the command line asks no node; one made on the System page " +
	"while the core runs asks each for its own"

// nodesAbsent is every node, named absent: why, and that the command line would ask none anyway. Its
// name is no node's, so it is not tagged.
func nodesAbsent(why string) problemreport.Component {
	return problemreport.Component{Name: "every node", Absent: why + "; " + nodeNotAsked}
}

// coreLogComponent is the core's recent log, read from the file the config has it write its log to,
// as of its newest line; a core writing to standard error is named absent, with why. A note on the
// lines that did not read is said beside it.
func coreLogComponent(config Config, version string) (core problemreport.Component, unread string) {
	core = problemreport.Component{Name: coreComponent, Version: version}
	if config.Log.Destination == logToStderr {
		core.Absent = "made from the command line, and the core writes its log to standard error " +
			"(log.destination), which only its service's journal keeps (`docker logs`, `journalctl`); a core " +
			"that is not running keeps no recent log in memory (D524 §3)"
		return core, ""
	}
	format, err := slogx.ParseFormat(config.Log.Format)
	if err != nil {
		core.Absent = "its log's format cannot be read: " + err.Error()
		return core, ""
	}
	file, err := os.Open(config.Log.Destination)
	if err != nil {
		core.Absent = "its log file, log.destination, cannot be read: " + err.Error()
		return core, ""
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read only
	input, err := logTail(file)
	if err != nil {
		core.Absent = "its log file, log.destination, cannot be read: " + err.Error()
		return core, ""
	}
	recent := slogx.NewRecent(slogx.ProcessRecentLimits())
	newest, unreadLines, err := recent.KeepWritten(input, format)
	core.Lines = recent.Lines(newest)
	switch {
	case err != nil:
		unread = "read from log.destination until " + err.Error()
	case unreadLines > 0:
		unread = fmt.Sprintf("read from log.destination; %d lines did not read as its %s lines", unreadLines, format)
	}
	return core, unread
}

// logTail is the last coreLogTailBytes bytes of file, from the first whole line in them.
func logTail(file *os.File) (io.Reader, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= coreLogTailBytes {
		return file, nil
	}
	if _, err = file.Seek(info.Size()-coreLogTailBytes, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	if _, err = reader.ReadString('\n'); err != nil {
		return nil, err
	}
	return reader, nil
}
