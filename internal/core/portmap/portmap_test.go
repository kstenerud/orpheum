// SPDX-License-Identifier: GPL-3.0-or-later

package portmap

import (
	"context"
	"encoding/binary"
	"encoding/xml"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// The ways the fake router answers on its PCP and NAT-PMP port.
const (
	speaksPCP    = "pcp"
	speaksNATPMP = "natpmp"
)

// publicAddress is the fake router's public IPv4 address, and grantedPort the port it maps.
var publicAddress = netip.MustParseAddr("203.0.113.7")

const grantedPort = 40443

// fakeRouter is a home router on loopback: PCP or NAT-PMP on a UDP port, or neither, and UPnP's
// search answer, description and actions; it keeps what it was asked.
type fakeRouter struct {
	t      *testing.T
	speaks string
	udp    net.PacketConn
	ssdp   net.PacketConn
	upnp   *httptest.Server

	mutex sync.Mutex
	// Guarded by mutex:
	lifetimes []uint32
	nonces    [][]byte
	actions   []string
	arguments []map[string]string
}

func startRouter(t *testing.T, speaks string) *fakeRouter {
	t.Helper()
	router := &fakeRouter{t: t, speaks: speaks}
	listen := func() net.PacketConn {
		connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
		return connection
	}
	router.udp, router.ssdp = listen(), listen()
	router.upnp = httptest.NewServer(http.HandlerFunc(router.serveUPnP))
	t.Cleanup(router.upnp.Close)
	go router.serveGateway()
	go router.serveSearch()
	return router
}

// parts are a service's parts that ask the fake router, for port 443 and the IPv6 addresses given.
func (router *fakeRouter) parts(clock *clocktest.Fake, isWanted *bool, ipv6 ...netip.Addr) Parts {
	gatewayPort := udpPort(router.t, router.udp)
	if router.speaks == "" {
		gatewayPort = closedPort(router.t)
	}
	return Parts{
		HTTP: &http.Client{}, Clock: clock, Logger: slog.New(slog.DiscardHandler), Port: 443,
		IsWanted:      func(context.Context) bool { return *isWanted },
		Gateway:       func() (netip.Addr, error) { return netip.MustParseAddr("127.0.0.1"), nil },
		IPv6Addresses: func() []netip.Addr { return ipv6 },
		GatewayPort:   gatewayPort, SearchAddress: router.ssdp.LocalAddr().String(),
	}
}

// closedPort is a UDP port nothing listens on: a request there is refused at once.
func closedPort(t *testing.T) int {
	t.Helper()
	connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := udpPort(t, connection)
	_ = connection.Close() //nolint:errcheck // closed to leave the port free
	return port
}

// serveGateway answers PCP and NAT-PMP as the router speaks them.
func (router *fakeRouter) serveGateway() {
	buffer := make([]byte, 1100)
	for {
		count, from, err := router.udp.ReadFrom(buffer)
		if err != nil {
			return
		}
		if answer := router.answer(buffer[:count]); answer != nil {
			_, _ = router.udp.WriteTo(answer, from) //nolint:errcheck // the core sends again
		}
	}
}

// answer is the router's answer to one PCP or NAT-PMP request.
func (router *fakeRouter) answer(request []byte) []byte {
	router.mutex.Lock()
	defer router.mutex.Unlock()
	switch {
	case request[0] == pcpVersion && router.speaks == speaksNATPMP:
		return []byte{natpmpVersion, request[1] | natpmpResponse, 0, resultBadVersion, 0, 0, 0, 0}
	case request[0] == pcpVersion:
		router.lifetimes = append(router.lifetimes, binary.BigEndian.Uint32(request[4:]))
		router.nonces = append(router.nonces, append([]byte(nil), request[24:36]...))
		answer := make([]byte, pcpHeaderSize+pcpMapSize)
		answer[0], answer[1] = pcpVersion, pcpResponseBit|pcpOpMap
		copy(answer[4:8], request[4:8])
		copy(answer[pcpHeaderSize:], request[pcpHeaderSize:])
		binary.BigEndian.PutUint16(answer[pcpHeaderSize+18:], grantedPort)
		external := netip.AddrFrom16(publicAddress.As16())
		copy(answer[pcpHeaderSize+20:], external.AsSlice())
		return answer
	case request[1] == natpmpOpAddress:
		answer := []byte{natpmpVersion, natpmpResponse, 0, 0, 0, 0, 0, 1}
		return append(answer, publicAddress.AsSlice()...)
	default:
		router.lifetimes = append(router.lifetimes, binary.BigEndian.Uint32(request[8:]))
		answer := make([]byte, natpmpMapSize)
		answer[1] = natpmpResponse + natpmpOpMapTCP
		copy(answer[8:10], request[4:6])
		binary.BigEndian.PutUint16(answer[10:], grantedPort)
		copy(answer[12:16], request[8:12])
		return answer
	}
}

// serveSearch answers an SSDP search with where the description is.
func (router *fakeRouter) serveSearch() {
	buffer := make([]byte, 2048)
	for {
		count, from, err := router.ssdp.ReadFrom(buffer)
		if err != nil {
			return
		}
		if !strings.Contains(string(buffer[:count]), igdV1) {
			continue
		}
		answer := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=120\r\nST: " + igdV1 + "\r\nLOCATION: " +
			router.upnp.URL + "/rootDesc.xml\r\n\r\n"
		_, _ = router.ssdp.WriteTo([]byte(answer), from) //nolint:errcheck // the core searches again
	}
}

// description is the fake's device description: an IGD 1 with its connection in an embedded
// device, and IGD 2's IPv6 firewall control, as miniupnpd describes itself.
const description = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device>
<deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
<deviceList><device><deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
<serviceList><service><serviceType>urn:schemas-upnp-org:service:WANIPv6FirewallControl:1</serviceType>
<controlURL>/ctl/IP6FCtl</controlURL></service></serviceList>
<deviceList><device><deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
<serviceList><service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
<controlURL>/ctl/IPConn</controlURL></service></serviceList>
</device></deviceList></device></deviceList></device></root>`

// serveUPnP serves the description and answers each action, keeping what it was asked.
func (router *fakeRouter) serveUPnP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		_, _ = io.WriteString(writer, description) //nolint:errcheck // the core asks again
		return
	}
	action := request.Header.Get("SOAPAction")
	action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		router.t.Error(err)
	}
	router.mutex.Lock()
	router.actions = append(router.actions, action)
	router.arguments = append(router.arguments, argumentsOf(router.t, body))
	router.mutex.Unlock()
	answers := map[string]string{
		"GetExternalIPAddress": "<NewExternalIPAddress>" + publicAddress.String() + "</NewExternalIPAddress>",
		"AddPinhole":           "<UniqueID>7</UniqueID>",
	}
	answer := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` +
		`<u:` + action + `Response xmlns:u="urn:x">` + answers[action] +
		`</u:` + action + `Response></s:Body></s:Envelope>`
	_, _ = writer.Write([]byte(answer)) //nolint:errcheck,gosec // a fake's answer, to the test's own client
}

// argumentsOf is each argument of a SOAP request, by name.
func argumentsOf(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var envelope struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		t.Error(err)
	}
	return valuesOf(envelope.Body.Inner)
}

// asked are the UPnP actions the router was asked for, in order.
func (router *fakeRouter) asked() []string {
	router.mutex.Lock()
	defer router.mutex.Unlock()
	return append([]string(nil), router.actions...)
}

// lifetimesAsked are the lifetimes, in seconds, of the PCP or NAT-PMP mappings asked for.
func (router *fakeRouter) lifetimesAsked() []uint32 {
	router.mutex.Lock()
	defer router.mutex.Unlock()
	return append([]uint32(nil), router.lifetimes...)
}

// startService is a service asking router, and whether its mappings are wanted, at first yes.
func startService(t *testing.T, router *fakeRouter, ipv6 ...netip.Addr) (*Service, *clocktest.Fake, *bool) {
	t.Helper()
	clock := clocktest.NewFake(time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC))
	isWanted := true
	service, err := New(router.parts(clock, &isWanted, ipv6...))
	if err != nil {
		t.Fatal(err)
	}
	return service, clock, &isWanted
}

// A router that speaks PCP maps the port, renewed with the same nonce at half the lifetime it
// granted and not before; the discovery is told the public address; once the mapping is no longer
// wanted it is removed (RFC 6887 §11, §15; D273 §7).
func TestStep_MapsByPCPRenewsAtHalfItsLifetimeAndRemovesIt(t *testing.T) {
	t.Parallel()
	router := startRouter(t, speaksPCP)
	service, clock, isWanted := startService(t, router)
	ctx := t.Context()
	service.step(ctx)
	want := []discoveryproto.Candidate{{
		Address: publicAddress.String(), Port: grantedPort, Source: discoveryproto.SourceMapped, Method: MethodPCP,
	}}
	if got := service.Candidates(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("candidates = %+v; want %+v", got, want)
	}
	clock.Advance(29 * time.Minute)
	service.step(ctx)
	if asked := router.lifetimesAsked(); len(asked) != 1 {
		t.Fatalf("before half its lifetime the mapping was asked for %d times; want once", len(asked))
	}
	clock.Advance(2 * time.Minute)
	service.step(ctx)
	*isWanted = false
	service.step(ctx)
	if asked := router.lifetimesAsked(); len(asked) != 3 || asked[1] != 3600 || asked[2] != 0 {
		t.Errorf("lifetimes asked = %v; want 3600, a renewal of 3600, then 0 to remove it", asked)
	}
	if string(router.nonces[1]) != string(router.nonces[0]) {
		t.Errorf("the renewal's nonce differs from the mapping's; want it sent again")
	}
	if report := service.Report(); report.IsWanted || len(report.Mappings) != 0 || len(service.Candidates()) != 0 {
		t.Errorf("after it is no longer wanted the report is %+v; want nothing mapped", report)
	}
}

// A router that answers PCP as NAT-PMP routers do is asked by NAT-PMP (RFC 6887 §9).
func TestStep_AsksByNATPMPWhenTheRouterSpeaksOnlyIt(t *testing.T) {
	t.Parallel()
	router := startRouter(t, speaksNATPMP)
	service, _, _ := startService(t, router)
	service.step(t.Context())
	report := service.Report()
	if len(report.Mappings) != 1 || report.Mappings[0].Method != MethodNATPMP ||
		report.Mappings[0].External != netip.AddrPortFrom(publicAddress, grantedPort) {
		t.Errorf("report = %+v; want one NAT-PMP mapping of %s:%d", report, publicAddress, grantedPort)
	}
}

// A router that speaks neither PCP nor NAT-PMP is found by UPnP: it forwards its port and opens its
// IPv6 firewall to each of the machine's global addresses, and releasing closes both, as the core
// does when it stops.
func TestStep_OpensByUPnPForIPv4AndIPv6AndReleaseClosesThem(t *testing.T) {
	t.Parallel()
	router := startRouter(t, "")
	ipv6 := netip.MustParseAddr("2a0f:1000:1::20")
	service, _, _ := startService(t, router, ipv6)
	service.step(t.Context())
	report := service.Report()
	if len(report.Mappings) != 2 || report.Mappings[0].Method != MethodUPnP ||
		report.Mappings[0].External != netip.AddrPortFrom(publicAddress, 443) ||
		report.Mappings[1].External != netip.AddrPortFrom(ipv6, 443) {
		t.Errorf("report = %+v; want the port forwarded and the IPv6 firewall opened, by UPnP", report)
	}
	if len(report.Problems) == 0 || !strings.HasPrefix(report.Problems[0], "pcp: ") {
		t.Errorf("problems = %q; want PCP's failure said", report.Problems)
	}
	service.release(t.Context())
	want := []string{"AddPortMapping", "GetExternalIPAddress", "AddPinhole", "DeletePortMapping", "DeletePinhole"}
	if got := router.asked(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the router was asked %v; want %v", got, want)
	}
	router.mutex.Lock()
	defer router.mutex.Unlock()
	if added := router.arguments[0]; added["NewInternalClient"] != "127.0.0.1" || added["NewInternalPort"] != "443" ||
		added["NewProtocol"] != "TCP" {
		t.Errorf("AddPortMapping was given %v; want 127.0.0.1:443 over TCP", added)
	}
	if pinhole := router.arguments[2]; pinhole["InternalClient"] != ipv6.String() || pinhole["Protocol"] != "6" {
		t.Errorf("AddPinhole was given %v; want %s over TCP", pinhole, ipv6)
	}
	if closed := router.arguments[4]; closed["UniqueID"] != "7" {
		t.Errorf("DeletePinhole was given %v; want the pinhole's id", closed)
	}
}

// The gateway is read from the default route of Linux's routing table, whose addresses are hex,
// least significant byte first.
func TestGatewayOf_ReadsTheDefaultRoutesGateway(t *testing.T) {
	t.Parallel()
	table := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t006FA8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
		"eth0\t00000000\t016FA8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n"
	if gateway, err := GatewayOf(table); err != nil || gateway != netip.MustParseAddr("192.168.1.1") {
		t.Errorf("gatewayOf = %v, %v; want 192.168.1.1", gateway, err)
	}
	if _, err := GatewayOf(strings.Split(table, "\n")[0] + "\n"); err == nil {
		t.Error("a table with no default route gave a gateway")
	}
}

// udpPort is the port a UDP socket listens on.
func udpPort(t *testing.T, connection net.PacketConn) int {
	t.Helper()
	address, isUDP := connection.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		t.Fatalf("%v is not a UDP address", connection.LocalAddr())
	}
	return address.Port
}
