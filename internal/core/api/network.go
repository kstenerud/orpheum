// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Network is the house's network settings that are the app's, not the compose file's (D462 §2): the
// HTTPS port devices at home reach the server on, the UDP port WebRTC is taken on there, and the
// router port mapping asks, empty for none.
type Network struct {
	HTTPSPort  int
	WebRTCPort int
	Router     string
}

// NetworkState is the network settings kept, the ones kept when the server started, and the ones it
// runs with: a port kept at the start that the server does not run on fell back to its default, as
// it would not bind (D462 §10.11).
type NetworkState struct {
	Kept, KeptAtStart, Running Network
}

// NetworkProblem is what is wrong with one setting of a network an admin asks for, Setting naming
// which: httpsPort, webrtcPort or router.
type NetworkProblem struct {
	Setting string
	Message string
}

// NetworkErrors are what is wrong with a network an admin asks for; NetworkSettings.Set's
// refusal, which keeps nothing.
type NetworkErrors []NetworkProblem

func (problems NetworkErrors) Error() string {
	var text strings.Builder
	text.WriteString("the network settings")
	for _, problem := range problems {
		text.WriteString("; " + problem.Setting + ": " + problem.Message)
	}
	return text.String()
}

// The settings a NetworkProblem names, as the API's fields do.
const (
	SettingHTTPSPort  = "httpsPort"
	SettingWebRTCPort = "webrtcPort"
	SettingRouter     = "router"
)

// NetworkSettings keeps the house's network settings the System page sets (D462 §2), which the
// server's next start takes: on a compose route its restart, on the install script's its next run.
type NetworkSettings interface {
	State() (NetworkState, error)
	// Set keeps network once each setting is good and each port that changed binds, refusing it
	// with NetworkErrors otherwise.
	Set(ctx context.Context, network Network) error
}

// NetworkBody is one set of the house's network settings, as the System page shows and sets them.
type NetworkBody struct {
	HTTPSPort  int    `doc:"The HTTPS port devices at home reach the server on by its name at home" json:"httpsPort" maximum:"65535" minimum:"1"`     //nolint:lll // one struct tag
	WebRTCPort int    `doc:"The UDP port WebRTC is taken on at home" json:"webrtcPort" maximum:"65535" minimum:"1"`                                   //nolint:lll // one struct tag
	Router     string `doc:"The house's router, whose port mapping is asked to forward the HTTPS port; empty asks none" json:"router" maxLength:"15"` //nolint:lll // one struct tag
}

// NetworkStateBody is the house's network settings: the ones kept, the ones the server runs with, and
// what stands between them.
type NetworkStateBody struct {
	Kept    NetworkBody `doc:"The settings kept, which the server's next start takes" json:"kept"`
	Running NetworkBody `doc:"The settings the server runs with now" json:"running"`
	// FellBack are the ports kept when the server started that it could not take, so it runs on the
	// default instead (D462 §10.11).
	FellBack []string `doc:"The settings kept at the start whose port would not bind, so the server took the default" enum:"httpsPort,webrtcPort" json:"fellBack" nullable:"false"` //nolint:lll // one struct tag
	Pending  bool     `doc:"Whether settings were kept since the server started, which its next start takes" json:"pending"`                                                        //nolint:lll // one struct tag
	// AppliedBy is what takes the settings kept: the restart this page asks for, or the install
	// script's next run, which writes the server's config.
	AppliedBy string `doc:"What takes the settings kept: restart (this page's) or script (the install script's next run)" enum:"restart,script" json:"appliedBy"` //nolint:lll // one struct tag
}

// NetworkStateOutput is GET and PUT /system/network's answer.
type NetworkStateOutput struct {
	Body NetworkStateBody
}

// SetNetworkInput is an admin's setting of the house's network.
type SetNetworkInput struct {
	AuthorizationHeader
	Body NetworkBody
}

func (server *Server) registerNetwork() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSystemNetwork", Method: http.MethodGet, Path: "/system/network",
		Tags:       []string{tagRunningIt},
		Summary:    "The house's network settings the System page keeps, and the ones the server runs with (D462 §2)",
		Extensions: callableBy(capabilityAdmin),
	}, server.getSystemNetwork)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setSystemNetwork", Method: http.MethodPut, Path: "/system/network",
		Tags: []string{tagRunningIt},
		Summary: "Keep the house's network settings for the server's next start, once each is good and " +
			"each port changed binds (D462 §2, §10.11)",
		Extensions: callableBy(capabilityAdmin),
	}, server.setSystemNetwork)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "restartServer", Method: http.MethodPost, Path: "/system/restart",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusAccepted,
		Summary: "Stop the server so its container's restart policy starts it again, with the network " +
			"settings kept (D462 §2)",
		Extensions: callableBy(capabilityAdmin),
	}, server.restartServer)
}

// errNoNetworkSettings is the refusal when the server keeps no network settings: one its config file
// alone sets, as a development server's.
var errNoNetworkSettings = newError(http.StatusConflict, CodeConflict,
	"this server's network settings are its config file's")

func (server *Server) getSystemNetwork(ctx context.Context, input *AuthorizationHeader) (*NetworkStateOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.networkState(ctx, dependencies)
}

func (server *Server) setSystemNetwork(ctx context.Context, input *SetNetworkInput) (*NetworkStateOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.Network == nil {
		return nil, errNoNetworkSettings
	}
	asked := Network(input.Body)
	var problems NetworkErrors
	switch err = dependencies.Network.Set(ctx, asked); {
	case errors.As(err, &problems):
		return nil, networkRefusal(problems)
	case err != nil:
		return nil, server.logInternalError(ctx, "keep the network settings", err)
	}
	return server.networkState(ctx, dependencies)
}

// networkRefusal is problems as the API refuses them, at the first one's field, its message naming
// every one.
func networkRefusal(problems NetworkErrors) *Error {
	setting := SettingHTTPSPort
	if len(problems) != 0 {
		setting = problems[0].Setting
	}
	return invalidRequestAt(http.StatusBadRequest, "body."+setting, problems.Error())
}

// networkState is the network settings as the System page shows them.
func (server *Server) networkState(ctx context.Context, dependencies *Dependencies) (*NetworkStateOutput, error) {
	if dependencies.Network == nil {
		return nil, errNoNetworkSettings
	}
	state, err := dependencies.Network.State()
	if err != nil {
		return nil, server.logInternalError(ctx, "read the network settings", err)
	}
	body := NetworkStateBody{
		Kept: networkBody(state.Kept), Running: networkBody(state.Running), FellBack: []string{},
		Pending: state.Kept != state.KeptAtStart, AppliedBy: "restart",
	}
	if dependencies.InstallRoute == installRouteScript {
		body.AppliedBy = installRouteScript
	}
	if state.KeptAtStart.HTTPSPort != state.Running.HTTPSPort {
		body.FellBack = append(body.FellBack, SettingHTTPSPort)
	}
	if state.KeptAtStart.WebRTCPort != state.Running.WebRTCPort {
		body.FellBack = append(body.FellBack, SettingWebRTCPort)
	}
	return &NetworkStateOutput{Body: body}, nil
}

// installRouteScript is the install route of a server the install script installed (D450 §2), whose
// config its next run writes.
const installRouteScript = "script"

// networkBody is network as the API sends it.
func networkBody(network Network) NetworkBody {
	return NetworkBody(network)
}

// RestartOutput is POST /system/restart's answer: accepted, as the server stops once it is sent.
type RestartOutput struct {
	Status int
}

func (server *Server) restartServer(ctx context.Context, input *AuthorizationHeader) (*RestartOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.Restart == nil || dependencies.InstallRoute == installRouteScript {
		return nil, newError(http.StatusConflict, CodeConflict,
			"this server is not restarted from here: its install script's next run restarts it")
	}
	dependencies.Restart()
	return &RestartOutput{Status: http.StatusAccepted}, nil
}
