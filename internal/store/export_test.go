// SPDX-License-Identifier: AGPL-3.0-or-later

package store

// WritesWaiting is how many writes wait for database's writer, for tests outside the package
// that need one write queued behind another.
func WritesWaiting(database *Database) int {
	database.gate.mutex.Lock()
	defer database.gate.mutex.Unlock()
	return len(database.gate.people) + len(database.gate.background)
}
