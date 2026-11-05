// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/resourceuse"
)

// ResourceUseProcess names whose figures they are: the core, or a node by its name.
type ResourceUseProcess struct {
	IsNode   bool   `json:"isNode"`
	NodeName string `doc:"The node's name; empty for the core" json:"nodeName"`
}

// ResourceUseSummaryBody is one process's figures over every minute kept.
type ResourceUseSummaryBody struct {
	ResourceUseProcess
	Since int64 `doc:"Unix milliseconds of its first minute kept" json:"since"`
	// CPUPercent and BusiestCPUPercent are absent until a minute has its CPU time.
	CPUPercent        *float64 `doc:"Its CPU time over its time, in percent of one core" json:"cpuPercent,omitempty"`
	BusiestCPUPercent *float64 `doc:"Its busiest minute's, in percent of one core" json:"busiestCpuPercent,omitempty"`
	HeldBytes         int64    `doc:"The most memory it held of its own" json:"heldBytes"`
	CacheBytes        int64    `doc:"The most file cache charged to it" json:"cacheBytes"`
	PeakBytes         int64    `doc:"The most the kernel saw it hold; 0 when it never said" json:"peakBytes"`
}

// ResourceUseHourBody is one process's figures over one hour, beside what it was doing.
type ResourceUseHourBody struct {
	ResourceUseProcess
	Start int64 `doc:"Unix milliseconds of the hour's start" json:"start"`
	// CPUPercent and BusiestCPUPercent are absent when no minute of the hour has its CPU time.
	CPUPercent        *float64 `doc:"Its CPU time over the hour, in percent of one core" json:"cpuPercent,omitempty"`
	BusiestCPUPercent *float64 `doc:"Its busiest minute's, in percent of one core" json:"busiestCpuPercent,omitempty"`

	HeldBytes  int64          `json:"heldBytes"`
	CacheBytes int64          `json:"cacheBytes"`
	PeakBytes  int64          `json:"peakBytes"`
	Plays      int64          `doc:"The core's: the most playbacks playing at once" json:"plays"`
	Running    map[string]int `doc:"A node's: the most of each kind of job running at once" json:"running"`
}

// ResourceUseOutput is what the core and each node used of their machines.
type ResourceUseOutput struct {
	Body struct {
		Summaries []ResourceUseSummaryBody `json:"summaries"`
		Hours     []ResourceUseHourBody    `doc:"The last day's hours, the core's first, newest first" json:"hours"`
	}
}

func (server *Server) registerResourceUse() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getResourceUse", Method: http.MethodGet, Path: "/system/resource-use",
		Tags: []string{tagRunningIt},
		Summary: "What the core and each node used of their machines, each minute, beside the plays and " +
			"the work running",
		Extensions: callableBy(capabilityAdmin),
	}, server.getResourceUse)
}

func (server *Server) getResourceUse(ctx context.Context, input *AuthorizationHeader) (*ResourceUseOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report, err := dependencies.ResourceUse.Report(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "read what the core and nodes used", err)
	}
	output := &ResourceUseOutput{}
	output.Body.Summaries = make([]ResourceUseSummaryBody, 0, len(report.Summaries))
	for _, summary := range report.Summaries {
		body := ResourceUseSummaryBody{
			ResourceUseProcess: processBody(summary.Process), Since: summary.Since.UnixMilli(),
			HeldBytes: summary.HeldBytes, CacheBytes: summary.CacheBytes, PeakBytes: summary.PeakBytes,
		}
		if summary.HasCPU {
			body.CPUPercent, body.BusiestCPUPercent = &summary.CPUPercent, &summary.BusiestCPUPercent
		}
		output.Body.Summaries = append(output.Body.Summaries, body)
	}
	output.Body.Hours = make([]ResourceUseHourBody, 0, len(report.Hours))
	for _, hour := range report.Hours {
		body := ResourceUseHourBody{
			ResourceUseProcess: processBody(hour.Process), Start: hour.Start.UnixMilli(),
			HeldBytes: hour.HeldBytes, CacheBytes: hour.CacheBytes, PeakBytes: hour.PeakBytes,
			Plays: hour.Plays, Running: hour.Running,
		}
		if hour.HasCPU {
			body.CPUPercent, body.BusiestCPUPercent = &hour.CPUPercent, &hour.BusiestCPUPercent
		}
		output.Body.Hours = append(output.Body.Hours, body)
	}
	return output, nil
}

// processBody is a process as the API names it.
func processBody(process resourceuse.Process) ResourceUseProcess {
	return ResourceUseProcess{IsNode: process.IsNode, NodeName: process.NodeName}
}
