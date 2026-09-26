// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Scope is whose a secret is: the server's, or one user's (schema.md, secret).
type Scope string

// The two scopes, as the secret table's CHECK names them.
const (
	ScopeServer Scope = "server"
	ScopeUser   Scope = "user"
)

// Name is a secret's name. The set is closed: every secret the server stores is declared in
// Registry with its scope, and nothing else can be set.
type Name string

// The secrets of Cut A (D194 §1).
const (
	TMDBAPIKey            Name = "tmdb_api_key" //nolint:gosec // G101: a secret's name, not a credential
	OpenSubtitlesAPIKey   Name = "opensubtitles_api_key"
	OpenSubtitlesUsername Name = "opensubtitles_username"
	OpenSubtitlesPassword Name = "opensubtitles_password"
	DNSProviderToken      Name = "dns_provider_token"
	TURNSecret            Name = "turn_secret"
)

// Definition says whose a secret is and whether it shows a hint. A hint is the last four
// characters of an API key; a password or username has none (D194 §3).
type Definition struct {
	Name     Name
	Scope    Scope
	HasHint  bool
	Describe string
}

// Registry lists every secret, in the order a settings page shows them.
func Registry() []Definition {
	return []Definition{
		{Name: TMDBAPIKey, Scope: ScopeServer, HasHint: true, Describe: "the TMDB API key metadata is fetched with"},
		{
			Name: DNSProviderToken, Scope: ScopeServer, HasHint: true,
			Describe: "the DNS provider token for the certificate",
		},
		{Name: TURNSecret, Scope: ScopeServer, HasHint: false, Describe: "the TURN shared secret voice relays use"},
		{Name: OpenSubtitlesAPIKey, Scope: ScopeUser, HasHint: true, Describe: "your OpenSubtitles API key"},
		{Name: OpenSubtitlesUsername, Scope: ScopeUser, HasHint: false, Describe: "your OpenSubtitles username"},
		{Name: OpenSubtitlesPassword, Scope: ScopeUser, HasHint: false, Describe: "your OpenSubtitles password"},
	}
}

// Lookup finds a secret's definition by name.
func Lookup(name Name) (Definition, bool) {
	for _, definition := range Registry() {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

var (
	// ErrUnknownSecret is returned for a name that is not in Registry.
	ErrUnknownSecret = errors.New("no such secret")
	// ErrWrongScope is returned when a secret is set or read in the other scope.
	ErrWrongScope = errors.New("the secret belongs to the other scope")
	// ErrNotSet is returned when a secret has no value.
	ErrNotSet = errors.New("the secret is not set")
	// ErrNeedsReentry is returned when a secret was sealed under a key this server no longer has,
	// as after a restore without secret.key. It is not an error to show; it is a state (D194 §4).
	ErrNeedsReentry = errors.New("the secret needs re-entering: it was sealed under a different secret.key")
)

// Status is what may be shown about a secret: never its value (D194 §5).
type Status struct {
	Name         Name
	Scope        Scope
	UserID       *int64
	IsSet        bool
	Hint         string
	SetBy        *int64
	SetAt        int64
	NeedsReentry bool
}

// Settings is where the store reads the hint length (D200).
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Store seals, opens and reports sealed secrets. It is the only reader or writer of the secret
// table (forbidigo [sealed-secret]).
type Store struct {
	database *store.Database
	key      Key
	clock    clock.Clock
	random   io.Reader
	settings Settings
}

// NewStore returns a Store that seals new secrets with key.
func NewStore(database *store.Database, key Key, now clock.Clock, random io.Reader, config Settings) *Store {
	return &Store{database: database, key: key, clock: now, random: random, settings: config}
}

// Set seals value and stores it for owner (nil for a server secret), replacing any old value.
func (secrets *Store) Set(ctx context.Context, name Name, owner *int64, value Value, setBy *int64) error {
	definition, err := checkScope(name, owner)
	if err != nil {
		return err
	}
	if value.IsZero() {
		return fmt.Errorf("set %s: an empty secret; clear it instead", name)
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err = io.ReadFull(secrets.random, nonce); err != nil {
		return fmt.Errorf("set %s: make a nonce: %w", name, err)
	}
	cipher, err := chacha20poly1305.NewX(secrets.key.material[:])
	if err != nil {
		return fmt.Errorf("set %s: %w", name, err)
	}
	sealed := cipher.Seal(nil, nonce, []byte(value.Reveal()), associatedData(definition.Scope, owner, name))
	var hint *string
	if definition.HasHint {
		length, lengthErr := secrets.settings.Int(ctx, settings.SecretHintChars)
		if lengthErr != nil {
			return lengthErr
		}
		hint = hintOf(value.Reveal(), length)
	}
	return secrets.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSealedSecret(ctx, sqlcgen.UpsertSealedSecretParams{
			Scope:      string(definition.Scope),
			UserID:     owner,
			Name:       string(name),
			KeyID:      secrets.key.ID(),
			Nonce:      nonce,
			Ciphertext: sealed,
			Hint:       hint,
			SetBy:      setBy,
			SetAt:      secrets.clock.Now().UnixMilli(),
		})
	})
}

// Open returns a secret's value for the client that must present it. A secret sealed under
// another key returns ErrNeedsReentry.
func (secrets *Store) Open(ctx context.Context, name Name, owner *int64) (Value, error) {
	definition, err := checkScope(name, owner)
	if err != nil {
		return Value{}, err
	}
	row, err := store.One(secrets.database.Read().GetSealedSecret(ctx, sqlcgen.GetSealedSecretParams{
		Scope: string(definition.Scope), UserID: owner, Name: string(name),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return Value{}, fmt.Errorf("%s: %w", name, ErrNotSet)
	}
	if err != nil {
		return Value{}, fmt.Errorf("read %s: %w", name, err)
	}
	if row.KeyID != secrets.key.ID() {
		return Value{}, fmt.Errorf("%s: %w", name, ErrNeedsReentry)
	}
	cipher, err := chacha20poly1305.NewX(secrets.key.material[:])
	if err != nil {
		return Value{}, fmt.Errorf("open %s: %w", name, err)
	}
	plain, err := cipher.Open(nil, row.Nonce, row.Ciphertext, associatedData(definition.Scope, owner, name))
	if err != nil {
		// The key matches but the row does not: it was copied from another row or altered.
		return Value{}, fmt.Errorf("%s: %w (the sealed value does not belong to this row)", name, ErrNeedsReentry)
	}
	return NewValue(string(plain)), nil
}

// Clear removes a secret's value; clearing one that is not set is not an error.
func (secrets *Store) Clear(ctx context.Context, name Name, owner *int64) error {
	definition, err := checkScope(name, owner)
	if err != nil {
		return err
	}
	return secrets.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.DeleteSealedSecret(ctx, sqlcgen.DeleteSealedSecretParams{
			Scope: string(definition.Scope), UserID: owner, Name: string(name),
		})
	})
}

// Statuses reports every secret of owner's scope (nil for the server), set or not.
func (secrets *Store) Statuses(ctx context.Context, owner *int64) ([]Status, error) {
	scope := ScopeServer
	if owner != nil {
		scope = ScopeUser
	}
	rows, err := secrets.database.Read().ListSealedSecretsInScope(ctx, sqlcgen.ListSealedSecretsInScopeParams{
		Scope: string(scope), UserID: owner,
	})
	if err != nil {
		return nil, fmt.Errorf("list %s secrets: %w", scope, err)
	}
	stored := map[string]sqlcgen.ListSealedSecretsInScopeRow{}
	for _, row := range rows {
		stored[row.Name] = row
	}
	statuses := []Status{}
	for _, definition := range Registry() {
		if definition.Scope != scope {
			continue
		}
		status := Status{Name: definition.Name, Scope: scope, UserID: owner}
		if row, isSet := stored[string(definition.Name)]; isSet {
			status.IsSet = true
			status.SetBy, status.SetAt = row.SetBy, row.SetAt
			status.NeedsReentry = row.KeyID != secrets.key.ID()
			if row.Hint != nil {
				status.Hint = *row.Hint
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// NeedingReentry lists every stored secret, of any scope, sealed under a key this server does
// not have: what doctor and the settings page report after a restore without secret.key.
func (secrets *Store) NeedingReentry(ctx context.Context) ([]Status, error) {
	rows, err := secrets.database.Read().ListAllSealedSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	needing := []Status{}
	for _, row := range rows {
		if row.KeyID != secrets.key.ID() {
			needing = append(needing, Status{
				Name: Name(row.Name), Scope: Scope(row.Scope), UserID: row.UserID, IsSet: true, NeedsReentry: true,
			})
		}
	}
	return needing, nil
}

// checkScope refuses an unknown name, or a server secret addressed to a user and the reverse.
func checkScope(name Name, owner *int64) (Definition, error) {
	definition, known := Lookup(name)
	if !known {
		return Definition{}, fmt.Errorf("%q: %w", name, ErrUnknownSecret)
	}
	if (definition.Scope == ScopeUser) != (owner != nil) {
		return Definition{}, fmt.Errorf("%s: %w", name, ErrWrongScope)
	}
	return definition, nil
}

// associatedData binds a ciphertext to its row's identity, so a sealed value copied into
// another row, or another user's row, fails to open (D194 §3).
func associatedData(scope Scope, owner *int64, name Name) []byte {
	user := "0"
	if owner != nil {
		user = strconv.FormatInt(*owner, 10)
	}
	return []byte(string(scope) + "\x00" + user + "\x00" + string(name))
}

// hintOf is the last length characters of an API key, or nothing when the key is too short for
// them to leave most of it hidden, or length is zero.
func hintOf(text string, length int64) *string {
	if length <= 0 || int64(utf8.RuneCountInString(text)) < 3*length {
		return nil
	}
	runes := []rune(text)
	hint := string(runes[int64(len(runes))-length:])
	return &hint
}
