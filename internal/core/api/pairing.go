// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// PairingRequestInput is a device not signed in asking for a code to show (D288 §1): with the secret
// of the pairing it has, for the same code again, its platform, and the device it was before.
type PairingRequestInput struct {
	Body struct {
		Secret              string `doc:"The pairing this device has, for the same code again" json:"secret,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
		Platform            string `json:"platform" maxLength:"40" minLength:"1"`
		PreviousDeviceID    int64  `doc:"The device this client was signed in as before (D221)" json:"previousDeviceId,omitempty" required:"false"`                                                //nolint:lll // one struct tag
		PreviousDeviceProof string `doc:"A session token that device was issued, live or not: the proof it was this client" json:"previousDeviceProof,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// PairingOutput is a device's code, the secret by which it alone asks how its pairing stands, and
// when the code stops working.
type PairingOutput struct {
	Body struct {
		Secret    string `json:"secret"`
		Code      string `json:"code"`
		ExpiresAt int64  `doc:"Unix milliseconds" json:"expiresAt"`
	}
}

// PairingSecretInput names a device's pairing by its secret, which only the device holds.
type PairingSecretInput struct {
	Body struct {
		Secret string `json:"secret" maxLength:"100" minLength:"1"`
	}
	ClientFacts
}

// PairingStateOutput is how a device's pairing stands, and the check code to show while it is
// checked.
type PairingStateOutput struct {
	Body struct {
		State     string `enum:"waiting,checking,activated,ended" json:"state"`
		CheckCode string `json:"checkCode,omitempty"`
	}
}

// ActivationInput is a person signed in entering the code a device shows (D288 §2): for themselves,
// or, an admin, for someone else, under the name the device is listed by.
type ActivationInput struct {
	AuthorizationHeader
	Body struct {
		Code       string `doc:"As typed: case, dashes and spaces do not matter" json:"code" maxLength:"20" minLength:"1"`             //nolint:lll // one struct tag
		DeviceName string `doc:"A new device's name: \"Living room TV\"" json:"deviceName,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
		UserID     int64  `doc:"Whose device; an admin's to name" json:"userId,omitempty" required:"false"`
	}
}

// ActivationOutput is the check code the device now shows too, and the name it will be listed by.
type ActivationOutput struct {
	Body struct {
		CheckCode  string `json:"checkCode"`
		DeviceName string `json:"deviceName"`
	}
}

// ActivationAnswerInput is the person saying whether the two check codes match (D288 §3).
type ActivationAnswerInput struct {
	AuthorizationHeader
	Body struct {
		Code    string `json:"code"    maxLength:"20" minLength:"1"`
		IsMatch bool   `json:"isMatch"`
	}
}

func (server *Server) registerPairing() {
	tags := []string{tagAccounts}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "requestPairing", Method: http.MethodPost, Path: "/pairings",
		Summary: "A code for this device to show, which a person signed in enters to activate it (D288)", Tags: tags,
	}, server.requestPairing)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getPairingState", Method: http.MethodPost, Path: "/pairings/state",
		Summary: "How this device's pairing stands, and the check code to show", Tags: tags,
	}, server.getPairingState)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "takePairedSession", Method: http.MethodPost, Path: "/pairings/session",
		DefaultStatus: http.StatusCreated,
		Summary:       "The session a confirmed pairing made, once", Tags: tags,
	}, server.takePairedSession)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "activateDevice", Method: http.MethodPost, Path: "/activations",
		Summary: "Enter the code a device shows; both then show a check code (D288)", Tags: tags,
	}, server.activateDevice)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "answerActivation", Method: http.MethodPost, Path: "/activations/answer",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Say whether the check codes match: only a match activates the device", Tags: tags,
	}, server.answerActivation)
}

func (server *Server) requestPairing(ctx context.Context, input *PairingRequestInput) (*PairingOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	asked, err := dependencies.Auth.RequestPairing(
		ctx, input.Body.Secret, input.Body.Platform,
		auth.PreviousDevice{ID: input.Body.PreviousDeviceID, Proof: input.Body.PreviousDeviceProof},
		input.clientAddress)
	if err != nil {
		return nil, server.authError(ctx, "make a pairing", err)
	}
	output := &PairingOutput{}
	output.Body.Secret, output.Body.Code, output.Body.ExpiresAt = asked.Secret, asked.Code, asked.ExpiresAt.UnixMilli()
	return output, nil
}

func (server *Server) getPairingState(ctx context.Context, input *PairingSecretInput) (*PairingStateOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	state := dependencies.Auth.PairingStateOf(ctx, input.Body.Secret)
	output := &PairingStateOutput{}
	output.Body.State, output.Body.CheckCode = state.State, state.CheckCode
	return output, nil
}

func (server *Server) takePairedSession(ctx context.Context, input *PairingSecretInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	token, err := dependencies.Auth.TakePairedSession(ctx, input.Body.Secret)
	if err != nil {
		return nil, server.authError(ctx, "take a paired session", err)
	}
	return server.newSessionOutput(ctx, dependencies, token, input.isOverTLS)
}

func (server *Server) activateDevice(ctx context.Context, input *ActivationInput) (*ActivationOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	forUserID := principal.UserID
	if input.Body.UserID != 0 && input.Body.UserID != forUserID {
		if !principal.IsAdmin() {
			return nil, newError(http.StatusForbidden, CodeForbidden,
				"only an administrator may activate a device for someone else")
		}
		forUserID = input.Body.UserID
	}
	claim, err := dependencies.Auth.ClaimPairing(
		ctx, principal.UserID, forUserID, input.Body.Code, input.Body.DeviceName)
	if err != nil {
		return nil, server.authError(ctx, "enter a device's code", err)
	}
	output := &ActivationOutput{}
	output.Body.CheckCode, output.Body.DeviceName = claim.CheckCode, claim.DeviceName
	return output, nil
}

func (server *Server) answerActivation(ctx context.Context, input *ActivationAnswerInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.AnswerPairing(ctx, principal.UserID, input.Body.Code, input.Body.IsMatch); err != nil {
		return nil, server.authError(ctx, "answer a device's check code", err)
	}
	return &struct{}{}, nil
}

// pairingRefusal is the API's error for auth's refusal of a pairing, or nil for another.
func pairingRefusal(err error) *Error {
	switch {
	case errors.Is(err, auth.ErrNoPairing):
		return newError(http.StatusNotFound, CodeNotFound, auth.ErrNoPairing.Error())
	case errors.Is(err, auth.ErrTooManyPairings):
		return newError(http.StatusTooManyRequests, CodeRateLimited, auth.ErrTooManyPairings.Error())
	case errors.Is(err, auth.ErrNoDeviceName):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.deviceName", auth.ErrNoDeviceName.Error())
	}
	return nil
}
