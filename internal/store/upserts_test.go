// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var (
	generatedQuery = regexp.MustCompile("(?s)const \\w+ = `-- name: (\\w+) :\\w+\\n(.*?)`")
	doUpdate       = regexp.MustCompile(`(?is)\bdo\s+update\b`)
	guardedUpdate  = regexp.MustCompile(`(?is)\bdo\s+update\b.*\bwhere\b`)
	optedOut       = regexp.MustCompile(`@sqlc-vet-disable upsert-only-if-changed`)
)

// Every upsert the store runs, as generated, writes only when it changes something, unless its
// query opts out and says why (D303, DL9): sqlc vet refuses an unguarded one when it generates,
// and this reads what it generated.
func TestUpserts_EachWritesOnlyWhenItChangesSomething(t *testing.T) {
	t.Parallel()
	generated, err := filepath.Glob(filepath.Join("*gen", "*.sql.go"))
	if err != nil || len(generated) == 0 {
		t.Fatalf("no generated queries: %v", err)
	}
	exempt := upsertsOptedOut(t)
	upserts := 0
	for _, path := range generated {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range generatedQuery.FindAllSubmatch(text, -1) {
			name, sql := string(match[1]), match[2]
			if !doUpdate.Match(sql) || exempt[name] {
				continue
			}
			upserts++
			if !guardedUpdate.Match(sql) {
				t.Errorf("%s (%s) updates on conflict with no guard: it rewrites the row when nothing changed",
					name, path)
			}
		}
	}
	if upserts == 0 {
		t.Error("no upsert found; the test reads nothing")
	}
}

// upsertsOptedOut are the queries whose comment opts their upsert out of the guard, with why.
func upsertsOptedOut(t *testing.T) map[string]bool {
	t.Helper()
	exempt := map[string]bool{}
	for _, directory := range []string{"queries", filepath.Join("cache", "queries")} {
		for _, query := range queriesIn(t, directory) {
			if optedOut.MatchString(query.comment) {
				exempt[query.name] = true
			}
		}
	}
	return exempt
}
