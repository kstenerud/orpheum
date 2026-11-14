// SPDX-License-Identifier: MPL-2.0

/**
 * Whether the page needs its WebRTC connection to the core while hidden as much as when shown
 * (D482 §3): a film plays in it (isFilmPlayingOn, as its freezes are told), or it shows a cast's
 * remote. While it does not, the transport's tries are paced (the SDK's HiddenPace): one at most
 * every 10 s for two minutes hidden, then none until the page is shown again. A gap of 10 s could
 * stall a film playing on in the background, or a remote working the receiver.
 */

import { use, useEffect } from "react";

import { isFilmPlayingOn, type FilmsPage } from "./page-freezes.ts";
import { ServicesContext } from "./services.tsx";

/** What the page needs while hidden, and the mark of a cast's remote shown on it. */
export interface HiddenNeed {
  /** Whether the page needs its connection while hidden: a film plays in it, or a remote is shown. */
  readonly isNeeded: () => boolean;
  /** Marks a cast's remote shown on the page; returns its removal, which counts once. */
  readonly remoteShown: () => () => void;
}

/** The hidden need of page: its videos read as each try is weighed, its remotes as they mark. */
export function createHiddenNeed(page: FilmsPage): HiddenNeed {
  let remotes = 0;
  return {
    isNeeded: () => remotes > 0 || isFilmPlayingOn(page),
    remoteShown: () => {
      remotes += 1;
      let isRemoved = false;
      return () => {
        if (!isRemoved) {
          isRemoved = true;
          remotes -= 1;
        }
      };
    },
  };
}

/**
 * Marks the page as showing a cast's remote while the calling component is up (Services.hiddenNeed);
 * drawn outside the services, as a test's static markup, it marks nothing, as modal-layer.tsx reads
 * its navigation.
 */
export function useShowsARemote(): void {
  const hiddenNeed = use(ServicesContext)?.hiddenNeed;
  useEffect(() => hiddenNeed?.remoteShown(), [hiddenNeed]);
}
