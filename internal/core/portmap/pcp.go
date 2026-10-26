// SPDX-License-Identifier: AGPL-3.0-or-later

package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// GatewayPort is where a router answers PCP and NAT-PMP (RFC 6887 §19.1, RFC 6886 §3.1).
const GatewayPort = 5351

// gatewayTries and firstWait are how often a request to the router is sent and how long the first
// waits, each wait twice the last (RFC 6886 §3.1 asks for nine tries from 250 ms; a router on the
// LAN that answers, answers in the first).
const (
	gatewayTries = 3
	firstWait    = 250 * time.Millisecond
)

// Opcodes, results and sizes of PCP (RFC 6887) and NAT-PMP (RFC 6886).
const (
	pcpVersion       = 2
	pcpOpMap         = 1
	pcpResponseBit   = 0x80
	pcpHeaderSize    = 24
	pcpMapSize       = 36
	pcpNonceSize     = 12
	natpmpVersion    = 0
	natpmpOpAddress  = 0
	natpmpOpMapTCP   = 2
	natpmpResponse   = 128
	natpmpMapSize    = 16
	natpmpAddrSize   = 12
	protocolTCP      = 6
	resultSuccess    = 0
	resultBadVersion = 1
)

// errOldProtocol is a router that answered PCP as a NAT-PMP router does: it speaks NAT-PMP alone.
var errOldProtocol = errors.New("the router speaks NAT-PMP, not PCP")

// pcpResultText is a PCP result code in words (RFC 6887 §7.4).
func pcpResultText(code byte) string {
	words := []string{
		"success", "unsupported version", "not authorized", "malformed request", "unsupported opcode",
		"unsupported option", "malformed option", "network failure", "no resources", "unsupported protocol",
		"user exceeded quota", "cannot provide external", "address mismatch", "excessive remote peers",
	}
	if int(code) < len(words) {
		return words[code]
	}
	return "an unknown result"
}

// natpmpResultText is a NAT-PMP result code in words (RFC 6886 §3.5).
func natpmpResultText(code uint16) string {
	words := []string{
		"success", "unsupported version", "not authorized or refused", "network failure", "out of resources",
		"unsupported opcode",
	}
	if int(code) < len(words) {
		return words[code]
	}
	return "an unknown result"
}

// gateway sends requests to the router's PCP and NAT-PMP port.
type gateway struct {
	address netip.AddrPort
	dialer  *net.Dialer
}

// exchange sends request to the router and returns its first answer that accept takes, sending it
// again after each wait that ends unanswered.
func (router gateway) exchange(ctx context.Context, request []byte, accept func([]byte) bool) ([]byte, error) {
	connection, err := router.dialer.DialContext(ctx, "udp", router.address.String())
	if err != nil {
		return nil, fmt.Errorf("reach the router at %s: %w", router.address, err)
	}
	defer func() { _ = connection.Close() }() //nolint:errcheck // an exchange's own socket
	wait := firstWait
	for range gatewayTries {
		answer, answerErr := sendAndWait(ctx, connection, request, wait, accept)
		if answerErr == nil {
			return answer, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		err = answerErr
		wait *= 2 // RFC 6886 §3.1's own schedule, three tries: none of backoff's cap or jitter
	}
	return nil, fmt.Errorf("the router at %s did not answer: %w", router.address, err)
}

// sendAndWait sends request once and waits up to wait for an answer accept takes.
func sendAndWait(
	ctx context.Context, connection net.Conn, request []byte, wait time.Duration, accept func([]byte) bool,
) ([]byte, error) {
	waiting, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	stopWaiting := context.AfterFunc(waiting, func() {
		_ = connection.SetReadDeadline(time.Unix(1, 0)) //nolint:errcheck // a deadline in the past wakes the read
	})
	defer stopWaiting()
	if err := connection.SetReadDeadline(time.Time{}); err != nil { // a try before may have set one
		return nil, fmt.Errorf("clear the read's deadline: %w", err)
	}
	if _, err := connection.Write(request); err != nil {
		return nil, fmt.Errorf("send to the router: %w", err)
	}
	buffer := make([]byte, 1100) // RFC 6887 §7: a PCP message is at most 1100 bytes
	for {
		count, err := connection.Read(buffer)
		if err != nil {
			return nil, fmt.Errorf("no answer within %v: %w", wait, err)
		}
		if accept(buffer[:count]) {
			return append([]byte(nil), buffer[:count]...), nil
		}
	}
}

// mapPCP asks the router by PCP to forward external TCP traffic to internal for lifetime, and
// returns the external address and port and the lifetime granted; zero lifetime removes the mapping.
// A renewal sends the mapping's nonce again (RFC 6887 §11, §15).
func (router gateway) mapPCP(
	ctx context.Context, nonce [pcpNonceSize]byte, internal netip.AddrPort, external uint16, lifetime time.Duration,
) (netip.AddrPort, time.Duration, error) {
	request := make([]byte, pcpHeaderSize+pcpMapSize)
	request[0], request[1] = pcpVersion, pcpOpMap
	binary.BigEndian.PutUint32(request[4:], uint32(lifetime.Seconds()))
	client := netip.AddrFrom16(internal.Addr().As16())
	copy(request[8:24], client.AsSlice())
	body := request[pcpHeaderSize:]
	copy(body, nonce[:])
	body[12] = protocolTCP
	binary.BigEndian.PutUint16(body[16:], internal.Port())
	binary.BigEndian.PutUint16(body[18:], external)
	anyIPv4 := netip.AddrFrom16(netip.IPv4Unspecified().As16())
	copy(body[20:36], anyIPv4.AsSlice())
	answer, err := router.exchange(ctx, request, func(answer []byte) bool {
		return isOldAnswer(answer) || (len(answer) >= pcpHeaderSize+pcpMapSize &&
			answer[1] == pcpResponseBit|pcpOpMap && string(answer[pcpHeaderSize:][:pcpNonceSize]) == string(nonce[:]))
	})
	if err != nil {
		return netip.AddrPort{}, 0, err
	}
	if isOldAnswer(answer) {
		return netip.AddrPort{}, 0, errOldProtocol
	}
	if result := answer[3]; result != resultSuccess {
		return netip.AddrPort{}, 0, fmt.Errorf("the router refused the mapping: %s (PCP %d)", pcpResultText(result),
			result)
	}
	granted := time.Duration(binary.BigEndian.Uint32(answer[4:])) * time.Second
	mapped := answer[pcpHeaderSize:]
	address := netip.AddrFrom16([16]byte(mapped[20:36])).Unmap()
	return netip.AddrPortFrom(address, binary.BigEndian.Uint16(mapped[18:])), granted, nil
}

// isOldAnswer is whether answer is a NAT-PMP router's refusal of a version it does not speak.
func isOldAnswer(answer []byte) bool {
	return len(answer) >= 4 && answer[0] == natpmpVersion &&
		binary.BigEndian.Uint16(answer[2:]) == resultBadVersion
}

// mapNATPMP asks the router by NAT-PMP to forward external TCP traffic to internal's port for
// lifetime, and returns the external address and port and the lifetime granted; zero lifetime
// removes the mapping (RFC 6886 §3.3, §3.4).
func (router gateway) mapNATPMP(
	ctx context.Context, internal netip.AddrPort, external uint16, lifetime time.Duration,
) (netip.AddrPort, time.Duration, error) {
	address, err := router.externalNATPMP(ctx)
	if err != nil {
		return netip.AddrPort{}, 0, err
	}
	request := make([]byte, natpmpAddrSize)
	request[0], request[1] = natpmpVersion, natpmpOpMapTCP
	binary.BigEndian.PutUint16(request[4:], internal.Port())
	binary.BigEndian.PutUint16(request[6:], external)
	binary.BigEndian.PutUint32(request[8:], uint32(lifetime.Seconds()))
	answer, err := router.exchange(ctx, request, func(answer []byte) bool {
		return len(answer) >= natpmpMapSize && answer[1] == natpmpResponse+natpmpOpMapTCP &&
			binary.BigEndian.Uint16(answer[8:]) == internal.Port()
	})
	if err != nil {
		return netip.AddrPort{}, 0, err
	}
	if result := binary.BigEndian.Uint16(answer[2:]); result != resultSuccess {
		return netip.AddrPort{}, 0, fmt.Errorf("the router refused the mapping: %s (NAT-PMP %d)",
			natpmpResultText(result), result)
	}
	granted := time.Duration(binary.BigEndian.Uint32(answer[12:])) * time.Second
	return netip.AddrPortFrom(address, binary.BigEndian.Uint16(answer[10:])), granted, nil
}

// externalNATPMP is the router's public IPv4 address, as NAT-PMP says it.
func (router gateway) externalNATPMP(ctx context.Context) (netip.Addr, error) {
	answer, err := router.exchange(ctx, []byte{natpmpVersion, natpmpOpAddress}, func(answer []byte) bool {
		return len(answer) >= natpmpAddrSize && answer[1] == natpmpResponse+natpmpOpAddress
	})
	if err != nil {
		return netip.Addr{}, err
	}
	if result := binary.BigEndian.Uint16(answer[2:]); result != resultSuccess {
		return netip.Addr{}, fmt.Errorf("the router would not say its address: %s (NAT-PMP %d)",
			natpmpResultText(result), result)
	}
	return netip.AddrFrom4([4]byte(answer[8:12])), nil
}
