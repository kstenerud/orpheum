// SPDX-License-Identifier: AGPL-3.0-or-later

// Package unexpected keeps the failures of unknown cause (D511): a failure neither known to block
// its work for good nor known to pass. Each is kept in persistent.db with everything it said, its
// kind's subject is unavailable until the server next starts, and at each start the failures an
// earlier start left are tried once more by their kind's Retry. It counts them per kind, per hour,
// and says which kinds are past the setting's count (D509).
//
// A kind of work plugs in by recording through Record where it gives up for an unknown cause,
// answering IsUnavailable where it would start the work again this start, calling Resolved when the
// work succeeds, and registering, before the start's RetryEarlierStarts, the Retry that tries its
// earlier starts' failures once more: most clear the mark the kind keeps its failures by, so its
// ordinary sweep queues the work again.
//
// # Invariants
//
//   - A failure is kept with its error and output whole, the version, when and how many times, and
//     its subject is unavailable for the rest of the start. Enforced by
//     TestRecord_KeepsEverythingTheFailureSaidAndHoldsItsSubjectUnavailableThisStart.
//   - Each is logged at ERROR marked unexpected, with its kind. Enforced by
//     TestRecord_LogsTheFailureMarkedUnexpectedWithItsKind.
//   - At a start, the failures an earlier start left are tried once, by their kind's Retry, and one
//     that fails again is kept again, its count raised. Enforced by
//     TestNew_AFailureOfAnEarlierStartIsTriedOnceByItsKindsRetryAndAvailableAgain and
//     TestRetryEarlierStarts_AKindsRetryThatFailsLeavesTheOthersTried.
//   - Work that succeeds takes its failure off. Enforced by
//     TestResolved_TakesTheFailureOffAndItsSubjectIsAvailable.
//   - A kind past unknownFailuresAlertPerHour in an hour raises the alert, which clears after an hour
//     under. Enforced by TestAlerts_AKindPastTheSettingInAnHourRaisesItAndItClearsAfterAnHourUnder.
//
// # Read before changing
//
//   - D511: the three classes of failure, and what an unknown one keeps.
//   - D509: the counts, the alert, and where it shows.
//
// # Where to change
//
//   - recorder.go: recording, unavailability, the start's retry. report.go: the alert and the
//     report the Needs attention page and the export give. The job queue's give-up records through
//     it (internal/core/jobs), and internal/core/api serves the alert and the report.
package unexpected
