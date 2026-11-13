// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// PairingRequestInput is a device not signed in asking for a code to show (D288 §1): with the secret
// of the pairing it has, for the same code again, its platform, and the device it was before.
type PairingRequestInput struct {
	Body struct {
		Secret              string `doc:"The pairing this device has, for the same code again" json:"secret,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
		Platform            string `json:"platform" maxLength:"40" minLength:"1"`
		PreviousDeviceID    int64  `doc:"The device this client was signed in as before (D221)" json:"previousDeviceId,omitempty" required:"false"`                                            //nolint:lll // one struct tag
		PreviousDeviceProof string `doc:"The device secret a sign-in on that device gave: the proof it was this client" json:"previousDeviceProof,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
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
		KeyHash    string `doc:"The device's key's hash, as its QR code carries it: the code was scanned, not typed (D321)" json:"keyHash,omitempty" maxLength:"64" required:"false"` //nolint:lll // one struct tag
	}
}

// ActivationOutput is the check code the device now shows too, none for a scanned code (D321), the
// name it will be listed by, and what it says it is and when it asked for its code, which the person
// is shown: a code asked for long before is more likely one someone sent them (the device code
// audit's M3).
type ActivationOutput struct {
	Body struct {
		CheckCode       string `json:"checkCode"`
		DeviceName      string `json:"deviceName"`
		Platform        string `json:"platform"`
		AskedSecondsAgo int64  `doc:"How long ago the device asked for its code, by the server's clock" json:"askedSecondsAgo"` //nolint:lll // one struct tag
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

// CastToPairingInput is a person signed in answering the code they scanned for themselves (D321) by
// casting to its device, for one cast (D469 §1), where they would have activated it; a typed code,
// whose check code is to match, is cast to by no one.
type CastToPairingInput struct {
	AuthorizationHeader
	Body struct {
		Code string `json:"code" maxLength:"20" minLength:"1"`
	}
}

// CastToPairingOutput is the guest receiver the device signs in as, which the asking device casts to
// once its page connects (POST /me/casts), and its name.
type CastToPairingOutput struct {
	Body struct {
		DeviceID   int64  `json:"deviceId"`
		DeviceName string `json:"deviceName"`
	}
}

func (server *Server) registerPairing() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "requestPairing", Method: http.MethodPost, Path: "/pairings",
		Summary: "A code for this device to show, which a person signed in enters to activate it (D288)", Tags: tags,
		Extensions: callableBy(capabilityNone),
	}, server.requestPairing)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getPairingState", Method: http.MethodPost, Path: "/pairings/state",
		Summary: "How this device's pairing stands, and the check code to show", Tags: tags,
		Extensions: callableBy(capabilityNone),
	}, server.getPairingState)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "takePairedSession", Method: http.MethodPost, Path: "/pairings/session",
		DefaultStatus: http.StatusCreated,
		Summary:       "The session a confirmed pairing made, once", Tags: tags,
		Extensions: callableBy(capabilityNone),
	}, server.takePairedSession)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "enterPairingCode", Method: http.MethodPost, Path: "/pairings/code",
		Summary: "Enter the code a device shows; both then show a check code (D288)", Tags: tags,
		Extensions: callableBy(capabilityAccountSecurity),
	}, server.enterPairingCode)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "answerPairingCheck", Method: http.MethodPost, Path: "/pairings/check",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Say whether the check codes match: only a match activates the device", Tags: tags,
		Extensions: callableBy(capabilityAccountSecurity),
	}, server.answerPairingCheck)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "castToPairing", Method: http.MethodPost, Path: "/pairings/cast",
		Summary: "Answer a code entered by casting to its device, signed in for that cast alone (D469)",
		Tags:    tags, DefaultStatus: http.StatusCreated,
		Extensions: callableBy(capabilityAccountSecurity),
	}, server.castToPairing)
	// The old paths, kept outside the document for one release (D425 §3): Connect serves the web
	// build it was last deployed with, which enters a code at /activations. They go in the next batch.
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "enterPairingCodeAtItsOldPath", Method: http.MethodPost, Path: "/activations",
		Hidden:     true,
		Extensions: callableBy(capabilityAccountSecurity),
	}, server.enterPairingCodeAtItsOldPath)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "answerPairingCheckAtItsOldPath", Method: http.MethodPost, Path: "/activations/answer",
		Hidden: true, DefaultStatus: http.StatusNoContent,
		Extensions: callableBy(capabilityAccountSecurity),
	}, server.answerPairingCheckAtItsOldPath)
}

// enterPairingCodeAtItsOldPath is enterPairingCode at the path it had before D425 §4, for one
// release (D425 §3).
func (server *Server) enterPairingCodeAtItsOldPath(
	ctx context.Context, input *ActivationInput,
) (*ActivationOutput, error) {
	return server.enterPairingCode(ctx, input)
}

// answerPairingCheckAtItsOldPath is answerPairingCheck at the path it had before D425 §4, for one
// release (D425 §3).
func (server *Server) answerPairingCheckAtItsOldPath(
	ctx context.Context, input *ActivationAnswerInput,
) (*struct{}, error) {
	return server.answerPairingCheck(ctx, input)
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
	issued, err := dependencies.Auth.TakePairedSession(ctx, input.Body.Secret)
	if err != nil {
		return nil, server.authError(ctx, "take a paired session", err)
	}
	return server.newSessionOutput(ctx, dependencies, issued, input.isOverTLS)
}

func (server *Server) enterPairingCode(ctx context.Context, input *ActivationInput) (*ActivationOutput, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	forUserID := principal.UserID
	if input.Body.UserID != 0 && input.Body.UserID != forUserID {
		// Administering here, not the role alone: a stolen admin session away from home must not
		// plant devices as others (the Plex-chain review's T1).
		if !administersHere(ctx, principal) {
			return nil, newError(http.StatusForbidden, CodeForbidden,
				"only an administrator may activate a device for someone else")
		}
		forUserID = input.Body.UserID
	}
	var claim auth.PairingClaim
	if input.Body.KeyHash != "" {
		claim, err = dependencies.Auth.ClaimScannedPairing(
			ctx, principal.UserID, forUserID, input.Body.Code, input.Body.KeyHash, input.Body.DeviceName)
	} else {
		claim, err = dependencies.Auth.ClaimPairing(
			ctx, principal.UserID, forUserID, input.Body.Code, input.Body.DeviceName)
	}
	if err != nil {
		return nil, server.authError(ctx, "enter a device's code", err)
	}
	output := &ActivationOutput{}
	output.Body.CheckCode, output.Body.DeviceName = claim.CheckCode, claim.DeviceName
	output.Body.Platform = claim.Platform
	output.Body.AskedSecondsAgo = max(int64(dependencies.Clock.Now().Sub(claim.AskedAt)/time.Second), 0)
	return output, nil
}

func (server *Server) answerPairingCheck(ctx context.Context, input *ActivationAnswerInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.AnswerPairing(ctx, principal.UserID, input.Body.Code, input.Body.IsMatch); err != nil {
		return nil, server.authError(ctx, "answer a device's check code", err)
	}
	return &struct{}{}, nil
}

// castToPairing signs the device that showed the code in as a guest receiver of the asking device's
// (D469 §1, §2): it takes its session as an activated device does, is listed nowhere, and is removed
// once its one cast ends, or if none starts within live.GuestByCodeWaitsAfter.
func (server *Server) castToPairing(ctx context.Context, input *CastToPairingInput) (*CastToPairingOutput, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	guest, err := dependencies.Auth.CastToPairing(ctx, principal, input.Body.Code)
	if err != nil {
		return nil, server.authError(ctx, "cast to a device's code", err)
	}
	dependencies.Live.ExpectGuestByCode(guest.UserID, guest.DeviceID, guest.SenderDeviceID, guest.Name)
	server.logger.InfoContext(ctx, "cast: a device's code made a guest receiver", slogx.EventKey,
		"guestReceiverByCode", logKeyUserID, guest.UserID, logKeyDeviceID, guest.DeviceID, "sender",
		guest.SenderDeviceID)
	output := &CastToPairingOutput{}
	output.Body.DeviceID, output.Body.DeviceName = guest.DeviceID, guest.Name
	return output, nil
}

// pairingRefusal is the API's error for auth's refusal of a pairing, or nil for another.
func pairingRefusal(err error) *Error {
	switch {
	case errors.Is(err, auth.ErrNoPairing):
		return newError(http.StatusNotFound, CodeNotFound, auth.ErrNoPairing.Error())
	case errors.Is(err, auth.ErrTooManyPairings):
		return newError(http.StatusTooManyRequests, CodeRateLimited, auth.ErrTooManyPairings.Error())
	case errors.Is(err, auth.ErrActivatesAnAdmin):
		return newError(http.StatusForbidden, CodeForbidden, auth.ErrActivatesAnAdmin.Error())
	case errors.Is(err, auth.ErrDeviceSilent):
		return newError(http.StatusConflict, CodeConflict, auth.ErrDeviceSilent.Error())
	case errors.Is(err, auth.ErrNotThatDevice):
		return newError(http.StatusConflict, CodeConflict, auth.ErrNotThatDevice.Error())
	case errors.Is(err, auth.ErrNoDeviceName):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.deviceName", auth.ErrNoDeviceName.Error())
	}
	return nil
}
