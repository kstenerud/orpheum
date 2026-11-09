// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// pairTimeout bounds each of pair's requests: the claim waits on the discovery's answer.
const pairTimeout = 60 * time.Second

// commandPair pairs the running core with its discovery by the key the admin's account there made:
// it signs in as the admin --name names, connects the core to the discovery (--discovery, an issuer:
// the suggested one by default) with the password unless it is connected to one, enters the key with
// the password, which makes the admin's realm key, and signs out. It says the realm, and on a line of
// its own where the discovery's loader is, which the install script tells the admin to sign in at. It
// reads the password, then the key, a line each from standard input, never flags, which the
// machine's process list would show. It asks the core over its own
// loopback, which the deployment counts as home: the install script runs it in the core's container
// (D334 §3).
func commandPair(ctx context.Context, call invocation) int {
	var name, issuer string
	config, configReport, err := call.loadConfigFromFlags("pair", kindCore, func(flags *flag.FlagSet) {
		flags.StringVar(&name, "name", "", "the admin to pair as: the account's name")
		flags.StringVar(&issuer, "discovery", discovery.Suggested().Issuer,
			"the issuer of the discovery to connect to, when the core is connected to none")
	})
	if err != nil {
		return call.fail(err)
	}
	if strings.TrimSpace(name) == "" {
		return call.fail(errors.New("--name is required: the admin to pair as"))
	}
	call.printConfigNotes(configReport)
	reader := bufio.NewReader(call.stdin)
	password, passwordErr := readLine(reader)
	key, keyErr := readLine(reader)
	if err = errors.Join(passwordErr, keyErr); err != nil || password == "" || key == "" {
		return call.fail(errors.Join(errors.New("give the password, then the key, a line each on standard input"), err))
	}
	_, port, err := net.SplitHostPort(config.Server.Listen)
	if err != nil {
		return call.fail(fmt.Errorf("server.listen %q: %w", config.Server.Listen, err))
	}
	paired, err := pair(ctx, &http.Client{Timeout: pairTimeout}, "http://127.0.0.1:"+port+api.Prefix,
		pairing{name: name, password: password, key: key, issuer: issuer})
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, paired+"\n", exitSuccess)
}

// pairing is what pair is given: the admin, their password, the account's key and the issuer of the
// discovery to connect to when the core is connected to none.
type pairing struct {
	name, password, key, issuer string
}

// pair pairs the core the API at base answers, as commandPair says, and returns the realm it is found
// by, or its state when the discovery gave none, and on a second line the loader its discovery's
// address document names, when it names one.
func pair(ctx context.Context, client *http.Client, base string, asked pairing) (string, error) {
	core := coreClient{client: client, base: base}
	var session struct {
		Token string `json:"token"`
	}
	signIn := signInBody{Name: asked.name, Password: asked.password}
	signIn.Device.Name, signIn.Device.Platform = "the install script, pairing", "script"
	if err := core.ask(ctx, http.MethodPost, "/session", signIn, &session); err != nil {
		return "", fmt.Errorf("sign in as %s: %w", asked.name, err)
	}
	core.token = session.Token
	signOut := func() error {
		return core.ask(context.WithoutCancel(ctx), http.MethodDelete, "/session", nil, nil)
	}
	var status struct {
		Issuer       string `json:"issuer"`
		State        string `json:"state"`
		Realm        string `json:"realm"`
		LoaderOrigin string `json:"loaderOrigin"`
	}
	if err := core.ask(ctx, http.MethodGet, "/system/discovery", nil, &status); err != nil {
		return "", errors.Join(fmt.Errorf("read the core's discovery: %w", err), signOut())
	}
	if status.Issuer == "" {
		err := core.ask(ctx, http.MethodPost, "/system/discovery",
			map[string]string{"issuer": asked.issuer, "password": asked.password}, &status)
		if err != nil {
			return "", errors.Join(fmt.Errorf("connect to the discovery %s: %w", asked.issuer, err), signOut())
		}
	}
	err := core.ask(ctx, http.MethodPost, "/system/discovery/claim",
		map[string]string{"key": asked.key, "password": asked.password}, &status)
	if err != nil {
		return "", errors.Join(fmt.Errorf("enter the key: %w", err), signOut())
	}
	if err = signOut(); err != nil {
		return "", fmt.Errorf("sign out: %w", err)
	}
	paired := "paired: " + status.State
	if status.Realm != "" {
		paired = "paired: realm " + status.Realm
	}
	if status.LoaderOrigin != "" {
		paired += "\n" + loaderLine + status.LoaderOrigin
	}
	return paired, nil
}

// loaderLine opens pair's line that says where the discovery's loader is, which the install script
// reads.
const loaderLine = "loader: "

// signInBody is a sign-in, as the API takes one.
type signInBody struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Device   struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
	} `json:"device"`
}

// coreClient asks the core's API at base, signed in by token once there is one.
type coreClient struct {
	client *http.Client
	base   string
	token  string
}

// ask sends body as JSON to the path, and decodes the answer into answer when it is not nil; an
// answer of 400 or more is an error carrying the API's own message.
func (core coreClient) ask(ctx context.Context, method, path string, body, answer any) error {
	var sent io.Reader = http.NoBody
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode the request: %w", err)
		}
		sent = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, core.base+path, sent)
	if err != nil {
		return fmt.Errorf("make the request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if core.token != "" {
		request.Header.Set("Authorization", "Bearer "+core.token)
	}
	response, err := core.client.Do(request)
	if err != nil {
		return fmt.Errorf("ask the core: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read in full below
	read, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read the core's answer: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		var problem struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if strictjson.Unmarshal(read, &problem) == nil && problem.Error.Message != "" {
			return errors.New(problem.Error.Message)
		}
		return fmt.Errorf("the core answered %d", response.StatusCode)
	}
	if answer == nil || len(bytes.TrimSpace(read)) == 0 {
		return nil
	}
	if err = strictjson.Unmarshal(read, answer); err != nil {
		return fmt.Errorf("decode the core's answer: %w", err)
	}
	return nil
}

// readLine is the next line of reader without its line ending; at the end, what is left.
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read standard input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
