// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/relay"
)

const relayUsage = "usage: orpheum relay --config <file>\n"

// relayFile is the hosted relay's config file (D558): its name, its public address, its listeners,
// its relayed ports, its certificate's files and the file of its secrets.
type relayFile struct {
	Name            string `toml:"name"`
	PublicAddress   string `toml:"public_address"`
	Listen          string `toml:"listen"`
	ListenTLS       string `toml:"listen_tls"`
	RelayPortMin    uint16 `toml:"relay_port_min"`
	RelayPortMax    uint16 `toml:"relay_port_max"`
	CertificateFile string `toml:"certificate_file"`
	KeyFile         string `toml:"key_file"`
	SecretsFile     string `toml:"secrets_file"`
	LogFormat       string `toml:"log_format"`
	LogLevel        string `toml:"log_level"`
	// Announce are the discoveries the relay announces itself to (D567 §1), each naming the
	// secrets file's line of the secret it shares with the relay. A secret no entry names is
	// announced to no one.
	Announce []relayAnnounce `toml:"announce"`
}

// relayAnnounce is one `[[announce]]` table: the discovery's HTTPS URL and its secret's line.
type relayAnnounce struct {
	URL        string `toml:"url"`
	SecretLine int    `toml:"secret_line"`
}

// announceTimeout bounds one announcement's POST.
const announceTimeout = 10 * time.Second

var (
	// A secrets file others than its owner may read or write.
	errSecretsFileOpen = errors.New("others than its owner may read or write it: chmod 600 it")
	// A config naming no certificate, key or secrets file.
	errRelayFilesMissing = errors.New("certificate_file, key_file and secrets_file are required")
	// A secrets file holding no secret.
	errNoSecret = errors.New("the relay's secrets file holds no secret")
)

// commandRelay runs the hosted relay until it is told to stop.
func commandRelay(ctx context.Context, call invocation) int {
	flags := flag.NewFlagSet("orpheum relay", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	configPath := flags.String("config", "", "the relay's config file (required)")
	if err := flags.Parse(call.arguments); err != nil {
		return exitUsageError
	}
	if *configPath == "" || flags.NArg() > 0 {
		return write(call.stderr, "orpheum relay: --config is required, and nothing after the flags\n\n"+relayUsage,
			exitUsageError)
	}
	config, logger, err := loadRelayConfig(*configPath, call)
	if err != nil {
		return call.fail(err)
	}
	client, err := outbound.NewClient(announceTimeout, "relay")
	if err != nil {
		return call.fail(err)
	}
	running, err := relay.Start(ctx, config, relay.Parts{Clock: clock.System{}, Logger: logger, HTTP: client})
	if err != nil {
		return call.fail(err)
	}
	<-ctx.Done()
	if err = running.Close(); err != nil {
		logger.ErrorContext(context.WithoutCancel(ctx), "the relay did not close cleanly", "err", err)
		return exitFailure
	}
	return exitSuccess
}

// loadRelayConfig reads the relay's config file and its secrets, refusing a key it does not know.
func loadRelayConfig(path string, call invocation) (relay.Config, *slog.Logger, error) {
	file := relayFile{
		Listen: ":3478", ListenTLS: ":443", RelayPortMin: 49152, RelayPortMax: 65535,
		LogFormat: string(slogx.FormatJSON), LogLevel: "info",
	}
	metadata, err := toml.DecodeFile(path, &file)
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("read the relay's config: %w", err)
	}
	if unknown := unknownKeys(metadata); len(unknown) > 0 {
		return relay.Config{}, nil, fmt.Errorf("the relay's config has keys nothing reads: %s",
			strings.Join(unknown, ", "))
	}
	public := net.ParseIP(file.PublicAddress)
	if public == nil {
		return relay.Config{}, nil, fmt.Errorf("public_address %q is not an IP address", file.PublicAddress)
	}
	if file.CertificateFile == "" || file.KeyFile == "" || file.SecretsFile == "" {
		return relay.Config{}, nil, errRelayFilesMissing
	}
	secrets, lines, err := readRelaySecrets(file.SecretsFile)
	if err != nil {
		return relay.Config{}, nil, err
	}
	announce := make([]relay.Announce, 0, len(file.Announce))
	for _, entry := range file.Announce {
		index, isSecret := lines[entry.SecretLine]
		if !isSecret {
			return relay.Config{}, nil, fmt.Errorf("[[announce]] %s names secret_line %d, which holds no secret",
				entry.URL, entry.SecretLine)
		}
		announce = append(announce, relay.Announce{URL: entry.URL, Secret: secrets[index]})
	}
	format, err := slogx.ParseFormat(file.LogFormat)
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("the relay's config: %w", err)
	}
	level, err := parseLevel(file.LogLevel)
	if err != nil {
		return relay.Config{}, nil, err
	}
	logger, err := slogx.New(call.stderr, format, level)
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("the relay's log: %w", err)
	}
	return relay.Config{
		Name: file.Name, PublicAddress: public, Listen: file.Listen, ListenTLS: file.ListenTLS,
		RelayPortMin: file.RelayPortMin, RelayPortMax: file.RelayPortMax,
		CertificateFile: file.CertificateFile, KeyFile: file.KeyFile, Secrets: secrets, Announce: announce,
	}, logger, nil
}

// readRelaySecrets reads the secrets file: one secret a line, each at least
// relaycred.MinSecretBytes, the file readable by its owner alone. A blank line is skipped. The lines
// map each secret's line, counted from 1, to its place in secrets.
func readRelaySecrets(path string) (secrets []relaycred.Secret, lines map[int]int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read the relay's secrets: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, nil, fmt.Errorf("the relay's secrets file %s: %w", path, errSecretsFileOpen)
	}
	text, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own config
	if err != nil {
		return nil, nil, fmt.Errorf("read the relay's secrets: %w", err)
	}
	lines = map[int]int{}
	for number, line := range bytes.Split(text, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		secret, err := relaycred.NewSecret(line)
		if err != nil {
			// The error names the length wanted, never the secret.
			return nil, nil, fmt.Errorf("the relay's secrets, line %d: %w", number+1, err)
		}
		lines[number+1] = len(secrets)
		secrets = append(secrets, secret)
	}
	if len(secrets) == 0 {
		return nil, nil, errNoSecret
	}
	return secrets, lines, nil
}
