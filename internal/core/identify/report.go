// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ReportStore keeps reports on the server's disk: instancedir.Directories.
type ReportStore interface {
	WriteReport(name string, contents []byte) (string, error)
	ListReports() ([]instancedir.ReportFile, error)
	ReadReport(name string) ([]byte, error)
}

// FilingReports are the reports an admin asks for of a work filed wrongly (D306 §3): a Markdown file
// on the server's disk, which says how each of its files was read and filed, for the admin to read,
// edit where it names what is theirs, and post as an issue. Nothing leaves the server by itself.
type FilingReports struct {
	database *store.Database
	reports  ReportStore
	version  string
	clock    clock.Clock
}

// NewFilingReports returns the reports, kept in reports, naming this build's version.
func NewFilingReports(database *store.Database, reports ReportStore, version string, now clock.Clock) *FilingReports {
	return &FilingReports{database: database, reports: reports, version: version, clock: now}
}

// ErrNothingToReport is a work with no file to report on: none, or none live.
var ErrNothingToReport = errors.New("the work has no file to report on")

// FilingReport is one report as it was written: its name, where it is, and its text.
type FilingReport struct {
	Name     string
	Path     string
	Markdown string
}

// filesReported bounds how many files a report lists: a series of many seasons.
const filesReported = 2000

// Report writes a report of how a film, an episode or a series was filed, and returns it.
func (reports *FilingReports) Report(ctx context.Context, itemID int64) (FilingReport, error) {
	queries := reports.database.Read()
	files, err := queries.ListFilesOfWork(ctx, sqlcgen.ListFilesOfWorkParams{ItemID: itemID, MaxRows: filesReported})
	if err != nil {
		return FilingReport{}, fmt.Errorf("list the files of item %d: %w", itemID, err)
	}
	if len(files) == 0 {
		return FilingReport{}, fmt.Errorf("item %d: %w", itemID, ErrNothingToReport)
	}
	libraryID, err := queries.GetFilingLibraryOfItem(ctx, itemID)
	if err != nil || libraryID == nil {
		return FilingReport{}, fmt.Errorf("find item %d's library: %w", itemID, errors.Join(err, ErrNothingToReport))
	}
	work, err := reports.workOf(ctx, queries, itemID, *libraryID)
	if err != nil {
		return FilingReport{}, err
	}
	now := reports.clock.Now().UTC()
	text := work.markdown(reports.version, now, files)
	name := "filed-wrongly-" + strconv.FormatInt(itemID, 10) + "-" + now.Format("20060102T150405Z") + ".md"
	path, err := reports.reports.WriteReport(name, []byte(text))
	if err != nil {
		return FilingReport{}, fmt.Errorf("keep the report of item %d: %w", itemID, err)
	}
	return FilingReport{Name: name, Path: path, Markdown: text}, nil
}

// List lists the reports, newest first.
func (reports *FilingReports) List() ([]instancedir.ReportFile, error) {
	listed, err := reports.reports.ListReports()
	if err != nil {
		return nil, fmt.Errorf("list the reports: %w", err)
	}
	return listed, nil
}

// Read is one report's text; instancedir.ErrNotFound when there is none of that name.
func (reports *FilingReports) Read(name string) (string, error) {
	contents, err := reports.reports.ReadReport(name)
	if err != nil {
		return "", fmt.Errorf("read report %s: %w", name, err)
	}
	return string(contents), nil
}

// reportedWork is what a report says of the work it is about: its name and year, how it was
// identified, the ids it was matched to, and its library's kind and roots.
type reportedWork struct {
	name, kind, identification string
	year                       int64
	isGuessed                  bool
	providerIDs                []string
	libraryKind                string
	roots                      []string
}

func (reports *FilingReports) workOf(
	ctx context.Context, queries *sqlcgen.Queries, itemID, libraryID int64,
) (reportedWork, error) {
	work := reportedWork{}
	if series, err := store.OrNotFound(queries.GetSeriesDetail(ctx, itemID)); err == nil {
		work.kind, work.name, work.year = "series", ptr.ValueOrZero(series.Name), ptr.ValueOrZero(series.Year)
		work.identification, work.isGuessed = ptr.ValueOrZero(series.IdentificationState), series.IsGuessed
	} else if errors.Is(err, store.ErrNotFound) {
		title, titleErr := queries.GetTitleDetail(ctx, itemID)
		if titleErr != nil {
			return reportedWork{}, fmt.Errorf("read title %d: %w", itemID, titleErr)
		}
		work.kind, work.name, work.year = title.Kind, ptr.ValueOrZero(title.Name), ptr.ValueOrZero(title.Year)
		work.identification, work.isGuessed = ptr.ValueOrZero(title.IdentificationState), title.IsGuessed
	} else {
		return reportedWork{}, fmt.Errorf("read series %d: %w", itemID, err)
	}
	ids, err := queries.ListItemProviderIDs(ctx, itemID)
	if err != nil {
		return reportedWork{}, fmt.Errorf("list item %d's ids: %w", itemID, err)
	}
	for _, id := range ids {
		work.providerIDs = append(work.providerIDs, id.Provider+":"+id.ProviderID)
	}
	held, err := queries.GetLibraryForIdentify(ctx, libraryID)
	if err != nil {
		return reportedWork{}, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	work.libraryKind = held.Kind
	roots, err := queries.ListLibraryRoots(ctx, libraryID)
	if err != nil {
		return reportedWork{}, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	for _, root := range roots {
		work.roots = append(work.roots, root.RootPath)
	}
	return work, nil
}

// markdown is the report's text. Each file is named by its path below its library's folder, which
// is what the parser reads; the folder itself, the server's own, is left out.
func (work reportedWork) markdown(version string, at time.Time, files []sqlcgen.ListFilesOfWorkRow) string {
	var text strings.Builder
	fmt.Fprintf(&text, "# Filed wrongly: %s\n\n", markdownText(work.name))
	fmt.Fprintf(&text, "- Orpheum %s, %s\n", version, at.Format(time.RFC3339))
	fmt.Fprintf(&text, "- A %s library\n", work.libraryKind)
	matched := "no provider id"
	if len(work.providerIDs) > 0 {
		matched = strings.Join(work.providerIDs, ", ")
	}
	guess := ""
	if work.isGuessed {
		guess = ", a guess no one confirmed"
	}
	fmt.Fprintf(&text, "- Filed as the %s %q%s, %s (%s%s)\n\n", work.kind, work.name, yearText(work.year),
		work.identification, matched, guess)
	text.WriteString("Edit anything here you would rather not post, such as names of your own, before posting " +
		"it as an issue.\n\n## Its files\n\n" +
		"| Path in the library | Read from the path as | Filed as |\n| --- | --- | --- |\n")
	for _, file := range files {
		relative := work.relative(file.Path)
		fmt.Fprintf(&text, "| `%s` | %s | %s |\n", strings.ReplaceAll(relative, "`", "'"),
			markdownText(work.readAs(relative)), markdownText(filedAs(file)))
	}
	if len(files) == filesReported {
		fmt.Fprintf(&text, "\nOnly the first %d files are listed.\n", filesReported)
	}
	text.WriteString("\n## What it should have been\n\n(Say here what you expected.)\n")
	return text.String()
}

// relative is a path below the library's folder that holds it, or the path as it is under none.
func (work reportedWork) relative(path string) string {
	for _, root := range work.roots {
		if rest, isUnder := strings.CutPrefix(path, strings.TrimSuffix(root, "/")+"/"); isUnder {
			return rest
		}
	}
	return path
}

// readAs is what the parser reads from a path in the work's library, as the pass would.
func (work reportedWork) readAs(relative string) string {
	parsed := ParsePath(relative)
	if work.libraryKind == library.LibraryKindSeries {
		parsed = ParseEpisodePath(relative)
	}
	if parsed.Kind != library.TitleKindEpisode {
		return fmt.Sprintf("%s %q%s", parsed.Kind, parsed.Name, yearText(int64(parsed.Year)))
	}
	if parsed.IsUnnumbered {
		return fmt.Sprintf("an episode of %q with no number, named %q", parsed.Name, parsed.EpisodeName)
	}
	place := fmt.Sprintf("S%02dE%02d", parsed.Season, parsed.Episode)
	if parsed.EpisodeEnd > parsed.Episode {
		place += fmt.Sprintf("-E%02d", parsed.EpisodeEnd)
	}
	return fmt.Sprintf("an episode of %q, %s", parsed.Name, place)
}

// filedAs is how a file was filed: its filing and why, and the title it is in.
func filedAs(file sqlcgen.ListFilesOfWorkRow) string {
	filed := file.Filing
	if reason := ptr.ValueOrZero(file.FilingReason); reason != "" {
		filed += " (" + reason + ")"
	}
	title := fmt.Sprintf("%s %q%s", file.TitleKind, file.TitleName, yearText(ptr.ValueOrZero(file.Year)))
	if file.TitleKind == library.TitleKindEpisode {
		if file.NumberInGroup == nil {
			title += fmt.Sprintf(", season %d, no number", ptr.ValueOrZero(file.GroupNumber))
		} else {
			title += fmt.Sprintf(", S%02dE%02d", ptr.ValueOrZero(file.GroupNumber), *file.NumberInGroup)
		}
	}
	return filed + ": " + title + ", " + ptr.ValueOrZero(file.IdentificationState)
}

// yearText is " (1995)", or nothing for no year.
func yearText(year int64) string {
	if year == 0 {
		return ""
	}
	return " (" + strconv.FormatInt(year, 10) + ")"
}

// markdownText is text that cannot break a table's row: no pipe, no line break.
func markdownText(text string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(text)
}
