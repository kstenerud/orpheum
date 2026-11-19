// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Form is which of a report's two downloads to render (D74, D524 §4). There is no default.
type Form string

const (
	// FormShareable has every identifying value replaced by its tag, for posting where others read.
	FormShareable Form = "shareable"
	// FormFull names everything as gathered, with the table of what each tag stands for, for the
	// household's admin alone.
	FormFull Form = "full"
)

// Options are one download's choices.
type Options struct {
	// Form is required: an empty or unknown one is an error.
	Form Form
	// IncludeOther puts an other library's names in the full form and its table (D349 §10); without
	// it they stay as their tags, and the report says which it did.
	IncludeOther bool
}

// Report is a problem report as gathered: what the caller fills in, before any of it is made safe.
type Report struct {
	// Made is when the report was made; Version the Orpheum version that made it.
	Made    time.Time
	Version string
	// Salt is the report's own, random and kept with it: one salt gives one value one tag, in
	// every download of the report; another report's salt gives it another. It must not be empty.
	Salt []byte
	// Reporter is who made the report, from which device; both names are tagged.
	Reporter Reporter
	// Words are the person's own, scrubbed.
	Words string
	// Page is the path of the page the report was made from; its query and fragment are dropped.
	Page string
	// Device are the device's facts in the order it gave them: a name of the client's, and a value
	// scrubbed.
	Device []Fact
	// Playback is the playback the report was made from, if any.
	Playback *Playback
	// Components are the core and each node, in the order the report lists them.
	Components []Component
	// Known are the values the caller knows can identify (every title, file path, library root,
	// user, device and house name, address), looked for in free text. There may be thousands.
	Known []Known
	// OtherRoots are the root paths of the libraries of kind other (D349 §10): a path under one is
	// an other video's.
	OtherRoots []string
}

// Reporter is who made a report.
type Reporter struct {
	UserName, DeviceName string
}

// Fact is one fact about the reporting device.
type Fact struct {
	Name, Value string
}

// Playback is the state of the playback a report was made from. Its ids are numbers, and kept.
type Playback struct {
	PlaybackID, ItemID, FileID int64
	Position                   time.Duration
	// Quality and State are as the player gives them; scrubbed.
	Quality, State string
	// File is the file's shape, if known.
	File *FileShape
}

// FileShape is a file's shape without its identity (D74): its path is tagged, depth and extension
// kept; nothing else of it names the film. Its streams' titles and the container's tags are never
// asked for.
type FileShape struct {
	Path      string
	Container string
	Duration  time.Duration
	SizeBytes int64
	Streams   []Stream
}

// Stream is one stream's shape.
type Stream struct {
	Index         int
	Kind          string // video, audio, subtitle, attachment
	Codec         string
	Profile       string
	Level         string
	BitDepth      int
	Width, Height int
	FrameRate     string
	ChannelLayout string
	Language      string
	Dispositions  []string
}

// Component is the core or one node, with its recent log or why it has none.
type Component struct {
	// Name is the component's; a node's is tagged.
	Name string
	Node bool
	// Version is the component's Orpheum version.
	Version string
	// Lines are the component's recent log, oldest first.
	Lines []slogx.Line
	// Absent, when not empty, is why the component gave no log ("no answer within 5 s, last seen 4 h
	// ago"); the report lists it as absent, never leaves it out. Lines must then be empty.
	Absent string
}

// Known is a value the caller knows identifies something, of a kind.
type Known struct {
	Kind  Kind
	Value string
	// Other marks an other video's value (D349 §10): its title, say.
	Other bool
}

// Rendered is a report in one form.
type Rendered struct {
	// Summary is at most SummaryLimit bytes, to paste into a GitHub issue: the newest lines that fit,
	// saying how many it leaves out.
	Summary string
	// Whole is everything, to attach as a file.
	Whole string
}

// SummaryLimit is the most bytes a summary has: under GitHub's 65,535-byte issue body, with room
// for the person's own text (D74).
const SummaryLimit = 64000

// ErrNoForm is a download that named no form, or one there is not.
var ErrNoForm = errors.New("problem report: the form must be shareable or full")

// Render is report in the form options name.
func Render(report Report, options Options) (Rendered, error) {
	if options.Form != FormShareable && options.Form != FormFull {
		return Rendered{}, fmt.Errorf("%w, not %q", ErrNoForm, options.Form)
	}
	if len(report.Salt) == 0 {
		return Rendered{}, errors.New("problem report: no salt: every report needs its own")
	}
	for _, each := range report.Known {
		if !each.Kind.known() {
			return Rendered{}, fmt.Errorf("problem report: a known value of kind %q, which there is not", each.Kind)
		}
	}
	for at, each := range report.Components {
		if each.Absent != "" && len(each.Lines) > 0 {
			return Rendered{}, fmt.Errorf("problem report: component %d is absent and has lines", at)
		}
	}
	builder := newBuilder(report)
	builder.build(report) // names every field's value first, so free text finds them
	builder.collecting = false
	built := builder.build(report)
	built.options = options
	built.table = builder.names.order
	return renderMarkdown(built), nil
}

// builder makes a report's model: everything that can identify named or scrubbed.
type builder struct {
	names    *names
	scrubber *scrubber
	known    map[string]Known
	// collecting is the first pass, which names the fields' values and scrubs nothing.
	collecting bool
}

func newBuilder(report Report) *builder {
	names := newNames(report.Salt, report.OtherRoots)
	made := &builder{names: names, scrubber: newScrubber(names), known: map[string]Known{}, collecting: true}
	for _, root := range report.OtherRoots {
		made.addKnown(Known{Kind: KindPath, Value: root, Other: true})
	}
	for _, each := range report.Known {
		made.addKnown(each)
	}
	return made
}

func (builder *builder) addKnown(each Known) {
	if each.Kind == KindPath && builder.names.underOtherRoot(each.Value) {
		each.Other = true
	}
	if _, ok := builder.known[each.Value]; !ok {
		builder.known[each.Value] = each
	}
	builder.scrubber.know(each.Kind, each.Value, each.Other)
}

// freeText is words scrubbed, once every field's value is named (the first pass scrubs nothing).
func (builder *builder) freeText(words string) text {
	if builder.collecting {
		return literal(words)
	}
	return builder.scrubber.scrub(words)
}

// model is a report made safe, before it is read in a form.
type model struct {
	options     Options
	made        time.Time
	version     string
	user        text
	device      text
	words       text
	page        text
	facts       []factRow
	playback    []factRow
	file        []factRow
	streams     []Stream
	components  []componentModel
	table       []*entry
	summaryNote string
}

type factRow struct {
	name  string
	value text
}

type componentModel struct {
	name     text
	node     bool
	version  string
	absent   text
	isAbsent bool
	lines    []lineModel
	// shown is how many of the newest lines this rendering shows.
	shown int
}

type lineModel struct {
	time    time.Time
	level   slog.Level
	message string
	fields  []field
}

func (builder *builder) build(report Report) *model {
	built := &model{
		made: report.Made, version: report.Version,
		user:   builder.person(KindUser, report.Reporter.UserName),
		device: builder.person(KindDevice, report.Reporter.DeviceName),
		page:   builder.page(report.Page),
	}
	for _, each := range report.Components {
		component := componentModel{node: each.Node, version: each.Version, isAbsent: each.Absent != ""}
		if each.Node {
			component.name = builder.person(KindNode, each.Name)
		} else {
			component.name = literal(each.Name)
		}
		component.absent = builder.freeText(each.Absent)
		for _, line := range each.Lines {
			made := lineModel{time: line.Time, level: line.Level, message: line.Message}
			for _, each := range line.Fields {
				made.fields = append(made.fields, builder.fieldOf(each))
			}
			component.lines = append(component.lines, made)
		}
		component.shown = len(component.lines)
		built.components = append(built.components, component)
	}
	if playback := report.Playback; playback != nil {
		built.playback = []factRow{
			{"Playback", literal(strconv.FormatInt(playback.PlaybackID, 10))},
			{"Item", literal(strconv.FormatInt(playback.ItemID, 10))},
			{"File", literal(strconv.FormatInt(playback.FileID, 10))},
			{"Position", literal(playback.Position.String())},
			{"Quality", builder.freeText(playback.Quality)},
			{"State", builder.freeText(playback.State)},
		}
		if file := playback.File; file != nil {
			path, depth := literal(""), ""
			if file.Path != "" {
				path = named(builder.nameValue(file.Path))
				depth = strconv.Itoa(strings.Count(strings.Trim(file.Path, "/"), "/") + 1)
			}
			built.file = []factRow{
				{"Path", path},
				{"Depth", literal(depth)},
				{"Extension", literal(extension(file.Path[strings.LastIndexByte(file.Path, '/')+1:]))},
				{"Container", literal(file.Container)},
				{"Duration", literal(file.Duration.String())},
				{"Size", literal(strconv.FormatInt(file.SizeBytes, 10) + " bytes")},
			}
			built.streams = file.Streams
		}
	}
	for _, each := range report.Device {
		built.facts = append(built.facts, factRow{each.Name, builder.freeText(each.Value)})
	}
	built.words = builder.freeText(report.Words)
	return built
}

// person is a name of kind, tagged; the scrubber looks for it in free text too.
func (builder *builder) person(kind Kind, value string) text {
	if value == "" {
		return literal("")
	}
	builder.scrubber.know(kind, value, false)
	return named(builder.names.name(kind, value, false))
}

// page is the page's path with its query and fragment dropped, each segment that is a number or a
// route's own word (lower-case letters and dashes) kept and any other tagged.
func (builder *builder) page(value string) text {
	suffix := ""
	if at := strings.IndexAny(value, "?#"); at >= 0 {
		suffix = value[at:at+1] + dropped
		value = value[:at]
	}
	var out text
	for at, part := range strings.Split(value, "/") {
		if at > 0 {
			out = append(out, segment{literal: "/"})
		}
		if routeWord(part) {
			out = append(out, segment{literal: part})
			continue
		}
		out = append(out, segment{named: builder.names.name(KindText, part, false)})
	}
	return append(out, segment{literal: suffix})
}

func routeWord(part string) bool {
	for _, r := range part {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
