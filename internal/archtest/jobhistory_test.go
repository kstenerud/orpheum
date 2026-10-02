// SPDX-License-Identifier: GPL-3.0-or-later

package archtest

import (
	"fmt"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Where the job queue lives, and cache.db's queries of its job table.
const (
	jobqueuePath = platformPath + "/jobqueue"
	jobsPath     = corePath + "/jobs"
	jobQueryFile = "job.sql"
)

// jobHistoryReaders are the functions that read a program's job rows, each with the packages that
// may name it: the storage over cache.db's job table (internal/store), the queue that drives it
// (internal/platform/jobqueue), the job view that lists them (internal/core/jobs) and the API
// that serves that view (internal/core/api). It is given the names of cache.db's queries of the
// job table.
func jobHistoryReaders(jobQueries []string) map[string][]string {
	readers := map[string][]string{
		jobqueuePath + ".Queue.List":       {jobqueuePath, jobsPath},
		jobqueuePath + ".Storage.List":     {jobqueuePath},
		jobqueuePath + ".Listing.List":     {jobqueuePath},
		storePath + ".JobStorage.List":     {storePath},
		jobsPath + ".Queue.List":           {jobsPath, apiPath},
		jobqueuePath + ".Storage.ListOpen": {jobqueuePath},
		jobqueuePath + ".Listing.ListOpen": {jobqueuePath},
		storePath + ".JobStorage.ListOpen": {storePath},
	}
	for _, query := range jobQueries {
		readers[cachegenPath+".Queries."+query] = []string{storePath}
	}
	return readers
}

// functionKey names a function as jobHistoryReaders does: its package's path, its receiver's type
// when it is a method, and its name.
func functionKey(function *types.Func) string {
	key := function.Pkg().Path() + "."
	if receiver := function.Signature().Recv(); receiver != nil {
		receiverType := receiver.Type()
		if pointer, isPointer := receiverType.(*types.Pointer); isPointer {
			receiverType = pointer.Elem()
		}
		if named, isNamed := receiverType.(*types.Named); isNamed {
			key += named.Obj().Name() + "."
		}
	}
	return key + function.Name()
}

// findJobHistoryReaders returns every package that names a function reading job rows that it may
// not: what became of a job is never a fact a program derives work from (D231 §3), so only the
// queue and the job view read them. It is given the functions each package names, by
// functionKey.
func findJobHistoryReaders(functionsByPackage, readers map[string][]string) []violation {
	var violations []violation
	for _, importPath := range slices.Sorted(maps.Keys(functionsByPackage)) {
		for _, function := range slices.Sorted(slices.Values(functionsByPackage[importPath])) {
			allowed, isReader := readers[function]
			if isReader && !slices.Contains(allowed, importPath) {
				violations = append(violations, violation{where: importPath, detail: fmt.Sprintf(
					"reads job rows through %s; what became of a job is no fact to derive work from, so only "+
						"%s read them (D231 §3)", function, strings.Join(allowed, " and "))})
			}
		}
	}
	return violations
}

// TestArch_NothingButTheQueueAndTheJobViewReadsJobHistory checks that no package reads cache.db's
// job rows but the storage, the queue and the job view (D231 §3): work is derived from persistent.db
// by sweeps, never from whether a job was queued, failed or ran, which a discarded cache.db loses.
func TestArch_NothingButTheQueueAndTheJobViewReadsJobHistory(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	var storeDirectory string
	for _, listed := range packages {
		if listed.ImportPath == storePath {
			storeDirectory = listed.Dir
		}
	}
	if storeDirectory == "" {
		t.Fatalf("go list did not find %s", storePath)
	}
	var jobQueries []string
	for _, file := range readQueryFiles(t, filepath.Join(storeDirectory, "cache", "queries")) {
		if filepath.Base(file.path) != jobQueryFile {
			continue
		}
		for _, header := range queryHeader.FindAllStringSubmatch(file.contents, -1) {
			jobQueries = append(jobQueries, header[1])
		}
	}
	if len(jobQueries) == 0 {
		t.Fatalf("found no query in cache.db's %s; the rule would pass vacuously", jobQueryFile)
	}
	functionsByPackage := map[string][]string{}
	for importPath, functions := range loadFunctionsUsed(t, packages) {
		for _, function := range functions {
			functionsByPackage[importPath] = append(functionsByPackage[importPath], functionKey(function))
		}
	}
	if !slices.Contains(functionsByPackage[apiPath], jobsPath+".Queue.List") {
		t.Fatal("the API names no jobs.Queue.List; the rule no longer sees the job view, and would pass vacuously")
	}
	reportViolations(t, findJobHistoryReaders(functionsByPackage, jobHistoryReaders(jobQueries)))
}

func TestFindJobHistoryReaders_CatchesAPackageThatReadsJobRowsAndPassesTheQueueAndTheView(t *testing.T) {
	t.Parallel()
	readers := jobHistoryReaders([]string{"ListJobs", "WasJobEverQueued"})
	clean := map[string][]string{
		storePath: {cachegenPath + ".Queries.ListJobs"},
		jobsPath:  {jobqueuePath + ".Queue.List"},
		apiPath:   {jobsPath + ".Queue.List"},
		cmdPath:   {jobsPath + ".Queue.Enqueue"},
	}
	if got := findJobHistoryReaders(clean, readers); len(got) != 0 {
		t.Errorf("the storage, the queue and the view reading job rows: got %v, want none", got)
	}
	broken := map[string][]string{
		cmdPath:                {jobsPath + ".Queue.List"},
		corePath + "/identify": {cachegenPath + ".Queries.WasJobEverQueued"},
	}
	got := findJobHistoryReaders(broken, readers)
	if len(got) != 2 || got[0].where != cmdPath || got[1].where != corePath+"/identify" {
		t.Errorf("a first match gated on job history, and a composition root reading the view: got %v, want both", got)
	}
}
