// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"encoding/json"
	"testing"
	"time"
)

// A refresh job's arguments are read back as refreshArgs: a person's refresh that asks for the
// missing images again says so under the name the job reads (D213 §2, D227 §6).
func TestRefreshJob_ItsArgumentsReadBackAsTheJobReadsThem(t *testing.T) {
	t.Parallel()
	for _, shouldRetryImages := range []bool{true, false} {
		request, err := RefreshJob(1, "a person asked", time.Time{}, shouldRetryImages, 0)
		if err != nil {
			t.Fatal(err)
		}
		var args refreshArgs
		if err = json.Unmarshal([]byte(request.ArgsJSON), &args); err != nil || !args.IsRefresh ||
			args.ShouldRetryImages != shouldRetryImages {
			t.Errorf("RefreshJob(retry %v) args %s read back as %+v (%v)", shouldRetryImages,
				request.ArgsJSON, args, err)
		}
	}
}
