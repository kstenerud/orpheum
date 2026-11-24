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
import { createContext, use, useEffect, type Context } from "react";

import { useLatest } from "./latest.ts";

/**
 * The subtitles as their menu stands: the track showing, by key, among those offered, and their
 * timing, ms from the film, later when positive.
 */
interface ShownSubtitles {
  readonly key: string | undefined;
  readonly options: readonly SubtitleOption[];
  readonly offsetMs: number;
}

/** A remote's choice the subtitle controls carry out: a track, or their timing. */
type SubtitlesChoice = Extract<PictureChoice, { readonly kind: "subtitles" | "subtitleOffset" }>;

/** How the subtitle controls carry a remote's choice out: a track chosen, and their timing set. */
interface SubtitleWays {
  readonly choose: (key: string | undefined) => void;
  readonly setOffset: (ms: number) => void;
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
  readonly showSubtitles: (shown: ShownSubtitles | undefined, ways?: SubtitleWays) => void;
  readonly offerOriginal: (onChosen: (() => void) | undefined) => void;
  /** The way the page reads its subtitle tracks again once a remote fetched one; none to forget it. */
  readonly readFetched: (onFetched: (() => void) | undefined) => void;
}

/** A panel with no control drawn yet. */
export function newPicturePanel(): PicturePanel & PicturePanelControls {
  let subtitles: ShownSubtitles | undefined;
  let subtitleWays: SubtitleWays | undefined;
  let playOriginal: (() => void) | undefined;
  let readAgain: (() => void) | undefined;
  /** A remote's choice of a track or the timing, made by the subtitle controls where it changes either. */
  const subtitlesChosen = (choice: SubtitlesChoice): void => {
    if (choice.kind === "subtitleOffset") {
      if (choice.ms !== subtitles?.offsetMs) {
        subtitleWays?.setOffset(choice.ms);
      }
    } else if (choice.key !== subtitles?.key) {
      subtitleWays?.choose(choice.key);
    }
  };
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
            subtitleOffsetMs: subtitles?.offsetMs,
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
      } else if (choice.kind === "subtitleFetched") {
        readAgain?.();
      } else {
        subtitlesChosen(choice);
      }
    },
    showSubtitles: (shown, ways) => {
      subtitles = shown;
      subtitleWays = shown === undefined ? undefined : ways;
      changed();
    },
    readFetched: (onFetched) => {
      readAgain = onFetched;
    },
    offerOriginal: (onChosen) => {
      playOriginal = onChosen;
      changed();
    },
  };
}

/** The Play page's panel, for its controls; undefined on a page with none. */
export const PicturePanelContext: Context<(PicturePanel & PicturePanelControls) | undefined> =
  createContext<(PicturePanel & PicturePanelControls) | undefined>(undefined);

/**
 * Has the subtitle menu say how it stands, the track showing among those offered, each named as the
 * menu names it, and their timing, while it is drawn, and carry out the remote's choice of a track
 * by choose and of the timing by setOffset, each as its own press.
 */
export function useSubtitlesShownToARemote(shown: ShownSubtitles, ways: SubtitleWays): void {
  const panel = use(PicturePanelContext);
  const waysRef = useLatest(ways);
  const { key, options, offsetMs } = shown;
  useEffect(() => {
    panel?.showSubtitles(
      { key, options, offsetMs },
      {
        choose: (chosen) => {
          waysRef.current.choose(chosen);
        },
        setOffset: (ms) => {
          waysRef.current.setOffset(ms);
        },
      },
    );
  }, [panel, key, options, offsetMs]);
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

/**
 * Has the page read its subtitle tracks again by onFetched once its remote fetched one for the film
 * (Find subtitles on the remote, D224), as its own Find subtitles does after a fetch; none, nothing.
 */
export function useSubtitleFetchedByARemote(onFetched: (() => void) | undefined): void {
  const panel = use(PicturePanelContext);
  const fetchedRef = useLatest(onFetched);
  const isHeard = onFetched !== undefined;
  useEffect(() => {
    if (!isHeard) {
      return undefined;
    }
    panel?.readFetched(() => {
      fetchedRef.current?.();
    });
    return () => {
      panel?.readFetched(undefined);
    };
  }, [panel, isHeard]);
}
