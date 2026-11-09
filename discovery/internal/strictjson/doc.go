// SPDX-License-Identifier: AGPL-3.0-or-later

// Package strictjson decodes JSON strictly (D456), for the public discovery packages, which cannot
// import the module's internal/: strictjson.go is a twin of internal/platform/strictjson/strictjson.go,
// byte for byte, as discovery/internal/atomics is of its original (the owner, 2026-10-14: duplicate,
// and keep the copies in line). A change is made there and copied here.
//
// # Invariants
//
//   - strictjson.go is internal/platform/strictjson/strictjson.go, byte for byte.
//     TestStrictjson_IsInternalPlatformStrictjsonByteForByte.
//
// # Read before changing
//
//   - internal/platform/strictjson, whose doc.go says what the code does and what tests it.
//
// # Where to change
//
//   - internal/platform/strictjson/strictjson.go, then copied here whole.
package strictjson
