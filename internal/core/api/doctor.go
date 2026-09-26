// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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
	return output, nil
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
