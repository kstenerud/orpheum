// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// AudioOutputKind is how a device makes sound, documented as one of auth.AudioOutputs, so the API
// names every answer without a second list to keep in step (D222).
type AudioOutputKind string

// Schema is an audio output's schema: a string, one of auth.AudioOutputs.
func (AudioOutputKind) Schema(huma.Registry) *huma.Schema {
	outputs := auth.AudioOutputs()
	names := make([]any, 0, len(outputs))
	for _, output := range outputs {
		names = append(names, output)
	}
	return &huma.Schema{Type: huma.TypeString, Enum: names}
}

// DeviceBody is one of a person's devices (D218 §4).
type DeviceBody struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Platform     string          `json:"platform"`
	AudioOutput  AudioOutputKind `doc:"Absent until the device is asked, when it first plays (D222)" json:"audioOutput,omitempty"` //nolint:lll // one struct tag
	AddedAt      int64           `doc:"Unix milliseconds" json:"addedAt"`
	LastUsedAt   int64           `doc:"Unix milliseconds" json:"lastUsedAt"`
	IsSignedIn   bool            `json:"isSignedIn"`
	IsByCode     bool            `doc:"A code activated it: it neither administers nor changes the account" json:"isByCode"` //nolint:lll // one struct tag
	IsThisDevice bool            `doc:"The device asking" json:"isThisDevice"`
}

// DevicesOutput is a person's devices, most recently used first.
type DevicesOutput struct {
	Body struct {
		Devices []DeviceBody `json:"devices"`
	}
}

// DeviceInput names one of the asker's devices.
type DeviceInput struct {
	AuthorizationHeader
	DeviceID int64 `minimum:"1" path:"id"`
}

// ChangeDeviceInput renames one of the asker's devices, or says how it makes sound, or both.
type ChangeDeviceInput struct {
	AuthorizationHeader
	DeviceID int64 `minimum:"1" path:"id"`
	Body     struct {
		Name        *string          `json:"name,omitempty" maxLength:"100" minLength:"1" required:"false"`
		AudioOutput *AudioOutputKind `doc:"How the device makes sound; built-in speakers and headphones are stereo (D222)" json:"audioOutput,omitempty" required:"false"` //nolint:lll // one struct tag
	}
}

// PersonInput names a person, for the admin.
type PersonInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
}

// PersonDeviceInput names one of a person's devices, for the admin.
type PersonDeviceInput struct {
	AuthorizationHeader
	UserID   int64 `minimum:"1" path:"id"`
	DeviceID int64 `minimum:"1" path:"deviceId"`
}

func (server *Server) registerDevices() {
	tags := []string{tagAccounts}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listMyDevices", Method: http.MethodGet, Path: "/me/devices",
		Summary: "One's own devices, most recently used first (D218)", Tags: tags,
	}, server.listMyDevices)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "changeMyDevice", Method: http.MethodPut, Path: "/me/devices/{id}",
		Summary: "Rename one of one's devices, or say how it makes sound", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.changeMyDevice)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeMyDevice", Method: http.MethodDelete, Path: "/me/devices/{id}",
		Summary: "Remove one of one's devices; it is signed out at its next request", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.removeMyDevice)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "signOutElsewhere", Method: http.MethodPost, Path: "/me/devices/sign-out-elsewhere",
		Summary: "Sign out every device but this one; they stay listed", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.signOutElsewhere)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listPersonDevices", Method: http.MethodGet, Path: "/users/{id}/devices",
		Summary: "A person's devices, for the admin", Tags: tags,
	}, server.listPersonDevices)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removePersonDevice", Method: http.MethodDelete, Path: "/users/{id}/devices/{deviceId}",
		Summary: "Remove a person's device, for the admin: a lost or given-away one", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.removePersonDevice)
}

func (server *Server) devicesOf(ctx context.Context, dependencies *Dependencies, userID, thisDeviceID int64) (
	*DevicesOutput, error,
) {
	devices, err := dependencies.Auth.Devices(ctx, userID)
	if err != nil {
		return nil, server.authError(ctx, "list devices", err)
	}
	output := &DevicesOutput{}
	output.Body.Devices = make([]DeviceBody, 0, len(devices))
	for _, device := range devices {
		output.Body.Devices = append(output.Body.Devices, DeviceBody{
			ID: device.ID, Name: device.Name, Platform: device.Platform,
			AudioOutput: AudioOutputKind(device.AudioOutput),
			AddedAt:     device.AddedAt.UnixMilli(),
			LastUsedAt:  device.LastUsedAt.UnixMilli(), IsSignedIn: device.IsSignedIn, IsByCode: device.IsByCode,
			IsThisDevice: device.ID == thisDeviceID,
		})
	}
	return output, nil
}

func (server *Server) listMyDevices(ctx context.Context, input *struct{ AuthorizationHeader }) (*DevicesOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.devicesOf(ctx, dependencies, principal.UserID, principal.DeviceID)
}

func (server *Server) changeMyDevice(ctx context.Context, input *ChangeDeviceInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if input.Body.Name == nil && input.Body.AudioOutput == nil {
		return nil, invalidRequestAt(http.StatusBadRequest, "body",
			"a change names a new name or how the device makes sound")
	}
	if name := input.Body.Name; name != nil {
		err = dependencies.Auth.RenameDevice(ctx, principal.UserID, input.DeviceID, *name)
	}
	if audioOutput := input.Body.AudioOutput; err == nil && audioOutput != nil {
		err = dependencies.Auth.SetAudioOutput(ctx, principal.UserID, input.DeviceID, string(*audioOutput))
	}
	return server.deviceAnswer(ctx, "change a device", err)
}

func (server *Server) removeMyDevice(ctx context.Context, input *DeviceInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RemoveDevice(ctx, principal.UserID, input.DeviceID)
	return server.deviceAnswer(ctx, "remove a device", err)
}

func (server *Server) signOutElsewhere(ctx context.Context, input *struct{ AuthorizationHeader }) (*struct{}, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.SignOutElsewhere(ctx, principal.UserID, bearerToken(input.Authorization))
	return server.deviceAnswer(ctx, "sign out elsewhere", err)
}

func (server *Server) listPersonDevices(ctx context.Context, input *PersonInput) (*DevicesOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.devicesOf(ctx, dependencies, input.UserID, principal.DeviceID)
}

func (server *Server) removePersonDevice(ctx context.Context, input *PersonDeviceInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RemoveDevice(ctx, input.UserID, input.DeviceID)
	return server.deviceAnswer(ctx, "remove a person's device", err)
}

// deviceAnswer is what a change to a device answers: nothing, or why not.
func (server *Server) deviceAnswer(ctx context.Context, action string, err error) (*struct{}, error) {
	if err != nil {
		return nil, server.authError(ctx, action, err)
	}
	return &struct{}{}, nil
}
