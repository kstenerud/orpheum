// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

// DeviceBody is one of a person's devices (D218 §4).
type DeviceBody struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	AddedAt      int64  `doc:"Unix milliseconds" json:"addedAt"`
	LastUsedAt   int64  `doc:"Unix milliseconds" json:"lastUsedAt"`
	IsSignedIn   bool   `json:"isSignedIn"`
	IsByCode     bool   `doc:"A code activated it: it neither administers nor changes the account" json:"isByCode"` //nolint:lll // one struct tag
	IsThisDevice bool   `doc:"The device asking" json:"isThisDevice"`
	// IsInYourWatchTogether is whether the device is in the Watch Together session the asker is in,
	// which is never a cast target (D384 §3); told of the asker's own devices alone.
	IsInYourWatchTogether bool `doc:"In the Watch Together session you are in, so never a cast target (D384 §3)" json:"isInYourWatchTogether,omitempty" required:"false"` //nolint:lll // one struct tag
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

// ChangeDeviceInput renames one of the asker's devices, or says how far it moves its sound from its
// picture, or both. How it makes sound is asked of no one (D336 §4).
type ChangeDeviceInput struct {
	AuthorizationHeader
	DeviceID int64 `minimum:"1" path:"id"`
	Body     struct {
		Name *string `json:"name,omitempty" maxLength:"100" minLength:"1" required:"false"`
		// SoundOffsetMs moves the device's sound from its picture, later when positive and earlier when
		// negative, for a device that plays one of them late: the owner's TCL TV, in TV Bro, plays its
		// sound half a second late (2026-10-24).
		SoundOffsetMs *int64 `doc:"How far the device moves its sound from its picture, in ms: later when positive, earlier when negative" json:"soundOffsetMs,omitempty" maximum:"2000" minimum:"-2000" required:"false"` //nolint:lll // one struct tag
	}
}

// UserInput names a user, for the admin.
type UserInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
}

// UserDeviceInput names one of a user's devices, for the admin.
type UserDeviceInput struct {
	AuthorizationHeader
	UserID   int64 `minimum:"1" path:"id"`
	DeviceID int64 `minimum:"1" path:"deviceId"`
}

func (server *Server) registerDevices() {
	server.registerCasts()
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listMyDevices", Method: http.MethodGet, Path: "/me/devices",
		Summary: "One's own devices, most recently used first (D218)", Tags: tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.listMyDevices)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "changeMyDevice", Method: http.MethodPut, Path: "/me/devices/{id}",
		Summary: "Rename one of one's devices, or set how far it moves its sound", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.changeMyDevice)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeMyDevice", Method: http.MethodDelete, Path: "/me/devices/{id}",
		Summary: "Remove one of one's devices; it is signed out at its next request", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAccountSecurity),
	}, server.removeMyDevice)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "signOutElsewhere", Method: http.MethodPost, Path: "/me/devices/sign-out-elsewhere",
		Summary: "Sign out every device but this one; they stay listed", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAccountSecurity),
	}, server.signOutElsewhere)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listUserDevices", Method: http.MethodGet, Path: "/users/{id}/devices",
		Summary: "A user's devices, for the admin", Tags: tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.listUserDevices)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeUserDevice", Method: http.MethodDelete, Path: "/users/{id}/devices/{deviceId}",
		Summary: "Remove a user's device, for the admin: a lost or given-away one", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAdmin),
	}, server.removeUserDevice)
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
			AddedAt:    device.AddedAt.UnixMilli(),
			LastUsedAt: device.LastUsedAt.UnixMilli(), IsSignedIn: device.IsSignedIn, IsByCode: device.IsByCode,
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
	output, err := server.devicesOf(ctx, dependencies, principal.UserID, principal.DeviceID)
	if err != nil {
		return nil, err
	}
	inSession := dependencies.Together.SignedInDevicesWith(principal.UserID)
	for index := range output.Body.Devices {
		output.Body.Devices[index].IsInYourWatchTogether = slices.Contains(inSession, output.Body.Devices[index].ID)
	}
	return output, nil
}

func (server *Server) changeMyDevice(ctx context.Context, input *ChangeDeviceInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if input.Body.Name == nil && input.Body.SoundOffsetMs == nil {
		return nil, invalidRequestAt(http.StatusBadRequest, "body", "a change names a new name or its sound offset")
	}
	if name := input.Body.Name; name != nil {
		err = dependencies.Auth.RenameDevice(ctx, principal.UserID, input.DeviceID, *name)
	}
	if offsetMs := input.Body.SoundOffsetMs; err == nil && offsetMs != nil {
		err = dependencies.Auth.SetSoundOffset(ctx, principal.UserID, input.DeviceID, *offsetMs)
	}
	return server.deviceAnswer(ctx, "change a device", err)
}

func (server *Server) removeMyDevice(ctx context.Context, input *DeviceInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RemoveDevice(ctx, principal.UserID, input.DeviceID)
	if err == nil {
		dependencies.Live.SignedOut(principal.UserID, input.DeviceID)
	}
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

func (server *Server) listUserDevices(ctx context.Context, input *UserInput) (*DevicesOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.devicesOf(ctx, dependencies, input.UserID, principal.DeviceID)
}

func (server *Server) removeUserDevice(ctx context.Context, input *UserDeviceInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RemoveDevice(ctx, input.UserID, input.DeviceID)
	if err == nil {
		dependencies.Live.SignedOut(input.UserID, input.DeviceID)
	}
	return server.deviceAnswer(ctx, "remove a person's device", err)
}

// deviceAnswer is what a change to a device answers: nothing, or why not.
func (server *Server) deviceAnswer(ctx context.Context, action string, err error) (*struct{}, error) {
	if err != nil {
		return nil, server.authError(ctx, action, err)
	}
	return &struct{}{}, nil
}
