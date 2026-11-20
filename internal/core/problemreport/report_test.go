// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport_test

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/problemreport"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// The values the fixture's report must not show in its shareable form.
const (
	title      = "Velvet Static"
	filePath   = "/media/films/Velvet Static/Velvet Static.mkv"
	root       = "/media/films"
	userName   = "Mirela"
	deviceName = "Hallway Tablet"
	houseName  = "Harbourside Flat"
	address    = "203.0.113.7"
	nodeName   = "attic-nas"
	otherTitle = "Hollow Violet"
	otherRoot  = "/media/other"
	otherPath  = "/media/other/Hollow Violet.mkv"
)

var (
	made = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	salt = []byte("salt-of-one-report")
)

func line(at time.Duration, message string, fields ...slogx.Field) slogx.Line {
	return slogx.Line{Time: made.Add(at), Level: slog.LevelInfo, Message: message, Fields: fields}
}

func str(key, value string) slogx.Field { return slogx.Field{Key: key, Value: slog.StringValue(value)} }

func num(key string, value int64) slogx.Field {
	return slogx.Field{Key: key, Value: slog.Int64Value(value)}
}

func fixture() problemreport.Report {
	return problemreport.Report{
		Made: made, Version: "v0.9.0", Salt: salt,
		Reporter: problemreport.Reporter{UserName: userName, DeviceName: deviceName},
		Words:    "Velvet Static stops at ten minutes at home (203.0.113.7); Mirela tried twice.",
		Page:     "/titles/77?from=search",
		Device: []problemreport.Fact{
			{Name: "Browser", Value: "Firefox 158 on " + deviceName},
			{Name: "Plays", Value: "h264, hevc Main10, aac, opus"},
		},
		Playback: &problemreport.Playback{
			PlaybackID: 9, ItemID: 77, FileID: 31, Position: 10 * time.Minute, Quality: "original",
			State: "stalled",
			File: &problemreport.FileShape{
				Path: filePath, Container: "matroska", Duration: 2 * time.Hour, SizeBytes: 4 << 30,
				Streams: []problemreport.Stream{
					{
						Index: 0, Kind: "video", Codec: "hevc", Profile: "Main 10", Level: "153", BitDepth: 10,
						Width: 3840, Height: 2160, FrameRate: "24000/1001",
					},
					{
						Index: 1, Kind: "audio", Codec: "eac3", ChannelLayout: "5.1(side)", Language: "eng",
						Dispositions: []string{"default"},
					},
				},
			},
		},
		Components: []problemreport.Component{
			{Name: "core", Version: "v0.9.0", Lines: []slogx.Line{
				line(0, "a playback started", str(slogx.EventKey, "playbackStarted"), num("itemId", 77),
					str("itemTitle", title), str("path", filePath), str("house", houseName), str("kind", "transcode")),
				line(time.Second, "a job failed", str("kind", "transcode"), num("attempts", 3),
					str("err", "open "+filePath+": no such file or directory"), str("remote", address)),
				line(2*time.Second, "an other video was scanned", str("path", otherPath), str("title", otherTitle)),
			}},
			{Name: nodeName, Node: true, Version: "v0.9.0", Lines: []slogx.Line{
				line(500*time.Millisecond, "an FFmpeg run ended", str("path", filePath),
					str("why", "the stream of Velvet Static ended early at "+filePath)),
			}},
		},
		Known: []problemreport.Known{
			{Kind: problemreport.KindItem, Value: title},
			{Kind: problemreport.KindPath, Value: filePath},
			{Kind: problemreport.KindPath, Value: root},
			{Kind: problemreport.KindUser, Value: userName},
			{Kind: problemreport.KindDevice, Value: deviceName},
			{Kind: problemreport.KindHouse, Value: houseName},
			{Kind: problemreport.KindNode, Value: nodeName},
			{Kind: problemreport.KindItem, Value: otherTitle, Other: true},
			{Kind: problemreport.KindPath, Value: otherPath},
		},
		OtherRoots: []string{otherRoot},
	}
}

func render(t *testing.T, report problemreport.Report, options problemreport.Options) problemreport.Rendered {
	t.Helper()
	rendered, err := problemreport.Render(report, options)
	if err != nil {
		t.Fatal(err)
	}
	return rendered
}

func shareable(t *testing.T, report problemreport.Report) problemreport.Rendered {
	t.Helper()
	return render(t, report, problemreport.Options{Form: problemreport.FormShareable})
}

// tagOf is the tag the full form's table gives value.
func tagOf(t *testing.T, full, value string) string {
	t.Helper()
	row := regexp.MustCompile(`(?m)^\| ([a-z]+:[^ |]+) \| ` + regexp.QuoteMeta(value) + ` \|$`)
	found := row.FindStringSubmatch(full)
	if found == nil {
		t.Fatalf("the full form's table has no tag for %q:\n%s", value, full)
	}
	return found[1]
}

func TestRender_TheShareableFormNamesNoTitlePathPersonOrAddress(t *testing.T) {
	t.Parallel()
	rendered := shareable(t, fixture())
	for _, output := range []string{rendered.Summary, rendered.Whole} {
		for _, secret := range []string{
			title, root, "Velvet", userName, deviceName, houseName, address, nodeName,
			otherTitle, otherRoot,
		} {
			if strings.Contains(output, secret) {
				t.Errorf("the shareable form shows %q:\n%s", secret, output)
			}
		}
	}
	for _, kept := range []string{
		"no such file or directory", "stops at ten minutes", "hevc", "Main 10", "matroska",
		"5.1(side)", "eng", "/titles/77?[dropped]", "Firefox 158 on device:",
	} {
		if !strings.Contains(rendered.Whole, kept) {
			t.Errorf("the shareable form lost its shape %q:\n%s", kept, rendered.Whole)
		}
	}
}

func TestRender_OneValueHasOneTagInFieldsFreeTextAndEveryComponent(t *testing.T) {
	t.Parallel()
	report := fixture()
	full := render(t, report, problemreport.Options{Form: problemreport.FormFull}).Whole
	whole := shareable(t, report).Whole
	_, logs, _ := strings.Cut(whole, "## The logs")
	core, node, _ := strings.Cut(logs, "### node")
	pathTag, titleTag := tagOf(t, full, filePath), tagOf(t, full, title)
	if !regexp.MustCompile(`^path:/[0-9a-f]{6,}/[0-9a-f]{6,}/[0-9a-f]{6,}/[0-9a-f]{6,}\.mkv$`).MatchString(pathTag) {
		t.Errorf("the path's tag %q does not keep its depth and extension", pathTag)
	}
	for where, want := range map[string]string{
		"the core's path field": "path=" + pathTag, "the core's error": "open " + pathTag + ": no such",
		"the node's path field": "path=" + pathTag, "the node's words": "ended early at " + pathTag,
		"the core's title field": "itemTitle=" + titleTag, "the node's words of the title": "stream of " + titleTag,
	} {
		section := core
		if strings.Contains(where, "node") {
			section = node
		}
		if !strings.Contains(section, want) {
			t.Errorf("%s does not read %q:\n%s", where, want, section)
		}
	}
	if !strings.Contains(whole, "> "+titleTag+" stops") || !strings.Contains(whole, "| Path | "+pathTag+" |") {
		t.Errorf("the person's words or the playback's file do not carry the same tags:\n%s", whole)
	}
}

func TestRender_AnotherSaltGivesOtherTags(t *testing.T) {
	t.Parallel()
	first, second := fixture(), fixture()
	second.Salt = []byte("salt-of-another-report")
	one := render(t, first, problemreport.Options{Form: problemreport.FormFull}).Whole
	other := render(t, second, problemreport.Options{Form: problemreport.FormFull}).Whole
	for _, value := range []string{title, filePath, userName, address} {
		if tagOf(t, one, value) == tagOf(t, other, value) {
			t.Errorf("two reports give %q the same tag %s", value, tagOf(t, one, value))
		}
	}
	if again := render(t, fixture(), problemreport.Options{Form: problemreport.FormFull}).Whole; again != one {
		t.Error("one salt rendered twice gives two reports")
	}
}

var tagPattern = regexp.MustCompile(
	`\b(?:item|path|user|device|node|house|address|email|text):[0-9a-f/]+(?:\.[A-Za-z0-9]+)?`)

func TestRender_TheFullFormNamesEverythingAndMapsEachTag(t *testing.T) {
	t.Parallel()
	report := fixture()
	shared := shareable(t, report).Whole
	full := render(t, report, problemreport.Options{Form: problemreport.FormFull, IncludeOther: true}).Whole
	if !strings.Contains(full, "DO NOT SHARE PUBLICLY") || strings.Contains(shared, "DO NOT SHARE PUBLICLY") {
		t.Error("the do-not-share banner is not the full form's alone")
	}
	if !strings.Contains(shared, "Review before sharing") || !strings.Contains(shared, "best-effort") ||
		!strings.Contains(shared, "password") {
		t.Errorf("the shareable form does not say to review it, that it is best-effort, and not for "+
			"credentials:\n%s", shared)
	}
	for _, value := range []string{title, filePath, userName, deviceName, houseName, address, nodeName} {
		if !strings.Contains(full, value) {
			t.Errorf("the full form does not name %q", value)
		}
	}
	tags := tagPattern.FindAllString(shared, -1)
	if len(tags) < 8 {
		t.Fatalf("the shareable form has %d tags, want the fixture's many:\n%s", len(tags), shared)
	}
	for _, tag := range tags {
		if !regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta(tag) + ` \| [^|]+ \|$`).MatchString(full) {
			t.Errorf("the full form's table does not say what %s stands for", tag)
		}
	}
}

func TestRender_AnOtherLibrarysNamesStayTagsUnlessTheDownloadAsks(t *testing.T) {
	t.Parallel()
	report := fixture()
	withheld := render(t, report, problemreport.Options{Form: problemreport.FormFull}).Whole
	for _, value := range []string{otherTitle, otherRoot, otherPath} {
		if strings.Contains(withheld, value) {
			t.Errorf("the full form without other libraries shows %q:\n%s", value, withheld)
		}
	}
	if !strings.Contains(withheld, "left as their tags and out of the table") ||
		!strings.Contains(withheld, "title=item:") {
		t.Errorf("the full form does not say it withheld the other library's names, or lost their tags:\n%s", withheld)
	}
	if !strings.Contains(withheld, title) {
		t.Error("withholding the other library withheld a film's title too")
	}
	included := render(t, report, problemreport.Options{Form: problemreport.FormFull, IncludeOther: true}).Whole
	if tagOf(t, included, otherTitle) == "" || tagOf(t, included, otherPath) == "" ||
		!strings.Contains(included, "included, as this download asked") {
		t.Errorf("the full form with other libraries does not name them and say so:\n%s", included)
	}
}

func TestRender_AFieldsStringIsNamedUnlessItsKeyIsShape(t *testing.T) {
	t.Parallel()
	report := fixture()
	report.Components = []problemreport.Component{{Name: "core", Version: "v0.9.0", Lines: []slogx.Line{
		line(0, "something happened", str("nickname", "Plain Unknown Words"), num("attempts", 3),
			num("fileId", 31), str("body.state", "stalled"), str(slogx.EventKey, "playbackStarted"),
			str("requestId", "r-17"), slogx.Field{Key: "waited", Value: slog.DurationValue(1500 * time.Millisecond)},
			slogx.Field{Key: "full", Value: slog.BoolValue(true)}),
	}}}
	whole := shareable(t, report).Whole
	if strings.Contains(whole, "Plain Unknown Words") || !strings.Contains(whole, "nickname=text:") {
		t.Errorf("a string under a key not known as shape is not named:\n%s", whole)
	}
	for _, kept := range []string{
		"attempts=3", "fileId=31", "body.state=stalled", "logEvent=playbackStarted",
		"requestId=r-17", "waited=1.5s", "full=true",
	} {
		if !strings.Contains(whole, kept) {
			t.Errorf("the shareable form does not keep %s:\n%s", kept, whole)
		}
	}
}

const canary = "CANARY-7f3a9c-secret-value"

func TestRender_NoSecretReachesEitherForm(t *testing.T) {
	t.Parallel()
	// The fields as the recent log keeps them: resolved, so a secret has given its "[redacted]".
	resolved := func(key string, value any) slogx.Field {
		return slogx.Field{Key: key, Value: slog.AnyValue(value).Resolve()}
	}
	report := fixture()
	report.Components = []problemreport.Component{{Name: "core", Version: "v0.9.0", Lines: []slogx.Line{
		line(0, "signed in to a provider", resolved("token", secrets.NewValue(canary)),
			resolved("err", errors.New("Get \"https://api.example.org/v3/search?api_key="+canary+
				"#"+canary+"\": timeout")),
			resolved("callback", "https://user:"+canary+"@example.org/cb?code="+canary)),
	}}}
	report.Device = append(report.Device,
		problemreport.Fact{Name: "Page", Value: "https://orpheum.example/x?t=" + canary})
	report.Words = "it broke at https://orpheum.example/watch?token=" + canary
	for _, options := range []problemreport.Options{
		{Form: problemreport.FormShareable}, {Form: problemreport.FormFull, IncludeOther: true},
	} {
		rendered := render(t, report, options)
		for _, output := range []string{rendered.Summary, rendered.Whole} {
			if strings.Contains(output, canary) || strings.Contains(output, fmt.Sprintf("%x", canary)) {
				t.Errorf("the %s form shows the secret:\n%s", options.Form, output)
			}
			if !strings.Contains(output, "token=[redacted]") || !strings.Contains(output, "?[dropped]") {
				t.Errorf("the %s form does not show the secret redacted and the query dropped:\n%s",
					options.Form, output)
			}
		}
	}
}

func TestRender_TheSummaryFitsAnIssueAndSaysWhatItLeavesOut(t *testing.T) {
	t.Parallel()
	report := fixture()
	var lines []slogx.Line
	for at := range 5000 {
		lines = append(lines, line(time.Duration(at)*time.Second, "a segment was served", num("segment", int64(at)),
			str("path", filePath), str("err", "a long enough error to make the line a row of some size")))
	}
	report.Components[0].Lines = lines
	report.Components[1] = problemreport.Component{
		Name: nodeName, Node: true, Version: "v0.9.0", Absent: "no answer within 5 s, last seen 4 h ago",
	}
	rendered := shareable(t, report)
	if len(rendered.Summary) > problemreport.SummaryLimit {
		t.Fatalf("the summary is %d bytes, over %d", len(rendered.Summary), problemreport.SummaryLimit)
	}
	shown := strings.Count(rendered.Summary, "a segment was served")
	if shown == 0 || shown == 5000 {
		t.Fatalf("the summary shows %d of 5000 lines, want some", shown)
	}
	if !strings.Contains(rendered.Summary, fmt.Sprintf("the %d older are left out of this summary", 5000-shown)) ||
		!strings.Contains(rendered.Summary, "segment=4999 ") || strings.Contains(rendered.Summary, "segment=0 ") {
		t.Errorf("the summary does not show the newest lines and say how many it left out:\n%.3000s", rendered.Summary)
	}
	if strings.Count(rendered.Whole, "a segment was served") != 5000 {
		t.Error("the whole report does not have every line")
	}
	for _, output := range []string{rendered.Summary, rendered.Whole} {
		if !strings.Contains(output, "### node node:") || !strings.Contains(output, ": absent") ||
			!strings.Contains(output, "no answer within 5 s, last seen 4 h ago") {
			t.Errorf("the absent node is not listed with its reason:\n%.3000s", output)
		}
	}
}

func TestRender_TheFormIsRequired(t *testing.T) {
	t.Parallel()
	for _, form := range []problemreport.Form{"", "safe", "unsafe"} {
		_, err := problemreport.Render(fixture(), problemreport.Options{Form: form})
		if !errors.Is(err, problemreport.ErrNoForm) {
			t.Errorf("form %q gave %v, want ErrNoForm", form, err)
		}
	}
}

func TestRender_ScrubsFreeTextByWholeWordsAndEachKindOfName(t *testing.T) {
	t.Parallel()
	report := fixture()
	report.Known = append(report.Known, problemreport.Known{Kind: problemreport.KindItem, Value: "Room"})
	report.Words = "Room will not play in the bathroom; room service; a Roomba. velvet static lower-cased. " +
		"From fd00::17 and 2001:db8:0:0:0:0:0:9 at 10:30:45; mail me at mirela@example.org. " +
		"open /srv/unknown dir/Kestrel Point.mkv: permission denied."
	whole := shareable(t, report).Whole
	for _, leaked := range []string{
		"Room will", "velvet static", "fd00::17", "2001:db8", "mirela@", "example.org",
		"unknown dir", "Kestrel",
	} {
		if strings.Contains(whole, leaked) {
			t.Errorf("the person's words still show %q:\n%s", leaked, whole)
		}
	}
	for _, kept := range []string{
		"bathroom", "room service", "a Roomba", "at 10:30:45", "lower-cased", ": permission denied",
		"email:", "address:",
	} {
		if !strings.Contains(whole, kept) {
			t.Errorf("the person's words lost %q:\n%s", kept, whole)
		}
	}
	if !regexp.MustCompile(`open path:/[0-9a-f]{6,}/[0-9a-f]{6,}/[0-9a-f]{6,}\.mkv: permission`).MatchString(whole) {
		t.Errorf("an unknown path with spaces is not named whole, depth and extension kept:\n%s", whole)
	}
}

func TestRender_AWordCannotEndTheReportsHTML(t *testing.T) {
	t.Parallel()
	report := fixture()
	report.Words = "</details><script>x</script> | not a cell"
	whole := shareable(t, report).Whole
	if strings.Count(whole, "</details>") != 1 || strings.Contains(whole, "<script>") {
		t.Errorf("the person's words are read as HTML:\n%s", whole)
	}
}

// A report made on the command line says so, by the command that made it, where one made in the
// app names its person, device and page, and calls its facts the machine's (D524 §5).
func TestRender_AReportFromTheCommandLineSaysTheCommandThatMadeIt(t *testing.T) {
	t.Parallel()
	report := fixture()
	report.Reporter, report.Page, report.Playback = problemreport.Reporter{}, "", nil
	report.Command = "orpheum bugreport safe"
	whole := shareable(t, report).Whole
	for _, said := range []string{"Made on the command line, by `orpheum bugreport safe`", "## The machine"} {
		if !strings.Contains(whole, said) {
			t.Errorf("the report does not say %q:\n%s", said, whole)
		}
	}
	for _, unsaid := range []string{"Made by", "## The device"} {
		if strings.Contains(whole, unsaid) {
			t.Errorf("the report made on the command line says %q:\n%s", unsaid, whole)
		}
	}
}
