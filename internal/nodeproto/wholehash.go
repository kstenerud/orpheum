// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandHashWhole asks the node to read a video whole and say the SHA-256 of its every byte: D116's
// whole-file pass, run for a file found at a second place whose payload a file found before it has,
// to confirm it is that file's copy before the two are one file at two places (D363 §2).
const CommandHashWhole CommandType = "hashWhole"

// HashWhole is one file to read whole.
type HashWhole struct {
	// File is the file, named by payload hash and checked before it is read (D100).
	File BytesFile `json:"file"`
}

// WholeHash is the node's answer: the SHA-256 of every byte of the file, with its size and modified
// time as the read found them, or why it could not be read through.
type WholeHash struct {
	// SHA256 is the hash, 64 hex digits; empty with a failure or a refusal.
	SHA256     string `json:"sha256,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	ModifiedAt int64  `json:"modifiedAt,omitempty"`
	// Failure is why the file could not be read through: the platform's words.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not read it: a file that changed since the scan (RefusalCode).
	Refusal     string      `json:"refusal,omitempty"`
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	// Stall is set when the read stalled on its disk.
	Stall *Stall `json:"stall,omitempty"`
}
