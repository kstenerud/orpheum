// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/atomics"
	"github.com/kstenerud/orpheum/internal/platform/errs"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// reportsDirectoryName is the folder of the backups directory that holds the reports an admin asked
// for (D306 §3): beside the backups, where an admin takes files from the server, and never pruned
// with them.
const reportsDirectoryName = "reports"

// reportNamePattern is a report's name, ours and never anyone's text.
var reportNamePattern = regexp.MustCompile(`^[a-z-]+-\d+-\d{8}T\d{6}Z\.md$`)

// ErrBadReportName is a report's name that is not one this server makes.
var ErrBadReportName = errors.New("not a report's name")

// reportMostBytes bounds a report read back: a report lists a title's or a series' files.
const reportMostBytes = 4 << 20

// ReportFile is one report in the reports directory.
type ReportFile struct {
	Name      string
	SizeBytes int64
	WrittenAt time.Time
}

// ReportPath is where a report is, or would be, for the admin to find on the server's disk.
func (directories *Directories) ReportPath(name string) string {
	return filepath.Join(directories.backupsDirectory, reportsDirectoryName, name)
}

// WriteReport keeps a report, atomically, mode 0600, and returns where it is.
func (directories *Directories) WriteReport(name string, contents []byte) (path string, err error) {
	reports, err := directories.openReports(true)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the reports directory", reports.Close())) }()
	if !reportNamePattern.MatchString(name) {
		return "", fmt.Errorf("%q: %w", name, ErrBadReportName)
	}
	if err = atomics.WriteFile(reports, name, contents, fileMode); err != nil {
		return "", fmt.Errorf("write report %s: %w", name, err)
	}
	return directories.ReportPath(name), nil
}

// ListReports lists the reports, newest first; none before the first is written.
func (directories *Directories) ListReports() ([]ReportFile, error) {
	entries, err := os.ReadDir(filepath.Join(directories.backupsDirectory, reportsDirectoryName))
	if errors.Is(err, os.ErrNotExist) {
		return []ReportFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the reports: %w", err)
	}
	files := []ReportFile{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !reportNamePattern.MatchString(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), infoErr)
		}
		files = append(files, ReportFile{Name: entry.Name(), SizeBytes: info.Size(), WrittenAt: info.ModTime()})
	}
	slices.SortFunc(files, func(first, second ReportFile) int { return second.WrittenAt.Compare(first.WrittenAt) })
	return files, nil
}

// ReadReport reads a report back; ErrNotFound when there is none of that name.
func (directories *Directories) ReadReport(name string) (contents []byte, err error) {
	if !reportNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%q: %w", name, ErrBadReportName)
	}
	reports, err := directories.openReports(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("report %s: %w", name, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	defer readonly.Close(reports)
	file, err := reports.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("report %s: %w", name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("open report %s: %w", name, err)
	}
	defer readonly.Close(file)
	contents, err = io.ReadAll(io.LimitReader(file, reportMostBytes))
	if err != nil {
		return nil, fmt.Errorf("read report %s: %w", name, err)
	}
	return contents, nil
}

// openReports opens the reports directory under the backups directory, creating it when asked,
// through os.Root, so no name can reach outside it.
func (directories *Directories) openReports(shouldCreate bool) (*os.Root, error) {
	backups, err := os.OpenRoot(directories.backupsDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the backups directory: %w", err)
	}
	defer readonly.Close(backups)
	if shouldCreate {
		if err = backups.MkdirAll(reportsDirectoryName, directoryMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", reportsDirectoryName, err)
		}
	}
	reports, err := backups.OpenRoot(reportsDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", reportsDirectoryName, err)
	}
	return reports, nil
}
