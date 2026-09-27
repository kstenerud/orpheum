// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The verdicts a finding gives.
const (
	verdictOK   = "ok"
	verdictWarn = "warn"
)

// FindingBody is one thing the running server checked, and for anything wrong, what to do.
type FindingBody struct {
	Check   string `json:"check"`
	Verdict string `enum:"ok,warn,fail" json:"verdict"`
	Detail  string `json:"detail"`
	Action  string `json:"action,omitempty"`
}

// DoctorOutput is what the running server finds wrong with itself (cut-a §5, Running it). The
// artwork cache is its first check; others join as their milestones build them.
type DoctorOutput struct {
	Body struct {
		Findings []FindingBody `json:"findings"`
	}
}

func (server *Server) registerDoctor() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getDoctor", Method: http.MethodGet, Path: "/doctor", Tags: []string{"Running it"},
		Summary: "What the running server finds wrong with itself, and what to do (admin)",
	}, server.doctor)
}

func (server *Server) doctor(ctx context.Context, input *AuthorizationHeader) (*DoctorOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &DoctorOutput{}
	output.Body.Findings = []FindingBody{artworkFinding(dependencies)}
	nodes, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list nodes", err)
	}
	for _, node := range nodes {
		if node.Connected {
			output.Body.Findings = append(output.Body.Findings, confinementFinding(node.Name,
				confinementOf(dependencies.NodeReports, node.Name)))
			if usage, known := dependencies.NodeReports.PreparedUsage(node.Name); known {
				output.Body.Findings = append(output.Body.Findings, preparedFinding(node.Name, usage))
			}
		}
	}
	return output, nil
}

// preparedFinding is what a node's prepared directory holds against its budget and its disk's
// limit (D217 §5): a warning when either is past, which eviction could not bring back.
func preparedFinding(node string, usage nodeproto.PreparedUsage) FindingBody {
	const gib = 1 << 30
	finding := FindingBody{
		Check: "prepared copies on node " + node, Verdict: verdictOK,
		Detail: fmt.Sprintf("%.1f GiB of a %d GiB budget; the disk is %d%% full, its limit %d%%",
			float64(usage.Bytes)/gib, usage.BudgetBytes/gib, usage.DiskPercent, usage.DiskLimitPercent),
	}
	if usage.DiskPercent > usage.DiskLimitPercent || usage.Bytes > usage.BudgetBytes {
		finding.Verdict = verdictWarn
		finding.Action = "free space on the disk that holds the prepared directory, or raise " +
			"node.prepared_budget_gib; new copies are refused while nothing can be evicted"
	}
	return finding
}

// confinementFinding is a node's FFmpeg confinement: a warning for each mechanism not in force,
// with what would put it there, because a user whose safety depends on it deserves to know
// (D112).
func confinementFinding(node string, confinement []ConfinementBody) FindingBody {
	finding := FindingBody{
		Check: "FFmpeg confinement on node " + node, Verdict: verdictOK,
		Detail: "every FFmpeg run is confined by Landlock and seccomp",
	}
	var missing, remedies []string
	for _, mechanism := range confinement {
		if !mechanism.IsInForce {
			missing = append(missing, mechanism.Mechanism+": "+mechanism.Reason)
			remedies = append(remedies, mechanism.Mechanism+": "+mechanism.Remedy)
		}
	}
	if len(confinement) == 0 {
		missing = []string{"the node reported no confinement"}
	}
	if len(missing) > 0 {
		finding.Verdict = verdictWarn
		finding.Detail = "FFmpeg runs on this node without all its confinement: " + strings.Join(missing, "; ")
		finding.Action = strings.Join(remedies, "; ")
	}
	return finding
}

// artworkFinding is the artwork cache against its ceiling: at high water the admin is told what
// the libraries need and the ceiling to set (D212 §7).
func artworkFinding(dependencies *Dependencies) FindingBody {
	pressure := dependencies.Artwork.Pressure()
	const gib = 1 << 30
	finding := FindingBody{
		Check: "artwork cache", Verdict: verdictOK,
		Detail: fmt.Sprintf("%.1f GiB of a %d GiB ceiling",
			float64(pressure.TotalBytes)/gib, pressure.CeilingBytes/gib),
	}
	if pressure.HighWater {
		finding.Verdict = verdictWarn
		finding.Detail = fmt.Sprintf("the libraries need %.1f GiB of artwork to be shown at once, more than the "+
			"%d GiB ceiling; artwork beyond it is fetched again when it is opened, which is slower",
			float64(pressure.RequiredBytes)/gib, pressure.CeilingBytes/gib)
		finding.Action = fmt.Sprintf("set artwork_cache_ceiling_gib to at least %d", pressure.RecommendedBytes/gib)
	}
	return finding
}
