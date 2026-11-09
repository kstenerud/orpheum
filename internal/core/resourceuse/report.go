// SPDX-License-Identifier: AGPL-3.0-or-later

package resourceuse

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// HoursShown is how far back the hours go: a day, so a day's evening plays sit beside its night's
// scans.
const HoursShown = 24 * time.Hour

// Process names whose figures they are: the core, or a node by its name.
type Process struct {
	// IsNode is a node's; NodeName names it.
	IsNode   bool
	NodeName string
}

// Summary is one process's figures over every row kept.
type Summary struct {
	Process
	// Since is its first row kept.
	Since time.Time
	// CPUPercent is its CPU time over its time, as a percentage of one core, and BusiestCPUPercent
	// its busiest minute's; HasCPU is false when no row has its CPU time yet.
	CPUPercent, BusiestCPUPercent float64
	HasCPU                        bool
	// HeldBytes, CacheBytes and PeakBytes are the most of each a row said; PeakBytes zero when the
	// kernel never said.
	HeldBytes, CacheBytes, PeakBytes int64
}

// Hour is one process's figures over one hour.
type Hour struct {
	Process
	// Start is the hour's start; the hours are whole hours of Unix time.
	Start time.Time
	// CPUPercent is its CPU time over the hour's measured time, as a percentage of one core, and
	// BusiestCPUPercent its busiest minute's; HasCPU false when no row has its CPU time.
	CPUPercent, BusiestCPUPercent float64
	HasCPU                        bool
	// HeldBytes, CacheBytes and PeakBytes are the most of each in the hour.
	HeldBytes, CacheBytes, PeakBytes int64
	// Plays is the most playbacks playing at once, on the core's hours.
	Plays int64
	// Running is, on a node's hours, the most of each kind of job running at once.
	Running map[string]int
}

// Report is what the System page shows: each process's summary, and its hours of the last day,
// newest first.
type Report struct {
	Summaries []Summary
	Hours     []Hour
}

// Report reads every process's summary and its hours since HoursShown ago.
func (recorder *Recorder) Report(ctx context.Context) (Report, error) {
	queries := recorder.database.Read()
	summaries, err := queries.SummarizeResourceUse(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("summarize what the core and nodes used: %w", err)
	}
	rows, err := queries.ListResourceUseSince(ctx, recorder.clock.Now().Add(-HoursShown).UnixMilli())
	if err != nil {
		return Report{}, fmt.Errorf("list what the core and nodes used: %w", err)
	}
	report := Report{Summaries: make([]Summary, 0, len(summaries)), Hours: hoursOf(rows)}
	for _, row := range summaries {
		summary := Summary{
			Process: processOf(row.Process, row.NodeName), Since: time.UnixMilli(row.FirstAt),
			HeldBytes: row.HeldBytes, CacheBytes: row.CacheBytes, PeakBytes: row.PeakBytes,
			HasCPU: row.ElapsedMs > 0,
		}
		if summary.HasCPU {
			summary.CPUPercent = percentOfOneCore(row.CpuMicros, row.ElapsedMs)
			summary.BusiestCPUPercent = float64(row.BusiestMicrosPerSecond) / 10_000
		}
		report.Summaries = append(report.Summaries, summary)
	}
	return report, nil
}

// hourKey is one process's hour.
type hourKey struct {
	process, nodeName string
	start             int64
}

// hourSums is what an hour's rows add up to.
type hourSums struct {
	hour                 Hour
	cpuMicros, elapsedMs int64
}

// hoursOf groups rows into each process's hours, newest first, the core's before the nodes'.
func hoursOf(rows []cachegen.ResourceUse) []Hour {
	sums := map[hourKey]*hourSums{}
	for _, row := range rows {
		start := row.At - row.At%time.Hour.Milliseconds()
		key := hourKey{process: row.Process, nodeName: row.NodeName, start: start}
		sum := sums[key]
		if sum == nil {
			sum = &hourSums{hour: Hour{
				Process: processOf(row.Process, row.NodeName), Start: time.UnixMilli(start), Running: map[string]int{},
			}}
			sums[key] = sum
		}
		sum.add(row)
	}
	hours := make([]Hour, 0, len(sums))
	for _, sum := range sums {
		if sum.elapsedMs > 0 {
			sum.hour.HasCPU = true
			sum.hour.CPUPercent = percentOfOneCore(sum.cpuMicros, sum.elapsedMs)
		}
		hours = append(hours, sum.hour)
	}
	slices.SortFunc(hours, func(a, b Hour) int {
		return cmp.Or(
			compareBool(a.IsNode, b.IsNode), cmp.Compare(a.NodeName, b.NodeName),
			b.Start.Compare(a.Start),
		)
	})
	return hours
}

// add adds one row to its hour.
func (sum *hourSums) add(row cachegen.ResourceUse) {
	hour := &sum.hour
	if row.CpuMicros != nil && row.ElapsedMs != nil {
		sum.cpuMicros += *row.CpuMicros
		sum.elapsedMs += *row.ElapsedMs
		hour.BusiestCPUPercent = max(hour.BusiestCPUPercent, percentOfOneCore(*row.CpuMicros, *row.ElapsedMs))
	}
	hour.HeldBytes = max(hour.HeldBytes, row.HeldBytes)
	hour.CacheBytes = max(hour.CacheBytes, row.CacheBytes)
	if row.PeakBytes != nil {
		hour.PeakBytes = max(hour.PeakBytes, *row.PeakBytes)
	}
	hour.Plays = max(hour.Plays, row.Plays)
	var running map[string]int
	if strictjson.Unmarshal([]byte(row.RunningJSON), &running) == nil {
		for kind, count := range running {
			hour.Running[kind] = max(hour.Running[kind], count)
		}
	}
}

// percentOfOneCore is CPU time over elapsed time as a percentage of one core.
func percentOfOneCore(cpuMicros, elapsedMs int64) float64 {
	return float64(cpuMicros) / float64(elapsedMs) / 10
}

// processOf is a row's process.
func processOf(process, nodeName string) Process {
	return Process{IsNode: process == processNode, NodeName: nodeName}
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
