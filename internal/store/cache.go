// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	_ "embed" // the cache schema
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// CacheApplicationID marks a SQLite file as our cache.db: 'VPMC'. It differs from library.db's,
// so a cache path pointed at library.db is refused rather than discarded.
const CacheApplicationID = 0x56504D43

//go:embed cache/schema.sql
var cacheSchema string

var cacheVersionLine = regexp.MustCompile(`(?m)^-- cache schema version (\d+)$`)

// ErrCacheOutdated is returned by OpenCache for a cache.db built from another schema version.
// The cache is versioned by discard (migrations.md): the caller deletes the file and opens again.
var ErrCacheOutdated = errors.New("cache.db was built by another schema version; discard it")

// Cache is cache.db: the job queue and kept match candidates, never backed up (schema.md).
type Cache struct {
	database *Database
}

// CacheVersion is the version the embedded cache schema declares.
func CacheVersion() (int64, error) {
	match := cacheVersionLine.FindStringSubmatch(cacheSchema)
	if match == nil {
		return 0, errors.New("the cache schema declares no version")
	}
	version, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("the cache schema's version: %w", err)
	}
	return version, nil
}

// OpenCache opens cache.db at path and builds its schema when the file is empty. A file of ours
// from another schema version returns ErrCacheOutdated, closed; a file not ours is refused.
func OpenCache(ctx context.Context, path string, options OpenOptions) (*Cache, error) {
	version, err := CacheVersion()
	if err != nil {
		return nil, err
	}
	database, err := open(ctx, path, options, CacheApplicationID)
	if err != nil {
		return nil, err
	}
	isEmpty, _, err := database.identity(ctx)
	if err == nil && isEmpty {
		err = database.transact(ctx, func(transaction *sql.Tx) error { return buildCache(ctx, transaction, version) })
	}
	var found int64
	if err == nil {
		err = database.readers.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found)
	}
	if err == nil && found != version {
		err = fmt.Errorf("%s: %w (it has version %d; this build's is %d)", path, ErrCacheOutdated, found, version)
	}
	if err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return &Cache{database: database}, nil
}

func buildCache(ctx context.Context, transaction *sql.Tx, version int64) error {
	for _, statement := range []string{
		fmt.Sprintf("PRAGMA application_id = %d", CacheApplicationID),
		fmt.Sprintf("PRAGMA user_version = %d", version),
		cacheSchema,
	} {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("build cache.db: %w", err)
		}
	}
	return nil
}

// Close closes cache.db.
func (cache *Cache) Close() error {
	return cache.database.Close()
}

// Read returns the cache queries over the read-only pool.
func (cache *Cache) Read() *cachegen.Queries {
	return cachegen.New(cache.database.readers)
}

// Write runs change in one write transaction on cache.db's single writer.
func (cache *Cache) Write(ctx context.Context, change func(queries *cachegen.Queries) error) error {
	return cache.database.transact(ctx, func(transaction *sql.Tx) error { return change(cachegen.New(transaction)) })
}
