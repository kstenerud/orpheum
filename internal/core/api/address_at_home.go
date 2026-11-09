// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// Ports a Host with none names: plain HTTP's and HTTPS's.
const (
	plainHTTPPort = 80
	httpsPort     = 443
)

// addressAtHome is the address at home a direct request teaches, and the ports on which a request
// naming the server's address teaches it: the house's plain and HTTPS ports, never the one only the
// server's own machine reaches (D452 §3).
type addressAtHome struct {
	keeper   *home.AddressAtHome
	teaching []uint16
}

// SetAddressAtHome names the address at home, which a direct request from home teaches by naming the
// server by its address on one of teachingPorts, or by a name that resolves to one (D452 §3, §4), and
// which the System page shows and an admin sets. Nil names none: nothing is learned.
func (server *Server) SetAddressAtHome(keeper *home.AddressAtHome, teachingPorts ...uint16) {
	if keeper == nil {
		server.addressAtHome.Store(nil)
		return
	}
	server.addressAtHome.Store(&addressAtHome{keeper: keeper, teaching: slices.Clone(teachingPorts)})
}

// keeper is the address at home SetAddressAtHome named; nil for none.
func (server *Server) keeper() *home.AddressAtHome {
	named := server.addressAtHome.Load()
	if named == nil {
		return nil
	}
	return named.keeper
}

// learnAddressAtHome has request, which came directly from home (isHome), teach the address at home:
// its Host's private IPv4 address on a teaching port, or its name, looked up apart from the request.
// Loopback, the port only this machine reaches and a request over WebRTC never teach (D452 §3); one
// that names the address known costs one comparison.
func (server *Server) learnAddressAtHome(request *http.Request, isHome bool) {
	named := server.addressAtHome.Load()
	if named == nil || !isHome || rtcapi.IsOverWebRTC(request.Context()) || server.isOnThisMachinesPort(request) ||
		server.isLoopback(request.RemoteAddr) {
		return
	}
	host, port := hostAndPort(request)
	if !slices.Contains(named.teaching, port) {
		return
	}
	// The device's own words, recorded as where the address was learned and never branched on, as
	// session.go records them (D2, D103).
	device := request.Header.Get("User-Agent")
	if address, err := netip.ParseAddr(host); err == nil {
		named.keeper.Learn(request.Context(), address, device)
		return
	}
	if !named.keeper.IsLearning() {
		return
	}
	go named.keeper.LearnName(context.WithoutCancel(request.Context()), host, device)
}

// hostAndPort is the host and port request's Host names, the port its scheme's when it names none.
func hostAndPort(request *http.Request) (string, uint16) {
	port := uint16(plainHTTPPort)
	if request.TLS != nil {
		port = httpsPort
	}
	host, portText, err := net.SplitHostPort(request.Host)
	if err != nil {
		return request.Host, port
	}
	if parsed, parseErr := strconv.ParseUint(portText, 10, 16); parseErr == nil {
		port = uint16(parsed)
	}
	return host, port
}

// isLoopback is whether remote, a request's "host:port", is a loopback address.
func (*Server) isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.Unmap().IsLoopback()
}

// AddressAtHomeBody is the server's address at home, as the System page shows it (D452 §3, §4).
type AddressAtHomeBody struct {
	// Address is the address; empty when none is known yet.
	Address string `json:"address"`
	// Origin is where it came from: configured (ORPHEUM_LAN_ADDRESS, which wins), learned from a
	// request at home, set by an admin, guessed from the default route's interface until one is, or
	// empty for none.
	Origin string `enum:"configured,learned,set,guessed," json:"origin"`
	// LearnedFrom is the device it was learned from, its browser's user agent.
	LearnedFrom string `json:"learnedFrom,omitempty"`
	// SetByName is the name of the admin who set it; empty once they are removed.
	SetByName string `json:"setByName,omitempty"`
	At        int64  `doc:"Unix milliseconds; absent when configured, guessed or none" json:"at,omitempty"`
}

// addressAtHomeBody is the address at home as the System page shows it.
func (server *Server) addressAtHomeBody() AddressAtHomeBody {
	known := server.keeper().Known()
	body := AddressAtHomeBody{Origin: string(known.Origin), LearnedFrom: known.Device, SetByName: known.SetByName}
	if known.Address.IsValid() {
		body.Address = known.Address.String()
	}
	if known.Address.IsValid() && (known.Origin == home.OriginLearned || known.Origin == home.OriginSet) {
		body.At = known.At.UnixMilli()
	}
	return body
}

// SetAddressAtHomeInput is an admin's setting of the address at home.
type SetAddressAtHomeInput struct {
	AuthorizationHeader
	Body struct {
		// Address is the server's private IPv4 address in the house.
		Address string `json:"address" maxLength:"15" minLength:"7"`
	}
}

func (server *Server) registerAddressAtHome() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setAddressAtHome", Method: http.MethodPut, Path: "/system/address-at-home",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary:    "Set the server's address in the house, which devices at home reach it at (D452 §4)",
		Extensions: callableBy(capabilityAdmin),
	}, server.setAddressAtHome)
}

func (server *Server) setAddressAtHome(ctx context.Context, input *SetAddressAtHomeInput) (*struct{}, error) {
	_, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	keeper := server.keeper()
	if keeper == nil {
		return nil, newError(http.StatusConflict, CodeConflict, "this server keeps no address at home")
	}
	address, err := netip.ParseAddr(input.Body.Address)
	if err != nil {
		return nil, invalidRequestAt(http.StatusBadRequest, "body.address", home.ErrNotAtHome.Error())
	}
	err = keeper.Set(ctx, address, principal.UserID)
	switch {
	case errors.Is(err, home.ErrConfigured):
		return nil, newError(http.StatusConflict, CodeConflict, err.Error())
	case errors.Is(err, home.ErrNotAtHome):
		return nil, invalidRequestAt(http.StatusBadRequest, "body.address", err.Error())
	case err != nil:
		return nil, server.logInternalError(ctx, "set the address at home", err)
	}
	return &struct{}{}, nil
}
