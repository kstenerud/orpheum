// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// Ports are the ports of the house's network a deployment takes that the System page sets: HTTPS
// over TCP and WebRTC over UDP (D462 §2).
type Ports struct {
	HTTPS  int
	WebRTC int
}

// errWontBind is a port's that another program holds, or this one may not take.
var errWontBind = errors.New("will not bind")

// portBinds is whether this machine lets the core take ports, each on every address: the address
// at home alone is never bound to, as it moves (D462 §10.11). A port of 0 is not tried, and none is
// held after.
func portBinds(ports Ports) error {
	var config net.ListenConfig
	if ports.HTTPS != 0 {
		listener, err := config.Listen(context.Background(), "tcp", net.JoinHostPort("", strconv.Itoa(ports.HTTPS)))
		if err != nil {
			return fmt.Errorf("the HTTPS port %d %w: %w", ports.HTTPS, errWontBind, err)
		}
		if closeErr := listener.Close(); closeErr != nil {
			return fmt.Errorf("free the HTTPS port %d: %w", ports.HTTPS, closeErr)
		}
	}
	if ports.WebRTC != 0 {
		packets, err := config.ListenPacket(context.Background(), "udp",
			net.JoinHostPort("", strconv.Itoa(ports.WebRTC)))
		if err != nil {
			return fmt.Errorf("the WebRTC port %d %w: %w", ports.WebRTC, errWontBind, err)
		}
		if closeErr := packets.Close(); closeErr != nil {
			return fmt.Errorf("free the WebRTC port %d: %w", ports.WebRTC, closeErr)
		}
	}
	return nil
}

// bindable is network with each port binds refuses replaced by start's default, and a line saying
// so for each (D462 §10.11): the System page shows that the port it keeps is not the one taken. A
// default that will not bind either is kept, and the core's start says why.
func bindable(network deployment.Network, binds func(Ports) error) (deployment.Network, []string) {
	var said []string
	if network.HTTPSPort != startHTTPSPort {
		if err := binds(Ports{HTTPS: network.HTTPSPort}); err != nil {
			said = append(said, fmt.Sprintf("%v; the HTTPS port is %d instead", err, startHTTPSPort))
			network.HTTPSPort = startHTTPSPort
		}
	}
	if network.WebRTCPort != startHTTPSPort {
		if err := binds(Ports{WebRTC: network.WebRTCPort}); err != nil {
			said = append(said, fmt.Sprintf("%v; the WebRTC port is %d instead", err, startHTTPSPort))
			network.WebRTCPort = startHTTPSPort
		}
	}
	return network, said
}
