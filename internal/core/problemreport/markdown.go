// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// lineTime is how a log line's time reads: in UTC, to the millisecond, with its date (a report
// holds a day of lines).
const lineTime = "2006-01-02T15:04:05.000Z"

// renderMarkdown is the report in both its outputs: the whole, and the summary that fits a GitHub
// issue, which shows the newest lines that fit and says how many it leaves out.
func renderMarkdown(built *model) Rendered {
	whole := built.markdown()
	if len(whole) <= SummaryLimit {
		return Rendered{Summary: whole, Whole: whole}
	}
	return Rendered{Summary: built.summary(), Whole: whole}
}

// summary shows the newest lines of every component that fit within SummaryLimit, taken newest
// first across all of them.
func (built *model) summary() string {
	for at := range built.components {
		built.components[at].shown = 0
	}
	total := 0
	for _, each := range built.components {
		total += len(each.lines)
	}
	built.summaryNote = fmt.Sprintf("This is the summary, to paste. It leaves out older log lines "+
		"to stay within %d bytes; the whole report, to attach, has every one of the %d.", SummaryLimit, total)
	// Room for the counts' digits, which grow as lines are shown.
	room := SummaryLimit - len(built.markdown()) - 32*len(built.components)
	type newest struct{ component, line int }
	var order []newest
	for component, each := range built.components {
		for line := range each.lines {
			order = append(order, newest{component, line})
		}
	}
	at := func(each newest) time.Time { return built.components[each.component].lines[each.line].time }
	slices.SortStableFunc(order, func(a, b newest) int { return at(b).Compare(at(a)) })
	// A component's lines are shown newest first, so its next is always the newest it has left.
	for _, each := range order {
		component := &built.components[each.component]
		size := len(built.lineRow(component.lines[len(component.lines)-1-component.shown]))
		if size > room {
			break
		}
		room -= size
		component.shown++
	}
	summary := built.markdown()
	for len(summary) > SummaryLimit && built.dropOldestShown() {
		summary = built.markdown()
	}
	if len(summary) > SummaryLimit {
		summary = cut(summary)
	}
	return summary
}

// dropOldestShown shows one line fewer, the oldest shown, and says whether there was one.
func (built *model) dropOldestShown() bool {
	oldest := -1
	var oldestTime time.Time
	for index, each := range built.components {
		if each.shown == 0 {
			continue
		}
		first := each.lines[len(each.lines)-each.shown].time
		if oldest < 0 || first.Before(oldestTime) {
			oldest, oldestTime = index, first
		}
	}
	if oldest < 0 {
		return false
	}
	built.components[oldest].shown--
	return true
}

// cutNote ends a summary too long even without its log lines.
const cutNote = "\n\n**The summary was cut here, to fit an issue; the whole report has the rest.**\n\n</details>\n"

// cut is summary cut at a line's end so that it and cutNote fit within SummaryLimit.
func cut(summary string) string {
	kept := summary[:SummaryLimit-len(cutNote)]
	if at := strings.LastIndexByte(kept, '\n'); at > 0 {
		kept = kept[:at]
	}
	return kept + cutNote
}

func (built *model) markdown() string {
	var out strings.Builder
	built.heading(&out)
	built.wordsSection(&out)
	built.versions(&out)
	writeLines(&out, "## The device", "")
	built.factTable(&out, built.facts)
	built.playbackSection(&out)
	built.logs(&out)
	built.omissions(&out)
	if built.options.Form == FormFull {
		built.tagTable(&out)
	}
	writeLines(&out, "</details>")
	return out.String()
}

// writeLines writes each line, and a line break after it.
func writeLines(out *strings.Builder, lines ...string) {
	for _, each := range lines {
		out.WriteString(each)
		out.WriteString("\n")
	}
}

func (built *model) heading(out *strings.Builder) {
	writeLines(out, "<details>", fmt.Sprintf("<summary>Orpheum problem report: %s, %s, %s</summary>",
		escape(built.version), built.made.UTC().Format(time.RFC3339), built.options.Form), "")
	if built.options.Form == FormShareable {
		writeLines(out,
			"> **Review before sharing.** This report replaces each title, path, name and address it found with a",
			"> tag (`item:…`, `path:…`, `user:…`), the same tag for the same value throughout, so the problem can",
			"> be followed without saying what it was about. The tags are new in every report. Making it safe",
			"> is best-effort: read it before you post it. A problem with a password, a key or signing in",
			"> should not be shared as a report at all; describe it in your own words instead.")
	} else {
		writeLines(out,
			"> **DO NOT SHARE PUBLICLY.** This full report names everything as gathered: titles, paths, people,",
			"> devices and addresses, and ends with what each tag of the shareable report stands for. Keep it to",
			"> yourself, or send it only to someone you trust; post the shareable report instead.")
	}
	writeLines(out, "")
	if built.summaryNote != "" {
		writeLines(out, "*"+built.summaryNote+"*", "")
	}
}

func (built *model) wordsSection(out *strings.Builder) {
	writeLines(out, "## In their words", "")
	for each := range strings.SplitSeq(built.words.read(built.options), "\n") {
		writeLines(out, "> "+escape(each))
	}
	writeLines(out, "", fmt.Sprintf("Made by %s on %s, from the page `%s`.", escape(built.user.read(built.options)),
		escape(built.device.read(built.options)), strings.ReplaceAll(built.page.read(built.options), "`", "'")), "")
}

func (built *model) versions(out *strings.Builder) {
	writeLines(out, "## Versions", "", "| Component | Version |", "| --- | --- |")
	for _, each := range built.components {
		writeLines(out, fmt.Sprintf("| %s | %s |", cell(built.componentName(each)), cell(each.version)))
	}
	writeLines(out, "")
}

func (built *model) playbackSection(out *strings.Builder) {
	if len(built.playback) > 0 {
		writeLines(out, "## The playback", "")
		built.factTable(out, built.playback)
	}
	if len(built.file) == 0 {
		return
	}
	writeLines(out, "### Its file", "")
	built.factTable(out, built.file)
	if len(built.streams) == 0 {
		return
	}
	writeLines(out,
		"| # | Kind | Codec | Profile | Level | Bit depth | Size | Frame rate | Channels | Language | Dispositions |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, each := range built.streams {
		writeLines(out, streamRow(each))
	}
	writeLines(out, "")
}

func streamRow(each Stream) string {
	size, depth := "", ""
	if each.Width > 0 || each.Height > 0 {
		size = fmt.Sprintf("%dx%d", each.Width, each.Height)
	}
	if each.BitDepth > 0 {
		depth = strconv.Itoa(each.BitDepth)
	}
	return fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |", each.Index, cell(each.Kind),
		cell(each.Codec), cell(each.Profile), cell(each.Level), depth, size, cell(each.FrameRate),
		cell(each.ChannelLayout), cell(each.Language), cell(strings.Join(each.Dispositions, ", ")))
}

func (built *model) logs(out *strings.Builder) {
	writeLines(out, "## The logs", "")
	for _, each := range built.components {
		name := escape(built.componentName(each))
		if each.isAbsent {
			writeLines(out, "### "+name+": absent", "",
				"**It gave no log:** "+escape(each.absent.read(built.options)), "")
			continue
		}
		writeLines(out, "### "+name, "")
		switch {
		case len(each.lines) == 0:
			writeLines(out, "No lines in its recent log.")
		case each.shown < len(each.lines):
			writeLines(out, fmt.Sprintf("The newest %d of its %d lines; the %d older are left out of this summary.",
				each.shown, len(each.lines), len(each.lines)-each.shown))
		default:
			writeLines(out, fmt.Sprintf("All %d of its recent lines.", len(each.lines)))
		}
		writeLines(out, "")
		if each.shown == 0 {
			continue
		}
		writeLines(out, "| Time | Level | Message | Fields |", "| --- | --- | --- | --- |")
		for _, made := range each.lines[len(each.lines)-each.shown:] {
			out.WriteString(built.lineRow(made))
		}
		writeLines(out, "")
	}
}

func (built *model) componentName(each componentModel) string {
	name := each.name.read(built.options)
	if each.node {
		return "node " + name
	}
	return name
}

func (built *model) factTable(out *strings.Builder, rows []factRow) {
	out.WriteString("| | |\n| --- | --- |\n")
	for _, each := range rows {
		fmt.Fprintf(out, "| %s | %s |\n", cell(each.name), cell(each.value.read(built.options)))
	}
	out.WriteString("\n")
}

// lineRow is one log line, one table row: its time, level, message and fields.
func (built *model) lineRow(made lineModel) string {
	fields := make([]string, 0, len(made.fields))
	for _, each := range made.fields {
		fields = append(fields, each.key+"="+quoted(each.value.read(built.options)))
	}
	return fmt.Sprintf("| %s | %s | %s | %s |\n", made.time.UTC().Format(lineTime), made.level,
		cell(made.message), cell(strings.Join(fields, " ")))
}

// quoted is a field's value quoted when it would not read as one word.
func quoted(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\n\"=") {
		return strconv.Quote(value)
	}
	return value
}

func (built *model) omissions(out *strings.Builder) {
	out.WriteString("## What this report leaves out\n\n| What | Why |\n| --- | --- |\n")
	rows := [][2]string{
		{"Secrets (keys, passwords, tokens)", "Never logged: a secret logs as `[redacted]` (D194)."},
		{"URLs' queries, fragments and sign-ins", "Dropped from both forms: tokens live there."},
		{"Streams' titles and the file's own tags", "Not gathered: they name the film; its shape is kept (D74)."},
		{"Log lines older than the recent log", "Each process keeps 30 minutes of every line and 24 hours " +
			"at info and above, until it restarts (D524 §3)."},
	}
	if built.options.Form == FormShareable {
		rows = append([][2]string{
			{
				"Titles, paths, people's, devices', nodes' and houses' names, addresses",
				"Replaced by tags: a path's keeps its depth and extension. Values in an error's words are " +
					"replaced where found; that is best-effort.",
			},
		}, rows...)
	} else {
		withheld := 0
		for _, each := range built.table {
			if each.other {
				withheld++
			}
		}
		switch {
		case withheld == 0:
		case built.options.IncludeOther:
			rows = append(rows, [2]string{"Nothing of the other libraries", fmt.Sprintf(
				"Their %d names are included, as this download asked (D349 §10).", withheld)})
		default:
			rows = append(rows, [2]string{"The other libraries' names", fmt.Sprintf(
				"%d left as their tags and out of the table: this download did not ask for them (D349 §10).",
				withheld)})
		}
	}
	for _, each := range built.components {
		if each.shown < len(each.lines) {
			rows = append(rows, [2]string{built.componentName(each) + "'s older lines", fmt.Sprintf(
				"%d left out of this summary to fit an issue; the whole report has them.", len(each.lines)-each.shown)})
		}
	}
	for _, each := range rows {
		fmt.Fprintf(out, "| %s | %s |\n", cell(each[0]), cell(each[1]))
	}
	out.WriteString("\n")
}

func (built *model) tagTable(out *strings.Builder) {
	out.WriteString("## What each tag stands for\n\n")
	out.WriteString("The shareable report of this same report uses these tags.\n\n")
	out.WriteString("| Tag | Stands for |\n| --- | --- |\n")
	for _, each := range built.table {
		if each.other && !built.options.IncludeOther {
			continue
		}
		fmt.Fprintf(out, "| %s | %s |\n", cell(each.tag), cell(each.value))
	}
	out.WriteString("\n")
}

// escape keeps text from being read as HTML: a person's `</details>` must not end the report.
func escape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// cell is text fit for a table's cell: escaped, its pipes and line breaks shown, not obeyed.
func cell(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", `\|`, "\r\n", `\n`, "\n", `\n`,
		"\r", `\n`).Replace(text)
}
