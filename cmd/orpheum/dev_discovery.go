// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/single"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// devDiscoveryPaths are where the development discovery serves its parts, on one origin: the core
// API, the client API, and its own two calls a test makes in the operator's place.
const (
	devCoreAPI   = "/core/v1"
	devClientAPI = "/client/v1"
	devClaim     = "/dev/claim"
	devCore      = "/dev/core"
)

// commandDevDiscovery is `orpheum dev discovery --listen <host:port> --state <dir>`: Orpheum's
// single-target discovery (discovery/single) and the web client it serves, on one origin, for the
// browser tier's test of a browser reaching its core through a discovery (discovery-mode.md W4b.6).
// A development build alone has it: it claims a core at a test's asking, with no operator.
func commandDevDiscovery(ctx context.Context, call invocation) int {
	flags := flag.NewFlagSet("dev discovery", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	listen := flags.String("listen", "127.0.0.1:0", "the address to serve on")
	state := flags.String("state", "", "the directory its record is kept in (required)")
	realm := flags.String("realm", "",
		"the realm its core is signed in to by, as a paid discovery's host has one (D297)")
	if err := flags.Parse(call.arguments); err != nil || *state == "" {
		return write(call.stderr, "Usage: orpheum dev discovery --listen <host:port> --state <dir> [--realm <realm>]\n",
			exitUsageError)
	}
	logger := slog.New(slog.NewTextHandler(call.stderr, nil))
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
	if err != nil {
		return call.fail(fmt.Errorf("listen on %s: %w", *listen, err))
	}
	base := "http://" + listener.Addr().String()
	clientFiles, hasClient := webclient.Files()
	if !hasClient {
		return call.fail(errors.New("this binary was built without the web client"))
	}
	handler, err := devDiscoveryHandler(devDiscoverySite{
		base: base, recordPath: filepath.Join(*state, "single.json"), realm: *realm, clientFiles: clientFiles,
		published: filepath.Join(*state, "published"),
	}, logger)
	if err != nil {
		return call.fail(errors.Join(err, listener.Close()))
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = server.Close() //nolint:errcheck // stopping
	}()
	// The browser tier's test reads the address it serves at from this line.
	fmt.Fprintf(call.stdout, "serving the discovery at %s\n", base) //nolint:errcheck // as above
	if err = server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return call.fail(err)
	}
	return exitSuccess
}

// devDiscoverySite is what the development discovery serves: at base, its record in recordPath, the
// realm its operator names (none for an owner's own), the client's files, and the root chain and
// address document it signs at its start, published into published. A development core reads no
// release record.
type devDiscoverySite struct {
	base, recordPath, realm, published string
	clientFiles                        fs.FS
}

// publishDev publishes into directory the development discovery's root chain, of one key made for this
// run alone, listed once per tier as an own discovery of one key lists it, and document under it,
// signed by that key; it returns the chain's identity, which a core enrolling by its issuer, base, is
// given (the root key audit's §3.8).
func publishDev(directory string, document addressdoc.Document) (string, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("make the development discovery's root key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return "", fmt.Errorf("make the development discovery's root key: %w", err)
	}
	text := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	root := trust.Root{
		Chain: trust.ChainDiscovery, Version: 1, Issued: document.Issued,
		Keys: []trust.RootKey{
			{Role: trust.RoleRecovery, Public: signer.PublicKey(), Text: text, Label: devKeyLabel},
			{Role: trust.RoleEveryday, Public: signer.PublicKey(), Text: text, Label: devKeyLabel},
		},
		Thresholds: map[trust.Role]int{trust.RoleEveryday: 1, trust.RoleRecovery: 1},
	}
	genesis, err := signedBy(signer, trust.NamespaceDiscoveryRoot, root.Payload)
	if err != nil {
		return "", err
	}
	document.ID = trust.IdentityOf(trust.ChainDiscovery, []byte(genesis.Payload))
	signedDocument, err := signedBy(signer, trust.NamespaceDiscoveryAddress, document.Payload)
	if err != nil {
		return "", err
	}
	files := append(trust.ChainFiles(addressdoc.RootPathPrefix, []trust.Signed{genesis}),
		trust.Published{Path: addressdoc.DocumentPath, Signed: signedDocument})
	return document.ID, trust.Publish(directory, files)
}

// devKeyLabel labels the development discovery's one key in its root statement.
const devKeyLabel = "development"

// signedBy is the payload made signed by signer in namespace.
func signedBy(signer ssh.Signer, namespace string, made func() ([]byte, error)) (trust.Signed, error) {
	payload, err := made()
	if err != nil {
		return trust.Signed{}, err
	}
	signature, err := trust.Sign(signer, namespace, payload)
	if err != nil {
		return trust.Signed{}, err
	}
	return trust.Signed{Payload: string(payload), Signature: signature}, nil
}

// devDiscoveryHandler is the development discovery at base, which the core calls: single's two
// APIs, the claim and the core's id and fingerprint a test asks for, and the loader (D308), its page
// naming its configuration, at every name the listener is reached by. Its loader is at localhost and
// each core's client origin at <coreId>.localhost, on base's port: names a browser resolves to this
// machine and takes for secure, as a service worker and crypto.subtle need (RFC 6761 §6.3).
func devDiscoveryHandler(site devDiscoverySite, logger *slog.Logger) (http.Handler, error) {
	base, realm := site.base, site.realm
	port := base[strings.LastIndex(base, ":")+1:]
	loaderOrigin := "http://localhost:" + port
	clientOrigins := "http://" + addressdoc.CorePlace + ".localhost:" + port
	now := clock.System{}.Now()
	identity, err := publishDev(site.published, addressdoc.Document{
		RootVersion: 1, Sequence: 1, Issued: now, Expires: now.Add(365 * 24 * time.Hour), Name: "Development",
		Issuer: base, Locations: []string{base}, CoreAPI: base + devCoreAPI, ClientAPI: base + devClientAPI,
		LoaderOrigin: loaderOrigin, ClientOrigins: clientOrigins,
	})
	if err != nil {
		return nil, err
	}
	store := single.FileStore{Path: site.recordPath}
	discovery, err := single.New(single.Parts{
		Identity: identity, Published: site.published, Zone: "discovery.test", Store: store,
		Now: clock.System{}.Now, Random: rand.Reader, Logger: logger, Realm: realm,
	})
	if err != nil {
		return nil, fmt.Errorf("make the discovery: %w", err)
	}
	// The client API at the page's own name, whichever it is, so no page's call is cross-origin.
	page, err := pageNaming(site.clientFiles, loaderMetas{
		clientAPI: devClientAPI, clientOrigins: clientOrigins, loaderOrigin: loaderOrigin, hasRealms: realm != "",
	})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle(trust.WellKnownPrefix, discovery.WellKnownHandler())
	mux.Handle(devCoreAPI+"/", http.StripPrefix(devCoreAPI, discovery.Handler()))
	mux.Handle(devClientAPI+"/", http.StripPrefix(devClientAPI, discovery.ClientHandler()))
	mux.HandleFunc("POST "+devClaim, func(writer http.ResponseWriter, request *http.Request) {
		var asked struct {
			Code string `json:"code"`
		}
		if strictjson.Decode(request.Body, &asked) != nil {
			http.Error(writer, "send {\"code\": ...}", http.StatusBadRequest)
			return
		}
		if claimErr := discovery.Claim(asked.Code); claimErr != nil {
			http.Error(writer, claimErr.Error(), http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET "+devCore, func(writer http.ResponseWriter, _ *http.Request) {
		record, readErr := store.Read()
		if readErr != nil || record.Core == nil {
			http.NotFound(writer, nil)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{ //nolint:errcheck // the test reads it
			"coreId": record.Core.ID, "fingerprint": proof.Fingerprint(record.Core.PublicKey),
			"state": string(record.Core.State), "isOnline": discovery.IsCoreOnline(),
		})
	})
	mux.Handle("/", page)
	return mux, nil
}

// loaderMetas are what a discovery's page tells its loader (D308): its client API, the pattern of
// its client origins, the loader's own origin, and whether it has realms.
type loaderMetas struct {
	clientAPI, clientOrigins, loaderOrigin string
	hasRealms                              bool
}

// pageNaming serves the client's files, its page naming what the loader reads in its meta elements
// (webrtc-transport.md W4c; D308), as a discovery serves it.
func pageNaming(files fs.FS, metas loaderMetas) (http.Handler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read the client's index.html: %w", err)
	}
	meta := `<head><meta name="orpheum-discovery-client-api" content="` + metas.clientAPI + `">` +
		`<meta name="orpheum-client-origins" content="` + metas.clientOrigins + `">` +
		`<meta name="orpheum-loader-origin" content="` + metas.loaderOrigin + `">`
	if metas.hasRealms {
		// The page offers a realm sign-in only where the discovery has realms (D297 §1).
		meta += `<meta name="orpheum-discovery-realms" content="true">`
	}
	index = bytes.Replace(index, []byte("<head>"), []byte(meta), 1)
	served := webclient.Handler(files, api.Prefix)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		info, statErr := fs.Stat(files, name)
		isScreen := name == "" || name == "index.html" || statErr != nil && path.Ext(name) == ""
		if !isScreen || statErr == nil && info.IsDir() && name != "" {
			served.ServeHTTP(writer, request)
			return
		}
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(writer, request, "index.html", time.Time{}, bytes.NewReader(index))
	}), nil
}
