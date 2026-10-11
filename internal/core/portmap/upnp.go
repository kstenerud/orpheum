// SPDX-License-Identifier: AGPL-3.0-or-later

package portmap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SSDPAddress is where UPnP devices listen for a search (UPnP Device Architecture 2.0 §1.3.2).
const SSDPAddress = "239.255.255.250:1900"

// searchWait is how long a search waits for the router to answer; the search asks it to answer
// within searchMX seconds.
const (
	searchWait = 2 * time.Second
	searchMX   = "1"
)

// maxDescriptionBytes bounds a router's device description and SOAP answers.
const maxDescriptionBytes = 1 << 20

// The device and service types a router's UPnP Internet Gateway Device has (IGD 1 and 2).
const (
	igdV1           = "urn:schemas-upnp-org:device:InternetGatewayDevice:1"
	igdV2           = "urn:schemas-upnp-org:device:InternetGatewayDevice:2"
	serviceIPv2     = "urn:schemas-upnp-org:service:WANIPConnection:2"
	serviceIPv1     = "urn:schemas-upnp-org:service:WANIPConnection:1"
	servicePPPv1    = "urn:schemas-upnp-org:service:WANPPPConnection:1"
	serviceFirewall = "urn:schemas-upnp-org:service:WANIPv6FirewallControl:1"
)

// onlyPermanentLeases is the error a router answers a mapping with a lease when it takes only
// permanent ones (IGD 1, WANIPConnection §2.4.16).
const onlyPermanentLeases = 725

// mappingDescription is how the core's mappings are named in the router's list.
const mappingDescription = "Orpheum"

// router is a UPnP Internet Gateway Device: where to ask it to map a port, and to open its IPv6
// firewall, each empty when it offers none.
type router struct {
	connection, connectionType string
	firewall                   string
}

// findRouter searches the LAN for gateway's UPnP Internet Gateway Device, sending the search to
// search (SSDPAddress), and reads its description.
func (service *Service) findRouter(ctx context.Context, gateway netip.Addr, search string) (router, error) {
	location, err := service.searchRouter(ctx, gateway, search)
	if err != nil {
		return router{}, err
	}
	description, err := service.fetch(ctx, location)
	if err != nil {
		return router{}, fmt.Errorf("read the router's UPnP description: %w", err)
	}
	return routerOf(description, location)
}

// searchRouter sends an SSDP search for an Internet Gateway Device and returns the description's
// location gateway answers with; an answer from another address is not the router.
func (service *Service) searchRouter(ctx context.Context, gateway netip.Addr, search string) (*url.URL, error) {
	target, err := net.ResolveUDPAddr("udp4", search)
	if err != nil {
		return nil, fmt.Errorf("read the search's address: %w", err)
	}
	connection, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("open a socket to search for the router: %w", err)
	}
	defer func() { _ = connection.Close() }() //nolint:errcheck // a search's own socket
	for _, deviceType := range []string{igdV2, igdV1} {
		request := "M-SEARCH * HTTP/1.1\r\nHOST: " + SSDPAddress + "\r\nMAN: \"ssdp:discover\"\r\nMX: " +
			searchMX + "\r\nST: " + deviceType + "\r\n\r\n"
		if _, err = connection.WriteTo([]byte(request), target); err != nil {
			return nil, fmt.Errorf("search for the router: %w", err)
		}
	}
	waiting, cancel := context.WithTimeout(ctx, searchWait)
	defer cancel()
	stopWaiting := context.AfterFunc(waiting, func() {
		_ = connection.SetReadDeadline(time.Unix(1, 0)) //nolint:errcheck // a deadline in the past wakes the read
	})
	defer stopWaiting()
	buffer := make([]byte, 2048)
	for {
		count, from, readErr := connection.ReadFrom(buffer)
		if readErr != nil {
			return nil, fmt.Errorf("no UPnP router answered within %v", searchWait)
		}
		sender, isUDP := from.(*net.UDPAddr)
		if !isUDP || sender.AddrPort().Addr().Unmap() != gateway {
			continue
		}
		if location := locationOf(buffer[:count]); location != nil {
			return location, nil
		}
	}
}

// locationOf is the description's location in a search's answer, nil for an answer that names none.
func locationOf(answer []byte) *url.URL {
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(answer)), nil)
	if err != nil {
		return nil
	}
	_ = response.Body.Close() //nolint:errcheck // an answer read whole from memory
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || location.Scheme != "http" {
		return nil
	}
	return location
}

// fetch reads a URL on the router.
func (service *Service) fetch(ctx context.Context, location *url.URL) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("make the request: %w", err)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ask the router: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the router answered %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDescriptionBytes))
	if err != nil {
		return nil, fmt.Errorf("read the router's answer: %w", err)
	}
	return body, nil
}

// deviceDescription is the part of a UPnP device description the core reads: each device's
// services and its embedded devices.
type deviceDescription struct {
	URLBase string `xml:"URLBase"`
	Device  device `xml:"device"`
}

type device struct {
	Services []struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
	} `xml:"serviceList>service"`
	Devices []device `xml:"deviceList>device"`
}

// routerOf is where a router's description says to ask it for each job, resolved against its
// location.
func routerOf(description []byte, location *url.URL) (router, error) {
	var parsed deviceDescription
	if err := xml.Unmarshal(description, &parsed); err != nil {
		return router{}, fmt.Errorf("read the router's UPnP description: %w", err)
	}
	base := location
	if parsed.URLBase != "" {
		if declared, err := url.Parse(parsed.URLBase); err == nil {
			base = declared
		}
	}
	services := map[string]string{}
	var walk func(device)
	walk = func(each device) {
		for _, offered := range each.Services {
			if _, isKnown := services[offered.ServiceType]; !isKnown {
				if control, err := base.Parse(offered.ControlURL); err == nil {
					services[offered.ServiceType] = control.String()
				}
			}
		}
		for _, embedded := range each.Devices {
			walk(embedded)
		}
	}
	walk(parsed.Device)
	found := router{firewall: services[serviceFirewall]}
	for _, connectionType := range []string{serviceIPv2, serviceIPv1, servicePPPv1} {
		if control := services[connectionType]; control != "" {
			found.connection, found.connectionType = control, connectionType
			break
		}
	}
	if found.connection == "" && found.firewall == "" {
		return router{}, errors.New("the router's UPnP offers neither port mapping nor IPv6 firewall control")
	}
	return found, nil
}

// soapArgument is one of an action's arguments, in the order the action declares them.
type soapArgument struct{ name, value string }

// upnpError is a router's refusal of an action, its UPnP error code and words.
type upnpError struct {
	code        int
	description string
}

func (refusal *upnpError) Error() string {
	return fmt.Sprintf("the router refused: %s (UPnP %d)", refusal.description, refusal.code)
}

// soapAnswer is the part of a SOAP answer the core reads: each value the action returned, and the
// UPnP error of a refusal.
type soapAnswer struct {
	Body struct {
		Inner []byte `xml:",innerxml"`
		Fault *struct {
			Code        int    `xml:"detail>UPnPError>errorCode"`
			Description string `xml:"detail>UPnPError>errorDescription"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// call runs a UPnP action on a router's service and returns the values it answered, by name.
func (service *Service) call(
	ctx context.Context, control, serviceType, action string, arguments []soapArgument,
) (map[string]string, error) {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + ` xmlns:u="` +
		serviceType + `">`)
	for _, argument := range arguments {
		body.WriteString("<" + argument.name + ">")
		if err := xml.EscapeText(&body, []byte(argument.value)); err != nil {
			return nil, fmt.Errorf("write the action's %s: %w", argument.name, err)
		}
		body.WriteString("</" + argument.name + ">")
	}
	body.WriteString("</u:" + action + "></s:Body></s:Envelope>")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, control, &body)
	if err != nil {
		return nil, fmt.Errorf("make the request: %w", err)
	}
	request.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	request.Header.Set("SOAPAction", `"`+serviceType+"#"+action+`"`)
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ask the router to %s: %w", action, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxDescriptionBytes))
	if err != nil {
		return nil, fmt.Errorf("read the router's answer to %s: %w", action, err)
	}
	var parsed soapAnswer
	if err = xml.Unmarshal(answer, &parsed); err != nil {
		return nil, fmt.Errorf("read the router's answer to %s (%s): %w", action, response.Status, err)
	}
	if fault := parsed.Body.Fault; fault != nil || response.StatusCode != http.StatusOK {
		if fault == nil {
			return nil, fmt.Errorf("the router answered %s to %s", response.Status, action)
		}
		return nil, &upnpError{code: fault.Code, description: fault.Description}
	}
	return valuesOf(parsed.Body.Inner), nil
}

// valuesOf is each element inside an action's answer, by its name, with its text.
func valuesOf(inner []byte) map[string]string {
	values := map[string]string{}
	decoder := xml.NewDecoder(bytes.NewReader(inner))
	var name string
	for {
		token, err := decoder.Token()
		if err != nil {
			return values
		}
		switch element := token.(type) {
		case xml.StartElement:
			name = element.Name.Local
		case xml.CharData:
			if name != "" {
				values[name] += string(element)
			}
		case xml.EndElement:
			name = ""
		}
	}
}

// mapUPnP asks the router to forward external TCP traffic to internal for lifetime, or for good
// when it takes only permanent mappings, and returns its public address and the port.
func (service *Service) mapUPnP(
	ctx context.Context, found router, internal netip.AddrPort, external uint16, lifetime time.Duration,
) (netip.AddrPort, time.Duration, error) {
	if found.connection == "" {
		return netip.AddrPort{}, 0, errors.New("the router's UPnP offers no port mapping")
	}
	add := func(lease time.Duration) error {
		_, err := service.call(ctx, found.connection, found.connectionType, "AddPortMapping", []soapArgument{
			{"NewRemoteHost", ""},
			{"NewExternalPort", strconv.Itoa(int(external))},
			{"NewProtocol", "TCP"},
			{"NewInternalPort", strconv.Itoa(int(internal.Port()))},
			{"NewInternalClient", internal.Addr().String()},
			{"NewEnabled", "1"},
			{"NewPortMappingDescription", mappingDescription},
			{"NewLeaseDuration", strconv.Itoa(int(lease.Seconds()))},
		})
		return err
	}
	err := add(lifetime)
	var refusal *upnpError
	if errors.As(err, &refusal) && refusal.code == onlyPermanentLeases {
		lifetime = 0
		err = add(lifetime)
	}
	if err != nil {
		return netip.AddrPort{}, 0, err
	}
	values, err := service.call(ctx, found.connection, found.connectionType, "GetExternalIPAddress", nil)
	if err != nil {
		return netip.AddrPort{}, 0, err
	}
	address, err := netip.ParseAddr(strings.TrimSpace(values["NewExternalIPAddress"]))
	if err != nil {
		return netip.AddrPort{}, 0, fmt.Errorf("the router's public address %q: %w",
			values["NewExternalIPAddress"], err)
	}
	return netip.AddrPortFrom(address, external), lifetime, nil
}

// unmapUPnP removes a mapping mapUPnP made.
func (service *Service) unmapUPnP(ctx context.Context, found router, external uint16) error {
	_, err := service.call(ctx, found.connection, found.connectionType, "DeletePortMapping", []soapArgument{
		{"NewRemoteHost", ""}, {"NewExternalPort", strconv.Itoa(int(external))}, {"NewProtocol", "TCP"},
	})
	return err
}

// openPinhole asks the router's IPv6 firewall to let TCP from anywhere reach internal for lifetime,
// and returns the pinhole's id (WANIPv6FirewallControl §2.5.4).
func (service *Service) openPinhole(
	ctx context.Context, found router, internal netip.AddrPort, lifetime time.Duration,
) (string, error) {
	if found.firewall == "" {
		return "", errors.New("the router's UPnP offers no IPv6 firewall control")
	}
	values, err := service.call(ctx, found.firewall, serviceFirewall, "AddPinhole", []soapArgument{
		{"RemoteHost", ""},
		{"RemotePort", "0"},
		{"InternalClient", internal.Addr().String()},
		{"InternalPort", strconv.Itoa(int(internal.Port()))},
		{"Protocol", strconv.Itoa(protocolTCP)},
		{"LeaseTime", strconv.Itoa(int(lifetime.Seconds()))},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(values["UniqueID"]), nil
}

// closePinhole closes a pinhole openPinhole opened.
func (service *Service) closePinhole(ctx context.Context, found router, id string) error {
	_, err := service.call(ctx, found.firewall, serviceFirewall, "DeletePinhole",
		[]soapArgument{{"UniqueID", id}})
	return err
}
