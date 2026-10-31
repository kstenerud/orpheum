// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// The ledger is the owner's own directory (a private git repository, the root key audit's §3.7):
// every statement signed, by chain and kind, each beside its .sig, and the expected file, the
// signatures the watcher checks every place against.
//
//	<ledger>/discovery/root/<n>           the discovery chain
//	<ledger>/discovery/address/<rv>-<seq>  its address documents
//	<ledger>/publisher/root/<n>           the publisher chain
//	<ledger>/publisher/release/<rv>-<seq>  its release records
//	<ledger>/drill/<issued>               drills
//	<ledger>/pending/<name>               payloads written, waiting for their signature
//	<ledger>/expected                     one line per signature: payload hash, key, counter
const expectedFile = "expected"

// ledger is the owner's ledger directory.
type ledger string

// path is name in the ledger.
func (dir ledger) path(parts ...string) string {
	return filepath.Join(append([]string{string(dir)}, parts...)...)
}

// chain is the chain of kind the ledger holds, checked from its genesis; ok false when it has none.
func (dir ledger) chain(kind trust.ChainKind) (trust.Chain, bool, error) {
	statements, err := dir.signed(string(kind), kindRoot)
	if err != nil || len(statements) == 0 {
		return trust.Chain{}, false, err
	}
	versions := map[string]int64{}
	for _, statement := range statements {
		root, parseErr := trust.ParseRoot([]byte(statement.Payload))
		if parseErr != nil {
			return trust.Chain{}, false, fmt.Errorf("the ledger's %s chain: %w", kind, parseErr)
		}
		versions[statement.Payload] = root.Version
	}
	slices.SortFunc(statements, func(one, other trust.Signed) int {
		return cmp.Compare(versions[one.Payload], versions[other.Payload])
	})
	chain, err := trust.Load(kind, "", statements)
	if err != nil {
		return trust.Chain{}, false, fmt.Errorf("the ledger's %s chain: %w", kind, err)
	}
	return chain, true, nil
}

// signed are the signed files in the ledger directory parts, in no order.
func (dir ledger) signed(parts ...string) ([]trust.Signed, error) {
	entries, err := os.ReadDir(dir.path(parts...))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the ledger: %w", err)
	}
	var found []trust.Signed
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), addressdoc.SignatureEnd) {
			continue
		}
		signed, readErr := readSigned(filepath.Join(dir.path(parts...), entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		found = append(found, signed)
	}
	return found, nil
}

// readSigned is the payload at path and its signature at path.sig.
func readSigned(path string) (trust.Signed, error) {
	payload, err := os.ReadFile(path) //nolint:gosec // the owner's own files
	if err != nil {
		return trust.Signed{}, fmt.Errorf("read %s: %w", path, err)
	}
	signature, err := os.ReadFile(path + addressdoc.SignatureEnd) //nolint:gosec // as above
	if err != nil {
		return trust.Signed{}, fmt.Errorf("read the signature %s.sig: ssh-keygen -Y sign writes it: %w", path, err)
	}
	return trust.Signed{Payload: string(payload), Signature: string(signature)}, nil
}

// keep writes signed into the ledger at parts and appends its signatures to the expected file.
func (dir ledger) keep(signed trust.Signed, parts ...string) error {
	path := dir.path(parts...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { //nolint:gosec // the owner's own ledger
		return fmt.Errorf("write the ledger: %w", err)
	}
	files := map[string]string{path: signed.Payload, path + addressdoc.SignatureEnd: signed.Signature}
	for written, contents := range files {
		if err := os.WriteFile(written, []byte(contents), 0o600); err != nil {
			return fmt.Errorf("write the ledger: %w", err)
		}
	}
	expected, err := trust.ExpectedOf(signed)
	if err != nil {
		return err
	}
	return trust.AppendExpected(dir.path(expectedFile), expected)
}

// kept is an object the ledger keeps by its root version and sequence.
type kept struct {
	signed                trust.Signed
	rootVersion, sequence int64
}

// newest is the newest object of directory parts by (root version, sequence), its name being
// "<rv>-<seq>"; zero when there is none.
func (dir ledger) newest(parts ...string) (kept, error) {
	entries, err := os.ReadDir(dir.path(parts...))
	if errors.Is(err, fs.ErrNotExist) {
		return kept{}, nil
	}
	if err != nil {
		return kept{}, fmt.Errorf("read the ledger: %w", err)
	}
	var bestName string
	var bestVersion, bestSequence int64
	for _, entry := range entries {
		version, sequence, isCut := strings.Cut(entry.Name(), "-")
		rootVersion, versionErr := strconv.ParseInt(version, 10, 64)
		count, sequenceErr := strconv.ParseInt(sequence, 10, 64)
		if !isCut || versionErr != nil || sequenceErr != nil {
			continue
		}
		if rootVersion > bestVersion || rootVersion == bestVersion && count > bestSequence {
			bestName, bestVersion, bestSequence = entry.Name(), rootVersion, count
		}
	}
	if bestName == "" {
		return kept{}, nil
	}
	signed, err := readSigned(dir.path(append(parts, bestName)...))
	return kept{signed: signed, rootVersion: bestVersion, sequence: bestSequence}, err
}

// publish writes the files every place serves from the ledger into out: the discovery chain and its
// newest address document, the publisher chain and its newest release record.
func (dir ledger) publish(out string) error {
	var files []trust.Published
	for kind, prefix := range map[trust.ChainKind]string{
		trust.ChainDiscovery: addressdoc.RootPathPrefix, trust.ChainPublisher: release.PublisherRootPathPrefix,
	} {
		chain, isHeld, err := dir.chain(kind)
		if err != nil {
			return err
		}
		if isHeld {
			files = append(files, trust.ChainFiles(prefix, chain.Statements())...)
		}
	}
	for path, parts := range map[string][]string{
		addressdoc.DocumentPath: {string(trust.ChainDiscovery), kindAddress},
		release.RecordPath:      {string(trust.ChainPublisher), kindRelease},
	} {
		newest, err := dir.newest(parts...)
		if err != nil {
			return err
		}
		if newest.signed.Payload != "" {
			files = append(files, trust.Published{Path: path, Signed: newest.signed})
		}
	}
	return trust.Publish(out, files)
}
