// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ClientManifestBody is the core's client's manifest, signed by the core's identity key, which a
// discovery's loader checks before it runs any of the client (D308 §6).
type ClientManifestBody struct {
	// Manifest is the manifest's exact bytes, which the signature covers, in unpadded base64url: its
	// JSON is webclient.Manifest.
	Manifest string `json:"manifest"`
	// Signature is the identity key's over proof.ClientManifestMessage of those bytes.
	Signature string `json:"signature"`
	// PublicKey is the identity key's public half, which the loader believes only when its
	// fingerprint is the one its link or realm named.
	PublicKey string `json:"publicKey"`
}

// ClientManifestOutput is GET /client-manifest's answer.
type ClientManifestOutput struct {
	Body ClientManifestBody
}

func (server *Server) registerClientManifest() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getClientManifest", Method: http.MethodGet, Path: "/client-manifest",
		Summary:    "The core's client's files by their hashes, signed by its identity key (D308)",
		Tags:       []string{tagSetupAndAuth},
		Extensions: callableBy(capabilityNone),
	}, server.getClientManifest)
}

// SetClientManifest is the manifest of the client this binary serves (webclient.ManifestOf); none,
// and GET /client-manifest answers that there is no client.
func (server *Server) SetClientManifest(manifest []byte) {
	server.clientManifest = manifest
}

// getClientManifest answers the signed manifest to anyone: it says only which files the core's
// client is, which the core serves to anyone anyway.
func (server *Server) getClientManifest(_ context.Context, _ *struct{}) (*ClientManifestOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	if len(server.clientManifest) == 0 || dependencies.Discovery == nil {
		return nil, newError(http.StatusNotFound, CodeNotFound, "this server serves no client to sign")
	}
	signature, publicKey := dependencies.Discovery.SignedClientManifest(server.clientManifest)
	return &ClientManifestOutput{Body: ClientManifestBody{
		Manifest:  base64.RawURLEncoding.EncodeToString(server.clientManifest),
		Signature: base64.RawURLEncoding.EncodeToString(signature),
		PublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}}, nil
}
