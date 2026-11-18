// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// This file is everything the release command does beyond the folder and the key: it fetches over
// HTTPS, asks an image registry for a tag's digest, copies the published files to the server with scp,
// on the owner's own terminal since it may ask for a passphrase there, and commits and pushes the folder
// with git. It and keygen.go alone start a process (the lint's exception names both).
// Everything else asks an outside, so a test puts a fake in its place and touches no network.

// outside is the world beyond the owner's folder and key.
type outside interface {
	// Fetch is the body an HTTPS GET of address answers with 200.
	Fetch(address string) ([]byte, error)
	// Digest is the digest the registry serves for the image pull (host/repository) at tag.
	Digest(pull, tag string) (string, error)
	// Copy copies sources, files and folders, into destination, an scp target (host:path).
	Copy(sources []string, destination string) error
	// Git runs git with arguments in folder, and returns what it said when it fails.
	Git(folder string, arguments ...string) error
}

// The most a fetched record or registry answer may be: a record is a few hundred bytes.
const fetchLimit = 1 << 20

// network is the real outside.
type network struct {
	client *http.Client
	// scheme is https, and http only in this file's own test of the registry's questions.
	scheme string
}

func newNetwork() network {
	return network{client: &http.Client{Timeout: 30 * time.Second}, scheme: "https"}
}

// httpAnswer is an HTTP request's answer: its status, headers and body, at most fetchLimit.
type httpAnswer struct {
	status  int
	headers http.Header
	body    []byte
}

// get is the answer to an HTTP request.
func (world network) get(method, address string, headers map[string]string) (httpAnswer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, address, http.NoBody)
	if err != nil {
		return httpAnswer{}, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := world.client.Do(request)
	if err != nil {
		return httpAnswer{}, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, fetchLimit))
	return httpAnswer{status: response.StatusCode, headers: response.Header, body: body},
		errors.Join(err, response.Body.Close())
}

func (world network) Fetch(address string) ([]byte, error) {
	got, err := world.get(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	if got.status != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", address, got.status)
	}
	return got.body, nil
}

// The manifest kinds a multi-architecture image's digest is the digest of.
const indexKinds = "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json"

// Digest asks the registry for the tag's manifest, as docker pull does: anonymously, taking the token
// its challenge names when it answers 401, and reads the digest from Docker-Content-Digest.
func (world network) Digest(pull, tag string) (string, error) {
	host, repository, isSplit := strings.Cut(pull, "/")
	if !isSplit || repository == "" {
		return "", fmt.Errorf("%q names no registry and repository", pull)
	}
	manifest := world.scheme + "://" + host + "/v2/" + repository + "/manifests/" + url.PathEscape(tag)
	headers := map[string]string{"Accept": indexKinds}
	got, err := world.get(http.MethodHead, manifest, headers)
	if err != nil {
		return "", err
	}
	if got.status == http.StatusUnauthorized {
		token, tokenErr := world.token(got.headers.Get("Www-Authenticate"))
		if tokenErr != nil {
			return "", tokenErr
		}
		headers["Authorization"] = "Bearer " + token
		if got, err = world.get(http.MethodHead, manifest, headers); err != nil {
			return "", err
		}
	}
	if got.status != http.StatusOK {
		return "", fmt.Errorf("%s:%s: the registry answered %d", pull, tag, got.status)
	}
	digest := got.headers.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("%s:%s: the registry named no digest", pull, tag)
	}
	return digest, nil
}

// token is the anonymous pull token a registry's Bearer challenge offers.
func (world network) token(challenge string) (string, error) {
	scheme, parameters, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("the registry asks for %q, not an anonymous token", scheme)
	}
	fields := map[string]string{}
	for part := range strings.SplitSeq(parameters, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(part), "=")
		fields[strings.ToLower(name)] = strings.Trim(value, `"`)
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || fields["realm"] == "" {
		return "", fmt.Errorf("the registry's challenge names no realm: %q", challenge)
	}
	query := realm.Query()
	for _, name := range []string{"service", "scope"} {
		if fields[name] != "" {
			query.Set(name, fields[name])
		}
	}
	realm.RawQuery = query.Encode()
	body, err := world.Fetch(realm.String())
	if err != nil {
		return "", fmt.Errorf("the registry's token: %w", err)
	}
	token, err := tokenOf(body)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (network) Copy(sources []string, destination string) error {
	arguments := append(append([]string{"-r"}, sources...), destination)
	//nolint:gosec // the owner's own files and server
	return onTerminal(exec.CommandContext(context.Background(), "scp", arguments...))
}

func (network) Git(folder string, arguments ...string) error {
	//nolint:gosec // the owner's own folder
	command := exec.CommandContext(context.Background(), "git", append([]string{"-C", folder}, arguments...)...)
	said, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(said)))
	}
	return nil
}

// onTerminal runs command on the owner's terminal.
func onTerminal(command *exec.Cmd) error {
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exited, isExit := errors.AsType[*exec.ExitError](err); isExit {
			return fmt.Errorf("%s ended with %d", command.Args[0], exited.ExitCode())
		}
		return err
	}
	return nil
}
