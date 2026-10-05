// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Reach is which libraries a caller may browse and play (D243 §4): every one, for a role that
// administers, or those its role lists. It is read when each request is authenticated, so a
// library taken back from a role is gone at its members' next request. Its zero value reaches no
// library: a caller whose reach was never read sees nothing.
type Reach struct {
	isEveryLibrary bool
	libraryIDs     []int64
}

// EveryLibrary is the reach of a role that administers, and of the server's own work.
func EveryLibrary() Reach { return Reach{isEveryLibrary: true} }

// LibrariesOnly is a reach of the libraries listed, and no others.
func LibrariesOnly(libraryIDs ...int64) Reach {
	return Reach{libraryIDs: slices.Clone(libraryIDs)}
}

// Reaches is whether the caller may reach the library: an item with no library (nil) is reached
// only by a caller who reaches every one.
func (reach Reach) Reaches(libraryID *int64) bool {
	if reach.isEveryLibrary {
		return true
	}
	return libraryID != nil && slices.Contains(reach.libraryIDs, *libraryID)
}

// IsEveryLibrary is whether the caller reaches every library, so a query need not ask.
func (reach Reach) IsEveryLibrary() bool { return reach.isEveryLibrary }

// QueryArgs are the reach as a query's two arguments (D243 §4): is_every_library, 1 or 0, and
// library_ids_json, the libraries reached as a JSON array for its json_each.
func (reach Reach) QueryArgs() (isEveryLibrary int64, libraryIDsJSON string) {
	if reach.isEveryLibrary {
		isEveryLibrary = 1
	}
	return isEveryLibrary, reach.libraryIDsJSON()
}

// libraryIDsJSON is the libraries reached, as a JSON array.
func (reach Reach) libraryIDsJSON() string {
	encoded, err := json.Marshal(append([]int64{}, reach.libraryIDs...))
	if err != nil {
		return "[]" // a slice of integers always encodes
	}
	return string(encoded)
}

// reachOfRole is a role's reach: every library when it administers, else the libraries it lists.
func reachOfRole(ctx context.Context, queries *sqlcgen.Queries, roleID int64, capabilities []string) (Reach, error) {
	if slices.Contains(capabilities, capabilityAdminister) {
		return EveryLibrary(), nil
	}
	libraryIDs, err := queries.ListLibraryIDsOfRole(ctx, roleID)
	if err != nil {
		return Reach{}, fmt.Errorf("read role %d's libraries: %w", roleID, err)
	}
	return LibrariesOnly(libraryIDs...), nil
}

// ReachOfUser is the libraries a person reaches now, by their role (D243 §4): what Authenticate
// reads for a request, read for someone who is not the one asking, a Watch Together session's
// host. A person not there reaches none.
func (service *Service) ReachOfUser(ctx context.Context, userID int64) (Reach, error) {
	user, err := liveUser(ctx, service.database.Read(), userID)
	if errors.Is(err, ErrNoUser) {
		return Reach{}, nil
	}
	if err != nil {
		return Reach{}, err
	}
	return reachOfRole(ctx, service.database.Read(), user.RoleID, strings.Split(user.Capabilities, ","))
}
