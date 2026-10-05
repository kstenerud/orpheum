// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Capabilities is the closed set a role's capabilities come from (D130): what its members may do
// beyond browsing and playing the libraries it reaches. A role that administers reaches every
// library (D243 §4).
func Capabilities() []string {
	return []string{
		capabilityAdminister, "delete_media", "download", capabilityEditMetadata, capabilityHostWatchTogether,
		"remote_access",
	}
}

var (
	// ErrNoRole is a role that is not there.
	ErrNoRole = errors.New("no such role")
	// ErrRoleNameTaken is a role name another role has.
	ErrRoleNameTaken = errors.New("a role with that name already exists")
	// ErrBadRole is a role the admin described wrongly: no name, or a capability not in
	// Capabilities.
	ErrBadRole = errors.New("a role needs a name, and capabilities from the closed set")
	// ErrBuiltinRole is a change a built-in role does not take: its name and capabilities are
	// fixed, it cannot be deleted, and the admin role reaches every library.
	ErrBuiltinRole = errors.New("a built-in role keeps its name and capabilities, and is never deleted")
	// ErrNoLibraryToGive is a role given a library that does not exist, or one listed twice.
	ErrNoLibraryToGive = errors.New("a role is given each live library once")
	// ErrRoleHeld is a role someone holds, which cannot be deleted until no one does.
	ErrRoleHeld = errors.New("someone holds this role; give them another first")
)

// RoleLibrary is one library a role reaches, and whether its members may rebroadcast it (D243 §5,
// stored now and offered when rebroadcasting is built).
type RoleLibrary struct {
	LibraryID      int64
	MayRebroadcast bool
}

// RoleSummary is a role as the admin sees it.
type RoleSummary struct {
	ID           int64
	Name         string
	IsBuiltin    bool
	Capabilities []string
	// IsEveryLibrary is a role that administers, which reaches every library and lists none.
	IsEveryLibrary bool
	Libraries      []RoleLibrary
	// HolderCount is how many live people hold it.
	HolderCount int64
}

// RoleChange is a role as the admin describes it.
type RoleChange struct {
	Name         string
	Capabilities []string
	Libraries    []RoleLibrary
}

// ListRoles is every role, the built-in ones first, with the libraries each reaches.
func (service *Service) ListRoles(ctx context.Context) ([]RoleSummary, error) {
	queries := service.database.Read()
	roles, err := queries.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	reached, err := queries.ListRoleLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the roles' libraries: %w", err)
	}
	summaries := make([]RoleSummary, 0, len(roles))
	for _, role := range roles {
		capabilities := capabilitiesOf(role.Capabilities)
		summary := RoleSummary{
			ID: role.ID, Name: role.Name, IsBuiltin: role.IsBuiltin == 1, Capabilities: capabilities,
			IsEveryLibrary: slices.Contains(capabilities, capabilityAdminister), HolderCount: role.HolderCount,
			Libraries: []RoleLibrary{},
		}
		for _, library := range reached {
			if library.RoleID == role.ID && !summary.IsEveryLibrary {
				summary.Libraries = append(summary.Libraries, RoleLibrary{
					LibraryID: library.LibraryID, MayRebroadcast: library.MayRebroadcast == 1,
				})
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// CreateRole makes a role from change, and returns its id.
func (service *Service) CreateRole(ctx context.Context, change RoleChange) (int64, error) {
	name, capabilities, err := checkRole(change)
	if err != nil {
		return 0, err
	}
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if takenErr := refuseTakenRoleName(ctx, queries, name, 0); takenErr != nil {
			return takenErr
		}
		var insertErr error
		if id, insertErr = queries.InsertRole(ctx, sqlcgen.InsertRoleParams{
			Name: name, Capabilities: capabilities,
		}); insertErr != nil {
			return fmt.Errorf("insert role %q: %w", name, insertErr)
		}
		return setRoleLibraries(ctx, queries, id, change.Libraries)
	})
	return id, err
}

// UpdateRole changes a role as change says, its libraries taken as given: a library left out is
// taken back from its members at their next request (D243 §4). A built-in role takes only a
// change of its libraries, and the admin role, which reaches every library, none.
func (service *Service) UpdateRole(ctx context.Context, roleID int64, change RoleChange) error {
	name, capabilities, err := checkRole(change)
	if err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		role, readErr := store.OrNotFound(queries.GetRole(ctx, roleID))
		if errors.Is(readErr, store.ErrNotFound) {
			return ErrNoRole
		}
		if readErr != nil {
			return fmt.Errorf("read role %d: %w", roleID, readErr)
		}
		if role.IsBuiltin == 1 {
			return updateBuiltinRole(ctx, queries, role, name, capabilities, change.Libraries)
		}
		if takenErr := refuseTakenRoleName(ctx, queries, name, roleID); takenErr != nil {
			return takenErr
		}
		if updateErr := queries.UpdateRole(ctx, sqlcgen.UpdateRoleParams{
			ID: roleID, Name: name, Capabilities: capabilities,
		}); updateErr != nil {
			return fmt.Errorf("update role %d: %w", roleID, updateErr)
		}
		return setRoleLibraries(ctx, queries, roleID, change.Libraries)
	})
}

// updateBuiltinRole changes a built-in role's libraries, the one thing it takes: its name and
// capabilities stay, and the admin role, which reaches every library, lists none.
func updateBuiltinRole(
	ctx context.Context, queries *sqlcgen.Queries, role sqlcgen.GetRoleRow, name, capabilities string,
	libraries []RoleLibrary,
) error {
	if name != role.Name || capabilities != role.Capabilities {
		return ErrBuiltinRole
	}
	if slices.Contains(capabilitiesOf(role.Capabilities), capabilityAdminister) {
		if len(libraries) > 0 {
			return ErrBuiltinRole
		}
		return nil
	}
	return setRoleLibraries(ctx, queries, role.ID, libraries)
}

// DeleteRole deletes a role the admin made that no one holds.
func (service *Service) DeleteRole(ctx context.Context, roleID int64) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		role, err := store.OrNotFound(queries.GetRole(ctx, roleID))
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoRole
		}
		if err != nil {
			return fmt.Errorf("read role %d: %w", roleID, err)
		}
		if role.IsBuiltin == 1 {
			return ErrBuiltinRole
		}
		deleted, err := queries.DeleteRole(ctx, roleID)
		if err != nil {
			return fmt.Errorf("delete role %d: %w", roleID, err)
		}
		if deleted == 0 {
			return ErrRoleHeld
		}
		return nil
	})
}

// SetPersonsRole gives a person the role named roleName, their whole access (D243 §4): what they
// may reach changes at their next request.
func (service *Service) SetPersonsRole(ctx context.Context, userID int64, roleName string) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := liveUser(ctx, queries, userID); err != nil {
			return err
		}
		role, err := store.OrNotFound(queries.GetRoleByName(ctx, roleName))
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoRole
		}
		if err != nil {
			return fmt.Errorf("find role %q: %w", roleName, err)
		}
		if err = queries.UpdateUserRole(ctx, sqlcgen.UpdateUserRoleParams{ID: userID, RoleID: role.ID}); err != nil {
			return fmt.Errorf("give person %d role %q: %w", userID, roleName, err)
		}
		return nil
	})
}

// checkRole is a role's cleaned name and its capabilities as stored, or ErrBadRole.
func checkRole(change RoleChange) (name, capabilities string, err error) {
	name = textindex.Line(change.Name)
	if name == "" {
		return "", "", ErrBadRole
	}
	ordered := slices.Clone(change.Capabilities)
	slices.Sort(ordered)
	ordered = slices.Compact(ordered)
	for _, capability := range ordered {
		if !slices.Contains(Capabilities(), capability) {
			return "", "", fmt.Errorf("%w: %q is not a capability", ErrBadRole, capability)
		}
	}
	return name, strings.Join(ordered, ","), nil
}

// capabilitiesOf is a role's stored capabilities as a list.
func capabilitiesOf(stored string) []string {
	if stored == "" {
		return []string{}
	}
	return strings.Split(stored, ",")
}

// refuseTakenRoleName is ErrRoleNameTaken when a role other than exceptRoleID (0 for none) has
// the name.
func refuseTakenRoleName(ctx context.Context, queries *sqlcgen.Queries, name string, exceptRoleID int64) error {
	other, err := store.OrNotFound(queries.GetRoleByName(ctx, name))
	if errors.Is(err, store.ErrNotFound) || (err == nil && other.ID == exceptRoleID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look for role %q: %w", name, err)
	}
	return ErrRoleNameTaken
}

// setRoleLibraries replaces a role's libraries with those given.
func setRoleLibraries(ctx context.Context, queries *sqlcgen.Queries, roleID int64, libraries []RoleLibrary) error {
	seen := map[int64]bool{}
	for _, library := range libraries {
		if seen[library.LibraryID] {
			return fmt.Errorf("%w: library %d is listed twice", ErrNoLibraryToGive, library.LibraryID)
		}
		seen[library.LibraryID] = true
		if _, err := store.OrNotFound(queries.GetLibrary(ctx, library.LibraryID)); errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: no library %d", ErrNoLibraryToGive, library.LibraryID)
		} else if err != nil {
			return fmt.Errorf("read library %d: %w", library.LibraryID, err)
		}
	}
	if err := queries.DeleteRoleLibraries(ctx, roleID); err != nil {
		return fmt.Errorf("clear role %d's libraries: %w", roleID, err)
	}
	for _, library := range libraries {
		var mayRebroadcast int64
		if library.MayRebroadcast {
			mayRebroadcast = 1
		}
		err := queries.InsertRoleLibrary(ctx, sqlcgen.InsertRoleLibraryParams{
			RoleID: roleID, LibraryID: library.LibraryID, MayRebroadcast: mayRebroadcast,
		})
		if err != nil {
			return fmt.Errorf("give role %d library %d: %w", roleID, library.LibraryID, err)
		}
	}
	return nil
}
