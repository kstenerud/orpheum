// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The revision of each row the writes below change.
const (
	userRevision     = "SELECT revision FROM user WHERE id = 5"
	roleRevision     = "SELECT revision FROM role WHERE id = 99"
	libraryRevision  = "SELECT revision FROM library WHERE id = 2"
	overrideRevision = "SELECT revision FROM field_override WHERE item_id = 1"
	settingRevision  = "SELECT revision FROM setting WHERE key = 'revision.test'"
)

// A write to the rows revisionRows inserts, its error wrapped.
type revisionWrite func(ctx context.Context, queries *sqlcgen.Queries) error

// counted drops a write's count of rows and wraps its error.
func counted[T any](_ T, err error) error {
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// Every write to a row people edit raises its revision (sql.md M1, step 0075): each query that
// updates a field set by hand, a library, a user, a role or a setting moves it on by one, so a
// compare-and-set on the revision sees every change.
func TestRevision_EachUpdateOfAnEditedRowRaisesIt(t *testing.T) {
	t.Parallel()
	removedAt, userID, note := int64(90), int64(5), "for the den"
	for _, write := range []struct {
		name     string
		revision string
		write    revisionWrite
	}{
		{"SetAdminAway", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.SetAdminAway(ctx, sqlcgen.SetAdminAwayParams{AdminAway: "always", ID: 5}))
		}},
		{"UpdateUserPasswordHash", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.UpdateUserPasswordHash(ctx,
				sqlcgen.UpdateUserPasswordHashParams{PasswordHash: []byte{1}, ID: 5}))
		}},
		{"SetUserAdminNote", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.SetUserAdminNote(ctx, sqlcgen.SetUserAdminNoteParams{AdminNote: &note, ID: 5}))
		}},
		{"RenameUser", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.RenameUser(ctx, sqlcgen.RenameUserParams{Name: "daniel", ID: 5}))
		}},
		{"UpdateUserRole", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.UpdateUserRole(ctx, sqlcgen.UpdateUserRoleParams{RoleID: 98, ID: 5}))
		}},
		{"DemoteAdministrators", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(queries.DemoteAdministrators(ctx))
		}},
		{"RemoveUser", userRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(queries.RemoveUser(ctx, sqlcgen.RemoveUserParams{RemovedAt: &removedAt, ID: 5}))
		}},
		{"UpdateRole", roleRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.UpdateRole(ctx, sqlcgen.UpdateRoleParams{Name: "renamed", ID: 99}))
		}},
		{"SetLibraryChangeDetection", libraryRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(queries.SetLibraryChangeDetection(ctx,
				sqlcgen.SetLibraryChangeDetectionParams{ChangeDetection: "manual", ID: 2}))
		}},
		{"RemoveLibrary", libraryRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(queries.RemoveLibrary(ctx, sqlcgen.RemoveLibraryParams{RemovedAt: &removedAt, ID: 2}))
		}},
		{"UpsertOverride", overrideRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.UpsertOverride(ctx, sqlcgen.UpsertOverrideParams{
				ItemID: 1, Field: "title", OverrideValue: "Wings", SetByUserID: &userID, SetAt: 60,
			}))
		}},
		{"ClearOverridesSetByRemovedUser", overrideRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.ClearOverridesSetByRemovedUser(ctx, &userID))
		}},
		{"UpsertSetting", settingRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
				Key: "revision.test", ValueJSON: "2", SetByUserID: &userID, SetAt: 70, IsModified: 1,
			}))
		}},
		{"UpdateUnmodifiedSetting", settingRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(queries.UpdateUnmodifiedSetting(ctx,
				sqlcgen.UpdateUnmodifiedSettingParams{ValueJSON: "3", SetAt: 80, Key: "revision.test"}))
		}},
		{"ClearSettingsSetByRemovedUser", settingRevision, func(ctx context.Context, queries *sqlcgen.Queries) error {
			return counted(0, queries.ClearSettingsSetByRemovedUser(ctx, &userID))
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			t.Parallel()
			database := storetest.OpenMigrated(t)
			storetest.ExecRaw(t, database.Path(),
				"INSERT INTO role (id, name, capabilities) VALUES (99, 'admins', 'administer'), (98, 'den', '')",
				"INSERT INTO user (id, name, role_id, created_at) VALUES (5, 'dan', 99, 1)",
				`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
				 VALUES (2, 'Films', 'movie', 'tmdb', 'watch', 0, 1)`,
				"INSERT INTO item (id, kind, identification_state, added_at) VALUES (1, 'movie', 'identified', 1)",
				`INSERT INTO field_override (item_id, field, override_value, origin, set_by_user_id, set_at)
				 VALUES (1, 'title', 'Wing', 'typed', 5, 50)`,
				`INSERT INTO setting (key, value_json, set_by_user_id, set_at, is_modified)
				 VALUES ('revision.test', '1', 5, 60, 0)`)
			ctx := t.Context()
			change := func(queries *sqlcgen.Queries) error { return write.write(ctx, queries) }
			if err := database.Write(ctx, change); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(storetest.QueryRaw(t, database.Path(), write.revision), ","); got != "1" {
				t.Errorf("%s: revision %s after one write; want 1", write.name, got)
			}
		})
	}
}
