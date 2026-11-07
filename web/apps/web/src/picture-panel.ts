// SPDX-License-Identifier: MPL-2.0

/**
 * What the Play page shows that its person may choose, its subtitles and the original's offer, as
 * one panel a cast's remote reads and works (playback-picture.ts in the SDK; D440 §5), beside the
 * sound's (sound-panel.ts): the subtitle menu and the original's offer each say here how they stand,
 * which the receiver tells its session for the remote to show, and each carries out the remote's
 * choice of it as its own press. A Fire TV's own menus are hidden while it is controlled (D407 §12),
 * so its subtitles could only be changed after taking control back.
 */

import type { PictureChoice, PlaybackPicture, SubtitleOption } from "@orpheum/sdk";
import { createContext, use, useEffect } from "react";

import { useLatest } from "./latest.ts";

/** The subtitles as their menu stands: the track showing, by key, among those offered. */
interface ShownSubtitles {
  readonly key: string | undefined;
  readonly options: readonly SubtitleOption[];
}

/** The page's picture controls, as one panel. */
export interface PicturePanel {
  /** What the film shows now, as its controls stand; undefined while none is drawn. */
  readonly read: () => PlaybackPicture | undefined;
  /** Calls listener at each change of how the controls stand; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Hands a remote's choice to the control it is of. */
  readonly choose: (choice: PictureChoice) => void;
}

/** What the controls do with the panel: say how they stand, and the way each carries a choice out. */
interface PicturePanelControls {
  readonly showSubtitles: (
    shown: ShownSubtitles | undefined,
    choose?: (key: string | undefined) => void,
  ) => void;
  readonly offerOriginal: (onChosen: (() => void) | undefined) => void;
}

/** A panel with no control drawn yet. */
export function newPicturePanel(): PicturePanel & PicturePanelControls {
  let subtitles: ShownSubtitles | undefined;
  let chooseSubtitles: ((key: string | undefined) => void) | undefined;
  let playOriginal: (() => void) | undefined;
  const listeners = new Set<() => void>();
  const changed = (): void => {
    for (const listener of listeners) {
      listener();
    }
  };
  return {
    read: () =>
      subtitles === undefined && playOriginal === undefined
        ? undefined
        : {
            subtitleKey: subtitles?.key,
            subtitles: subtitles?.options ?? [],
            canPlayOriginal: playOriginal !== undefined,
          },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    choose: (choice) => {
      if (choice.kind === "original") {
        playOriginal?.();
      } else if (choice.key !== subtitles?.key) {
        chooseSubtitles?.(choice.key);
      }
    },
    showSubtitles: (shown, choose) => {
      subtitles = shown;
      chooseSubtitles = shown === undefined ? undefined : choose;
      changed();
    },
    offerOriginal: (onChosen) => {
      playOriginal = onChosen;
      changed();
    },
  };
}

/** The Play page's panel, for its controls; undefined on a page with none. */
export const PicturePanelContext = createContext<(PicturePanel & PicturePanelControls) | undefined>(
  undefined,
);

/**
 * Has the subtitle menu say how it stands, the track showing among those offered, each named as the
 * menu names it, while it is drawn, and carry out the remote's choice by choose, as its own press.
 */
export function useSubtitlesShownToARemote(
  key: string | undefined,
  options: readonly SubtitleOption[],
  choose: (key: string | undefined) => void,
): void {
  const panel = use(PicturePanelContext);
  const chooseRef = useLatest(choose);
  useEffect(() => {
    panel?.showSubtitles({ key, options }, (chosen) => {
      chooseRef.current(chosen);
    });
  }, [panel, key, options]);
  useEffect(
    () => () => {
      panel?.showSubtitles(undefined);
    },
    [panel],
  );
}

/**
 * Has the original's offer say it is offered while it is drawn, and play the film's own file by
 * onChosen at the remote's choice, as its own press.
 */
export function useOriginalOfferedToARemote(onChosen: () => void): void {
  const panel = use(PicturePanelContext);
  const chosenRef = useLatest(onChosen);
  useEffect(() => {
    panel?.offerOriginal(() => {
      chosenRef.current();
    });
    return () => {
      panel?.offerOriginal(undefined);
    };
  }, [panel]);
}
