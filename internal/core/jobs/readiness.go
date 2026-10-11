// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

// Readiness is how far work a person waits for is: a prepared copy, a subtitle's text.
type Readiness string

// The readinesses: being made, done, or failed.
const (
	ReadinessPreparing Readiness = "preparing"
	ReadinessComplete  Readiness = "complete"
	ReadinessFailed    Readiness = "failed"
)
