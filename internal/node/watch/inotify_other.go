// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package watch

import (
	"context"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// watchKernel watches nothing where there is no inotify: every root is scanned on its schedule alone.
func watchKernel(ctx context.Context, watcher *Watcher) {
	for _, root := range watcher.roots {
		watcher.setState(ctx, nodeproto.RootWatch{
			RootPath: root, Cause: nodeproto.WatchCauseUnavailable,
			Reason: "this system has no inotify",
		})
	}
}
