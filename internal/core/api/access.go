// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "github.com/danielgtaylor/huma/v2"

// capability is who may call an operation, named for its handler's role check (D425 §6). Every
// operation declares one where it is registered (callableBy), and the document says it as
// x-orpheum-capability beside the operation's security.
// TestArch_EveryHandlerChecksTheCallersItsOperationDeclares holds each handler to it.
type capability string

const (
	// Anyone signed in: requireSignedIn, or one of its forms that also checks the caller reaches a
	// file or an item.
	capabilitySignedIn capability = "signedIn"
	// An admin administering here: requireAdmin.
	capabilityAdmin capability = "admin"
	// A session that may change its account's security, which a device a code activated may not
	// (D292 §4): requireAccountManager.
	capabilityAccountSecurity capability = "accountSecurity"
	// A role with edit_metadata: requireMetadataEditor.
	capabilityEditMetadata capability = "editMetadata"
	// A role that may host Watch Together: requireTogetherHost.
	capabilityHostTogether capability = "hostTogether"
	// Anyone signed in while the admin offers the device tests: requireDeviceTests.
	capabilityDeviceTests capability = "deviceTests"
	// A Watch Together member, by their membership credential (D246 §3).
	capabilityMember capability = "member"
	// A byte path's caller, signed in by the header or the media cookie (D216).
	capabilityMedia capability = "media"
	// Anyone: the operation checks a secret in the request itself, a password, a code, a link's or
	// a pairing's, or holds nothing to keep from anyone.
	capabilityNone capability = "none"
)

// capabilityExtension is the operation's extension that names its capability.
const capabilityExtension = "x-orpheum-capability"

// The document's security schemes (D425 §6), any one of an operation's security sufficing.
const (
	// A session's token as a bearer token in the Authorization header (D154).
	schemeSession = "session"
	// A Watch Together member's membership credential as a bearer token in the Authorization header
	// (D246 §3); a viewer's byte paths take it in the viewer cookie or the Orpheum-Viewer header too.
	schemeMembership = "membership"
	// The media cookie that sign-in sets for the byte paths alone (D216).
	schemeMedia = "media"
)

// schemesExtension carries an operation's schemes from callableBy to declareSecurity, which takes
// it out of the document.
const schemesExtension = "orpheum-schemes"

// declareCallers has the document say who may call each operation: its schemes, and each
// operation's security from what callableBy declared.
func declareCallers(config *huma.Config) {
	config.Components.SecuritySchemes = securitySchemes()
	config.OnAddOperation = append(config.OnAddOperation, declareSecurity)
}

// securitySchemes are the document's schemes.
func securitySchemes() map[string]*huma.SecurityScheme {
	return map[string]*huma.SecurityScheme{
		schemeSession: bearerScheme("A session's token, as sign-in, a hand-off or a pairing answers it, in the " +
			"Authorization header and never in a URL (D154)."),
		schemeMembership: bearerScheme("A Watch Together member's membership credential, as opening or joining a " +
			"session answers it, in the Authorization header (D246 §3). A viewer's byte paths take it in the " +
			ViewerCookieName + " cookie or the " + ViewerHeaderName + " header too."),
		schemeMedia: {
			Type: "apiKey", In: "cookie", Name: hostOnlyPrefix + MediaCookieName,
			Description: "The media cookie sign-in sets for the byte paths alone, HttpOnly and SameSite Strict " +
				"(D216); over plain HTTP its name is " + MediaCookieName + ".",
		},
	}
}

// bearerScheme is a scheme of a bearer token in the Authorization header.
func bearerScheme(description string) *huma.SecurityScheme {
	return &huma.SecurityScheme{Type: "http", Scheme: "bearer", Description: description}
}

// callableBy is an operation's Extensions declaring who may call it: its capability and, when they
// are not the capability's own (defaultSchemes), the schemes any one of which it takes. A
// capability of none with schemes takes those or nothing.
func callableBy(declared capability, schemes ...string) map[string]any {
	if len(schemes) == 0 {
		schemes = defaultSchemes(declared)
	}
	return map[string]any{capabilityExtension: string(declared), schemesExtension: schemes}
}

// defaultSchemes are the schemes an operation of a capability takes unless it says others.
func defaultSchemes(declared capability) []string {
	switch declared {
	case capabilityNone:
		return nil
	case capabilityMember:
		return []string{schemeMembership}
	case capabilityMedia:
		return []string{schemeSession, schemeMedia}
	case capabilitySignedIn, capabilityAdmin, capabilityAccountSecurity, capabilityEditMetadata,
		capabilityHostTogether, capabilityDeviceTests:
	}
	return []string{schemeSession}
}

// declareSecurity is the OpenAPI document's hook on each operation added: it writes the operation's
// security from what callableBy declared. One declaring nothing is left without, which
// TestArch_EveryHandlerChecksTheCallersItsOperationDeclares fails.
func declareSecurity(_ *huma.OpenAPI, operation *huma.Operation) {
	declared, isDeclared := operation.Extensions[capabilityExtension].(string)
	schemes, hasSchemes := operation.Extensions[schemesExtension].([]string)
	if !isDeclared || !hasSchemes {
		return
	}
	delete(operation.Extensions, schemesExtension)
	security := []map[string][]string{}
	if capability(declared) == capabilityNone && len(schemes) > 0 {
		security = append(security, map[string][]string{}) // nothing at all, or one of the schemes
	}
	for _, scheme := range schemes {
		security = append(security, map[string][]string{scheme: {}})
	}
	operation.Security = security
}
