// SPDX-License-Identifier: GPL-3.0-or-later

// Package secrets holds the third-party secrets the server must present to someone else: the TMDB
// key, a user's OpenSubtitles credentials, the DNS provider token and the TURN shared secret. Each
// is sealed with XChaCha20-Poly1305 under secret.key, which never enters the database, and bound
// to its row's identity as associated data.
//
// In Go a secret is a Value. Its String, GoString, Format, MarshalJSON, MarshalText and LogValue
// all emit "[redacted]"; the only accessor is Reveal.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Only provider clients call Value.Reveal. Enforced by the forbidigo rule tagged [reveal],
//     whose path exceptions name the permitted packages.
//   - Only this package reads or writes the secret table. Enforced by the forbidigo rule tagged
//     [sealed-secret] (the generated queries are named *SealedSecret*, standards/sql.md).
//   - Nothing that prints, logs or serialises a Value shows it. Enforced by
//     TestValue_NeverShowsItsContents; across the whole server, by the canary test
//     TestCanary_ASecretAppearsNowhereButAsCiphertext in cmd/orpheum.
//   - A sealed value opens only in its own row: scope, user and name are its associated data.
//     Enforced by TestStore_ASealedValueCopiedToAnotherRowDoesNotOpen.
//   - A row sealed under another key reads as "needs re-entering", not as an error, and nothing
//     else is lost. Enforced by TestStore_AnotherKeyMarksSecretsNeedingReentry.
//
// # Read before changing
//
//   - D194: the whole design, and its canary test.
//   - D74: bug reports carry no secrets.
//   - D16: provider keys belong to the user.
//   - D190: the canary test must first be seen to fail against a leaking handler.
//   - docs/contributors/design/plans/auth.md.
//
// # Where to change
//
//   - value.go: Value and its redacting methods.
//   - key.go: the key and secret.key's format (the file lives in the state directory, D198).
//   - store.go: the registry of secrets, seal and open, and the "needs re-entering" state after a
//     restore without the key. Key rotation (orpheum secrets rotate) is not built yet.
package secrets
