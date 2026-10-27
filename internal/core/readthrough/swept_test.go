// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// The sweep reads the index alone first of a film with no timeline in a container whose index the
// node reads, Matroska's family and MP4's, QuickTime, F4V and 3GPP's among them, whatever the case
// of its extension; a film of any other container is read through ahead of the sweep, its keyframes
// known no other way (D269 §4, §5).
func TestSweptRead_ReadsTheIndexFirstOfEveryContainerTheNodeReadsAnIndexOf(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"/m/a.mkv", "/m/a.mk3d", "/m/a.webm", "/m/a.mp4", "/m/a.m4v", "/m/a.mov",
		"/m/a.qt", "/m/a.f4v", "/m/a.3gp", "/m/a.3g2", "/m/A.QT", "/m/A.F4V",
	} {
		read := sweptRead(sweptFile{id: 1, path: []byte(path)})
		if read.Priority != jobs.PriorityIndexFirst || !argsOf(jobs.Job{ArgsJSON: read.ArgsJSON}).IsFromIndexOnly {
			t.Errorf("%s is read at %v with %s; want its index alone first", path, read.Priority, read.ArgsJSON)
		}
	}
	for _, path := range []string{"/m/a.avi", "/m/a.ts", "/m/a.m2ts", "/m/a.mpg", "/m/a.rm", "/m/a"} {
		if read := sweptRead(sweptFile{id: 1, path: []byte(path)}); read.Priority != jobs.PriorityAheadOfSweep {
			t.Errorf("%s is read at %v; want read through ahead of the sweep", path, read.Priority)
		}
	}
}
