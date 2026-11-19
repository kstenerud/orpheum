// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// ErrNoLinkPage is a discovery whose answer to a link token named no account page to send the
// admin's browser to, or one that is not https.
var ErrNoLinkPage = errors.New("the discovery named no account page to link this server at")

// LinkToken asks the discovery the core is enrolled with for a link token, which the admin's browser
// takes to the discovery's account site, where an account signed in there links the core to itself
// and sends the browser back to returnTo (D527 §2.1): returnTo is the core's page the admin pressed
// Connect on, name the server's name the approval page shows. A new token replaces the last.
func (service *Service) LinkToken(
	ctx context.Context, returnTo, name string,
) (discoveryproto.LinkTokenResponse, error) {
	status := service.Status()
	if status.BaseURL == "" {
		return discoveryproto.LinkTokenResponse{}, ErrNotEnrolled
	}
	var token discoveryproto.LinkTokenResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathLinkToken,
		body: discoveryproto.LinkTokenRequest{ReturnTo: returnTo, Name: name}, into: &token,
	})
	if err != nil {
		return discoveryproto.LinkTokenResponse{}, err
	}
	// The browser is sent there, so it is the discovery's own kind of address and nothing else.
	page, err := url.Parse(token.LinkURL)
	if err != nil || page.Scheme != "https" || page.Host == "" {
		return discoveryproto.LinkTokenResponse{}, fmt.Errorf("%w: %q", ErrNoLinkPage, token.LinkURL)
	}
	service.parts.Logger.InfoContext(ctx, "discovery: a link token was made for the admin's browser")
	return token, nil
}
