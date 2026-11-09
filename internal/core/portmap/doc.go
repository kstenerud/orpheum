// SPDX-License-Identifier: AGPL-3.0-or-later

// Package portmap keeps the core's HTTPS port open at the home router while its discovery publishes
// it and shouldMapPorts is on (D273 §7): IPv4 forwarded by PCP, then NAT-PMP, then UPnP IGD, and
// each global IPv6 address let through the router's firewall by UPnP IGD 2. Each is spoken here, on
// the standard library alone, and each mapping is renewed before it lapses and removed when no
// longer wanted. What it opened becomes the discovery's mapped candidates and the System page's
// report.
//
// # Invariants
//
//   - A PCP mapping is renewed with its own nonce at half the lifetime the router granted, not
//     before, and removed (lifetime 0) once no longer wanted.
//     TestStep_MapsByPCPRenewsAtHalfItsLifetimeAndRemovesIt.
//   - A router that answers PCP as NAT-PMP routers do is asked by NAT-PMP.
//     TestStep_AsksByNATPMPWhenTheRouterSpeaksOnlyIt.
//   - A router that speaks neither is found by an SSDP search answered from the gateway's own
//     address; it forwards IPv4 and opens IPv6 by UPnP, and both are removed when the core stops.
//     TestStep_OpensByUPnPForIPv4AndIPv6AndReleaseClosesThem.
//   - Every UPnP request goes to the gateway's own address over plain HTTP: the search's location, the
//     description's URL base and its control URLs on any other host are left out (the Plex-chain
//     review's O2). TestRouterOf_TakesOnlyURLsOnTheGateway.
//   - The gateway is the router the deployment names, else the default route's, from Linux's routing
//     table. TestGatewayOf_ReadsTheDefaultRoutesGateway; cmd/orpheum's
//     TestPortMappingParts_InAContainerAskTheNamedRouterForTheMachinesAddress.
//   - Asked to forward elsewhere (ForwardTo), the router is asked for that address and port, not the
//     machine's own: in a container on a bridge network, the machine's address at home and its
//     published HTTPS port. TestStep_AskedToForwardElsewhereMapsTheMachinesAddressAndPublishedPort.
//
// # Limits
//
//   - Only the router of the default route is asked: behind two routers (an ISP's and the
//     owner's), the outer one is not, and its own forwarding is the owner's to set.
//   - A core in a container on a bridge network cannot see the house's router, nor send SSDP's
//     group search past the machine: it asks the router the deployment names (lan.router, which the
//     install script fills in), searching it alone, and with none named it says so and maps nothing.
//     Its IPv6 addresses are its own network's, so no IPv6 is opened from there.
//
// # Read before changing
//
//   - D273 §7: mapping on by default, said on the System page, and switched off by a setting.
//   - RFC 6887 (PCP), RFC 6886 (NAT-PMP), UPnP IGD 1 and 2 (WANIPConnection,
//     WANIPv6FirewallControl).
//
// # Where to change
//
//   - portmap.go: the loop, renewals and the report; pcp.go: PCP and NAT-PMP; upnp.go: the
//     search, the description and the actions; route.go: the gateway and the IPv6 addresses.
package portmap
