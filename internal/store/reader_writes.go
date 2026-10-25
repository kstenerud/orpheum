// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import "strings"

// ReaderWriteAllowed says whether a request that only reads may write through caller, the function
// outside the store that wrote, and why (D303, DL10). The databases storetest opens fail the test
// whose reading request writes through any other, and the server logs any other as a warning; these
// it logs at debug, since a play's GET writes through them on purpose. One list serves both, so the
// server never warns of a write its tests allow (nas.example's log, 2026-10-23).
func ReaderWriteAllowed(caller string) (why string, isAllowed bool) {
	why, isAllowed = readerWritesAllowed()[caller[strings.LastIndex(caller, "/")+1:]]
	return why, isAllowed
}

// readerWritesAllowed are the writes a reading request may make, by the function that makes them
// without its package path ("chunks.(*Service).storeIDOf"), each with why. An entry naming D303 is a
// defect the redesign removes.
func readerWritesAllowed() map[string]string {
	return map[string]string{
		"settings.(*Service).heal": "a setting row removed while the server runs is written back once, with its " +
			"starting value, the first time it is read (D202); every start writes them all",
		// A play's GET asks for the work it plays, on cache.db's writer, once a run, never once a
		// chunk: the person waits on it, and nothing else can ask for it (D234 §3, D247 §10).
		"jobqueue.(*Queue[...]).Enqueue": "a play's GET queues the run of chunks, the read-through or the " +
			"text it plays",
		"jobqueue.(*Queue[...]).CancelOpenOf": "a play's GET cancels the run of chunks a seek went past (D250 §5)",
		"chunks.(*Service).storeIDOf":         "a play's GET names the chunk store its first run makes",
		"renditions.(*Service).want":          "a play's GET asks for the prepared copy it plays",
		"renditions.(*Service).redo":          "a play's GET asks again for a prepared copy that failed",
	}
}
