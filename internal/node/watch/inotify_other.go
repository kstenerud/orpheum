// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package watch

import (
	"context"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// watchKernel watches nothing where there is no inotify: every library folder is scanned on its
// schedule alone.
func watchKernel(ctx context.Context, watcher *Watcher) {
	watcher.refuseEvery(ctx, nodeproto.WatchCauseUnavailable, "this system has no inotify")
}
