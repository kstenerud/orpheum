// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import "testing"

// A choice whose row was retired because the cache lost its image, and never stored again, keeps
// its is_chosen, and does not refuse a new choice of the item's kind (D510 section 3, step 0089);
// two live choices of one kind are still refused.
func TestSchema_ARetiredChoiceDoesNotRefuseANewChoiceOfItsKind(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at, identification_state) VALUES (7, 'movie', 1, 'identified')",
		"INSERT INTO artwork (id, item_id, kind, source, source_ref, is_chosen, removed_at) "+
			"VALUES (1, 7, 'poster', 'tmdb', '/lost.jpg', 1, 5)",
		"INSERT INTO artwork (id, item_id, kind, source, source_ref, is_chosen) "+
			"VALUES (2, 7, 'poster', 'tmdb', '/new.jpg', 1)")
	_, err := database.writer.ExecContext(t.Context(),
		"INSERT INTO artwork (id, item_id, kind, source, source_ref, is_chosen) "+
			"VALUES (3, 7, 'poster', 'tmdb', '/another.jpg', 1)")
	if err == nil {
		t.Fatal("a second live choice of the item's posters was taken; the index must refuse it")
	}
}
