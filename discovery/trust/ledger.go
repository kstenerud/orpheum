// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Expected is one signature the owner's signing tool made, as its ledger keeps it: the signed
// payload's SHA-256, and the block's key fingerprint and counter. The watcher reports any signature
// served that the ledger does not hold (the root key audit's F4).
type Expected struct {
	PayloadHash, Fingerprint string
	Counter                  uint32
}

// ExpectedOf are the ledger lines for signed: one per block of its signature.
func ExpectedOf(signed Signed) ([]Expected, error) {
	blocks, err := Inspect(signed.Signature)
	if err != nil {
		return nil, err
	}
	hash := PayloadHash(signed.Payload)
	expected := make([]Expected, 0, len(blocks))
	for _, block := range blocks {
		expected = append(expected, Expected{PayloadHash: hash, Fingerprint: block.Fingerprint, Counter: block.Counter})
	}
	return expected, nil
}

// PayloadHash is payload's SHA-256, in hexadecimal.
func PayloadHash(payload string) string {
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

// line is the ledger's line for one expected signature.
func (expected Expected) line() string {
	return expected.PayloadHash + " " + expected.Fingerprint + " " + strconv.FormatUint(uint64(expected.Counter), 10)
}

// AppendExpected appends expected to the ledger file at path, made when absent.
func AppendExpected(path string, expected []Expected) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the owner's ledger
	if err != nil {
		return fmt.Errorf("open the ledger: %w", err)
	}
	for _, each := range expected {
		if _, err = file.WriteString(each.line() + "\n"); err != nil {
			_ = file.Close() //nolint:errcheck // the write failed already
			return fmt.Errorf("write the ledger: %w", err)
		}
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("write the ledger: %w", err)
	}
	return nil
}

// ReadExpected is every signature the ledger at path holds.
func ReadExpected(path string) (map[Expected]bool, error) {
	file, err := os.Open(path) //nolint:gosec // the owner's ledger
	if err != nil {
		return nil, fmt.Errorf("open the ledger: %w", err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read only
	held := map[Expected]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		counter, parseErr := strconv.ParseUint(fields[2], 10, 32)
		if parseErr != nil {
			continue
		}
		held[Expected{PayloadHash: fields[0], Fingerprint: fields[1], Counter: uint32(counter)}] = true
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("read the ledger: %w", err)
	}
	return held, nil
}
