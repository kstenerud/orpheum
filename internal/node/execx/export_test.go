// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

// CuttingDownFrom has the runner cut down every movie box of at least fromBytes, for a test of
// another package whose film is small (D512).
func (runner *Runner) CuttingDownFrom(fromBytes int64) *Runner {
	runner.cutDownFromBytes = fromBytes
	return runner
}
