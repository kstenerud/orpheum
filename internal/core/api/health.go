// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// HealthOutput is /health's answer.
type HealthOutput struct {
	Status int `json:"-"`
	Body   struct {
		Status  Status   `enum:"starting,migrating,ready,degraded" json:"status"`
		Detail  string   `json:"detail,omitempty"`
		Reasons []string `json:"reasons"`
	}
}

// tagRunningIt groups what an admin runs the server with: health, doctor, the System page.
const tagRunningIt = "Running it"

func (server *Server) registerHealth() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getHealth",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Whether the server is starting, migrating, ready or degraded",
		Description: "Unauthenticated. Starting and migrating answer 200 so a container manager does not kill a " +
			"busy server; degraded answers 503 with its reasons.",
		Tags: []string{tagRunningIt},
	}, server.getHealth)
}

// getHealth answers GET /health: whether the server is starting, migrating, ready or degraded.
func (server *Server) getHealth(context.Context, *struct{}) (*HealthOutput, error) {
	status, detail, reasons := server.currentHealth()
	output := &HealthOutput{Status: http.StatusOK}
	if status == StatusDegraded {
		output.Status = http.StatusServiceUnavailable
	}
	output.Body.Status, output.Body.Detail, output.Body.Reasons = status, detail, reasons
	return output, nil
}
