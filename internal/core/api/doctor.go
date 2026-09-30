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
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getDoctor", Method: http.MethodGet, Path: "/doctor", Tags: []string{tagRunningIt},
		Summary: "What the running server finds wrong with itself, and what to do (admin)",
	}, server.getDoctor)
}

func (server *Server) getDoctor(ctx context.Context, input *AuthorizationHeader) (*DoctorOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &DoctorOutput{}
	output.Body.Findings, err = server.collectFindings(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// collectFindings lists what the running server finds wrong with itself, for /doctor and the
// System page.
func (server *Server) collectFindings(ctx context.Context, dependencies *Dependencies) ([]FindingBody, error) {
	findings := []FindingBody{artworkFinding(dependencies)}
	listed, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list nodes", err)
	}
	for _, node := range listed {
		facts, found := dependencies.NodeReports.Facts(node.Name)
		if !found {
			continue
		}
		findings = append(findings, nodeBuildFinding(node.Name, facts.Hello.Build, dependencies.System.Build),
			ConfinementFinding("node "+node.Name, facts.Hello.Confinement),
			hardwareFinding(node.Name, facts.Hello.Hardware))
		if facts.Status != nil && facts.Status.PreparedUsage != nil {
			findings = append(findings, preparedFinding(node.Name, *facts.Status.PreparedUsage))
		}
	}
	return findings, nil
}

// preparedFinding is what a node's prepared directory holds against its budget and its disk's
// limit (D217 §5): a warning when either is past, which eviction could not bring back.
func preparedFinding(nodeName string, usage nodeproto.PreparedUsage) FindingBody {
	const gib = 1 << 30
	finding := FindingBody{
		Check: "prepared copies on node " + nodeName, Verdict: verdictOK,
		Detail: fmt.Sprintf("%.1f GiB of a %d GiB budget; the disk is %d%% full, its limit %d%%",
			float64(usage.UsedBytes)/gib, usage.BudgetBytes/gib, usage.DiskUsedPercent, usage.DiskUsedLimitPercent),
	}
	if usage.DiskUsedPercent > usage.DiskUsedLimitPercent || usage.UsedBytes > usage.BudgetBytes {
		finding.Verdict = verdictWarn
		finding.Action = "free space on the disk that holds the prepared directory, or raise " +
			"node.prepared_budget_gib; new copies are refused while nothing can be evicted"
	}
	return finding
}

// ConfinementFinding is FFmpeg's confinement where it runs, judged from the mechanisms a node
// reports, or would report, in its hello: a warning naming each mechanism not in force, with what
// would put it there, because a user whose safety depends on it deserves to know (D112). A report
// with no mechanism in it is a warning too. It is the one judgement of confinement: /doctor and
// the System page use it for each node, and the doctor command for this machine. The place,
// where, reads "node devnas" or "this machine".
func ConfinementFinding(where string, confinement []nodeproto.Confinement) FindingBody {
	finding := FindingBody{
		Check: "FFmpeg confinement on " + where, Verdict: verdictOK,
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
		missing = []string{"no confinement was reported"}
	}
	if len(missing) > 0 {
		finding.Verdict = verdictWarn
		finding.Detail = "FFmpeg runs without all its confinement: " + strings.Join(missing, "; ")
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
	if pressure.IsHighWater {
		finding.Verdict = verdictWarn
		finding.Detail = fmt.Sprintf("the libraries need %.1f GiB of artwork to be shown at once, more than the "+
			"%d GiB ceiling; artwork beyond it is fetched again when it is opened, which is slower",
			float64(pressure.RequiredBytes)/gib, pressure.CeilingBytes/gib)
		finding.Action = fmt.Sprintf("set artwork_cache_ceiling_gib to at least %d", pressure.RecommendedBytes/gib)
	}
	return finding
}
