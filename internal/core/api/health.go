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

func (server *Server) registerHealth() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getHealth",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Whether the server is starting, migrating, ready or degraded",
		Description: "Unauthenticated. Starting and migrating answer 200 so a container manager does not kill a " +
			"busy server; degraded answers 503 with its reasons.",
		Tags: []string{"Running it"},
	}, func(context.Context, *struct{}) (*HealthOutput, error) {
		status, detail, reasons := server.currentHealth()
		output := &HealthOutput{Status: http.StatusOK}
		if status == StatusDegraded {
			output.Status = http.StatusServiceUnavailable
		}
		output.Body.Status, output.Body.Detail, output.Body.Reasons = status, detail, reasons
		return output, nil
	})
}
