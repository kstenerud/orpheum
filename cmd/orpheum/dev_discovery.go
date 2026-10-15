// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"bytes"
	"context"
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

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/single"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/webclient"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
	handler, err := devDiscoveryHandler(base, filepath.Join(*state, "single.json"), *realm, logger)
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

// devDiscoveryHandler is the development discovery at base: single's two APIs, the claim and the
// core's id and fingerprint a test asks for, and the client, its page naming the client API.
func devDiscoveryHandler(base, recordPath, realm string, logger *slog.Logger) (http.Handler, error) {
	store := single.FileStore{Path: recordPath}
	discovery, err := single.New(single.Parts{
		BaseURL: base + devCoreAPI, Zone: "discovery.test", ClientOrigin: base, Store: store,
		Now: clock.System{}.Now, Random: rand.Reader, Logger: logger, Realm: realm,
	})
	if err != nil {
		return nil, fmt.Errorf("make the discovery: %w", err)
	}
	clientFiles, hasClient := webclient.Files()
	if !hasClient {
		return nil, errors.New("this binary was built without the web client")
	}
	page, err := pageNaming(clientFiles, base+devClientAPI, realm != "")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle(devCoreAPI+"/", http.StripPrefix(devCoreAPI, discovery.Handler()))
	mux.Handle(devClientAPI+"/", http.StripPrefix(devClientAPI, discovery.ClientHandler()))
	mux.HandleFunc("POST "+devClaim, func(writer http.ResponseWriter, request *http.Request) {
		var asked struct {
			Code string `json:"code"`
		}
		if json.NewDecoder(request.Body).Decode(&asked) != nil {
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

// pageNaming serves the client's files, its page naming clientAPI in the meta element the client
// reads (webrtc-transport.md W4c), as a discovery serves it.
func pageNaming(files fs.FS, clientAPI string, hasRealms bool) (http.Handler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read the client's index.html: %w", err)
	}
	meta := `<head><meta name="orpheum-discovery-client-api" content="` + clientAPI + `">`
	if hasRealms {
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
