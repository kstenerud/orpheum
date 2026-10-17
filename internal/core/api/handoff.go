// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// tagSetupAndAuth groups the operations that set up the server and sign people in, for the OpenAPI
// document.
const tagSetupAndAuth = "Setup and auth"

// HandOffOutput is a sign-in kept for the client origin to take, and the code it takes it by.
type HandOffOutput struct {
	Body struct {
		Code string `doc:"Taken once, within a minute, by POST /session/redeem at the client origin" json:"code"`
	}
}

// RedeemInput is a hand-off's code, taken at the client origin.
type RedeemInput struct {
	Body struct {
		Code string `json:"code" maxLength:"100" minLength:"1"`
	}
	ClientFacts
}

func (server *Server) registerHandOff() {
	tags := []string{tagSetupAndAuth}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "handOffSession", Method: http.MethodPost, Path: handOffPath, DefaultStatus: http.StatusCreated,
		Summary: "Sign in at the discovery's loader; returns a code the client origin takes the session by (D308)",
		Tags:    tags,
	}, server.handOffSession)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "redeemSession", Method: http.MethodPost, Path: "/session/redeem",
		DefaultStatus: http.StatusCreated,
		Summary:       "Take the session a sign-in at the loader kept, by its code, once", Tags: tags,
	}, server.redeemSession)
}

// handOffPath is the hand-off's path, which the loader's origin may call (isOpenToTheLoader).
const handOffPath = "/session/handoff"

// handOffSession signs a person in at the loader, where no session may be kept (D308 §3), and
// answers a code for the client origin instead of the session.
func (server *Server) handOffSession(ctx context.Context, input *SignInInput) (*HandOffOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	code, err := dependencies.Auth.HandOff(
		ctx, input.Body.Name, input.Body.Password, input.clientAddress, newDevice(input.Body.Device))
	if err != nil {
		return nil, server.authError(ctx, "sign in for a hand-off", err)
	}
	output := &HandOffOutput{}
	output.Body.Code = code
	return output, nil
}

// redeemSession answers the session a hand-off kept, as a sign-in would have. Whether it
// administers is the request's, as for every session (D297 §6).
func (server *Server) redeemSession(ctx context.Context, input *RedeemInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	issued, err := dependencies.Auth.TakeHandOff(ctx, input.Body.Code)
	if err != nil {
		return nil, server.authError(ctx, "take a hand-off", err)
	}
	return server.newSessionOutput(ctx, dependencies, issued, input.isOverTLS)
}

// handOffRefusal is the API's error for auth's refusal of a hand-off, or nil for another.
func handOffRefusal(err error) *Error {
	switch {
	case errors.Is(err, auth.ErrNoHandOff):
		return newError(http.StatusUnauthorized, CodeAuthRequired, auth.ErrNoHandOff.Error())
	case errors.Is(err, auth.ErrTooManyHandOffs):
		return newError(http.StatusTooManyRequests, CodeRateLimited, auth.ErrTooManyHandOffs.Error())
	}
	return nil
}
