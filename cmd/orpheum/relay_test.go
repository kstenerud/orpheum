// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/relaycred"
)

// relaySecretLine is a secret long enough for the relay, made up for the test.
const relaySecretLine = "a-test-secret-of-forty-bytes-or-so-0123"

// relayStage writes a relay's config, its certificate and its secrets into a directory, the
// secrets file with mode, and returns the config's path.
func relayStage(t *testing.T, secrets string, mode os.FileMode, extra string) string {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "relay.test"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4_000_000_000, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"cert.pem":    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"key.pem":     pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		"secrets.txt": []byte(secrets),
	}
	for name, text := range files {
		if err = os.WriteFile(filepath.Join(dir, name), text, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Chmod(filepath.Join(dir, "secrets.txt"), mode); err != nil {
		t.Fatal(err)
	}
	config := "name = \"relay-test\"\npublic_address = \"127.0.0.1\"\n" +
		"listen = \"127.0.0.1:0\"\nlisten_tls = \"127.0.0.1:0\"\n" +
		"relay_port_min = 50000\nrelay_port_max = 50100\n" +
		"certificate_file = \"" + filepath.Join(dir, "cert.pem") + "\"\n" +
		"key_file = \"" + filepath.Join(dir, "key.pem") + "\"\n" +
		"secrets_file = \"" + filepath.Join(dir, "secrets.txt") + "\"\n" + extra
	path := filepath.Join(dir, "relay.toml")
	if err = os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// runRelay runs `orpheum relay --config path` with its context already ended, so it starts the
// relay, closes it and returns.
func runRelay(t *testing.T, path string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	status := dispatch(ctx, []string{"relay", "--config", path}, noEnvironment, nil, &stdout, &stderr)
	return status, stderr.String()
}

// The relay starts from its config file and its secrets, one a line, each long enough.
func TestRelay_StartsFromItsConfigAndSecrets(t *testing.T) {
	t.Parallel()
	path := relayStage(t, relaySecretLine+"\n\n"+relaySecretLine+"-second\n", 0o600, "")
	if status, stderr := runRelay(t, path); status != exitSuccess {
		t.Fatalf("status %d, stderr %q; want the relay started and stopped", status, stderr)
	}
}

// The relay refuses a secrets file others may read, a secret too short, a file with no secret and
// a config key nothing reads, and never prints a secret saying so.
func TestRelay_RefusesAnOpenSecretsFileAShortSecretAndAnUnknownKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, secrets, extra, want string
		mode                       os.FileMode
	}{
		{"open to its group", relaySecretLine + "\n", "", "chmod 600", 0o640},
		{"open to all", relaySecretLine + "\n", "", "chmod 600", 0o604},
		{"a short secret", relaySecretLine + "\nshort-secret\n", "", "line 2", 0o600},
		{"no secret", "\n\n", "", "holds no secret", 0o600},
		{"an unknown key", relaySecretLine + "\n", "relay_ports = 3\n", "relay_ports", 0o600},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			status, stderr := runRelay(t, relayStage(t, each.secrets, each.mode, each.extra))
			if status == exitSuccess || !strings.Contains(stderr, each.want) {
				t.Errorf("status %d, stderr %q; want a failure saying %q", status, stderr, each.want)
			}
			if strings.Contains(stderr, relaySecretLine) || strings.Contains(stderr, "short-secret") {
				t.Errorf("stderr %q prints a secret", stderr)
			}
		})
	}
}

// Each [[announce]] table names a discovery's URL and the line of the secret it shares with the
// relay; a secret no table names is announced to no one, and a line with no secret or a URL that
// is not HTTPS is refused.
func TestRelay_AnnouncesOnlyToTheDiscoveriesItsConfigNames(t *testing.T) {
	t.Parallel()
	secrets := relaySecretLine + "\n\n" + relaySecretLine + "-second\n"
	path := relayStage(t, secrets, 0o600,
		"[[announce]]\nurl = \"https://discovery.test/relay/announce\"\nsecret_line = 3\n")
	config, _, err := loadRelayConfig(path, invocation{stderr: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Secrets) != 2 || len(config.Announce) != 1 ||
		config.Announce[0].URL != "https://discovery.test/relay/announce" {
		t.Fatalf("%d secrets, announcing to %+v; want two secrets and the one discovery named", len(config.Secrets),
			config.Announce)
	}
	username, password, err := relaycred.Make(config.Secrets[1], relaycred.Credential{
		ExpiresAt: time.Unix(4_000_000_000, 0), Role: relaycred.RoleCore, Household: relaycred.SelfTestHousehold,
		Kbps: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !relaycred.Matches(config.Announce[0].Secret, username, password) {
		t.Error("the announcement's secret is not line 3's")
	}
	for name, extra := range map[string]string{
		"a blank line": "[[announce]]\nurl = \"https://discovery.test/a\"\nsecret_line = 2\n",
		"no such line": "[[announce]]\nurl = \"https://discovery.test/a\"\nsecret_line = 9\n",
		"plain HTTP":   "[[announce]]\nurl = \"http://discovery.test/a\"\nsecret_line = 1\n",
	} {
		if status, stderr := runRelay(t, relayStage(t, secrets, 0o600, extra)); status == exitSuccess {
			t.Errorf("%s: status %d, stderr %q; want it refused", name, status, stderr)
		}
	}
}
