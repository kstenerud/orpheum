// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// originOf is a client origin a discovery names, as the core takes it: an https origin, or an http one
// on this machine as checkBaseURL allows, with no path, query or credentials, written
// scheme://host[:port]; anything else is none. The discovery already
// serves its client's code, so its word gives it nothing more, and this keeps the core's links from
// leading anywhere but a web origin (D293 §4).
func originOf(named string) string {
	parsed, err := url.Parse(named)
	isSecure := err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http" && isThisMachine(parsed))
	if !isSecure || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host)
}

// ErrNotADiscoveryURL is a base URL a core will not enrol with.
var ErrNotADiscoveryURL = errors.New("a discovery's address is https://, or http:// only on this machine")

// checkBaseURL refuses a base URL that is not HTTPS, but for one on this machine's loopback, so a
// claim code and a session never cross a network in the clear.
func checkBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return ErrNotADiscoveryURL
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isThisMachine(parsed) {
			return nil
		}
	}
	return ErrNotADiscoveryURL
}

// isThisMachine is whether parsed names this machine's loopback, where plain HTTP crosses no network.
func isThisMachine(parsed *url.URL) bool {
	address := net.ParseIP(parsed.Hostname())
	return address != nil && address.IsLoopback() || parsed.Hostname() == "localhost"
}
