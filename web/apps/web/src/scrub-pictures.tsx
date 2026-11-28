// SPDX-License-Identifier: MPL-2.0

/**
 * A film's scrub-bar pictures (trickplay.md §5, phase F3): the pictures its nodes made, every
 * interval of the film, in sheets of columns × rows, which a timeline shows above itself at the
 * place pointed at, dragged to or stepped to (Timeline). They are read as the player opens
 * (GET /files/{id}/trickplay) and, while not every sheet is made, again as the film plays
 * (scrubPicturesReadAgainEveryMs); their sheets are loaded only once the film has started playing, so
 * they never compete with its first seconds of buffering; over WebRTC each sheet waits on the
 * cosmetic rung besides, below the film's bytes (rungOf, D479). A film with none made, or a stretch
 * whose sheet is not made yet, shows the time alone, as before.
 */

import { readScrubPictures, reportFailure, type ScrubPictures } from "@orpheum/sdk";
import {
  createContext,
  useEffect,
  useMemo,
  type Context,
  type CSSProperties,
  type JSX,
} from "react";

import { pollerUntil, shownAndEveryPoller, type Poller } from "./polling.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

export type { ScrubPictures } from "@orpheum/sdk";

/**
 * The scrub-bar pictures of the film the timelines under it play: the Play page's and the cast
 * remote's (TargetTimeline); none on a cast's receiver, which shows the film alone (D337 §3).
 */
export const ScrubPicturesContext: Context<ScrubPictures | undefined> = createContext<
  ScrubPictures | undefined
>(undefined);

/** Where a timeline's picture of one place is: its sheet's address and the cell's top left. */
export interface ScrubCell {
  readonly url: string;
  readonly leftPx: number;
  readonly topPx: number;
}

/**
 * The picture of atMs: the one at or before it, n·intervalMs, the last for a place past the last
 * one, in its sheet's cell, row by row; undefined where that sheet is not made yet.
 */
export function scrubCellAt(pictures: ScrubPictures, atMs: number): ScrubCell | undefined {
  const { intervalMs, pictureCount, columns, rows } = pictures;
  if (intervalMs <= 0 || pictureCount <= 0 || columns <= 0 || rows <= 0) {
    return undefined;
  }
  const index = Math.min(Math.max(Math.floor(atMs / intervalMs), 0), pictureCount - 1);
  const perSheet = columns * rows;
  const sheetIndex = Math.floor(index / perSheet);
  const sheet = pictures.sheets.find((made) => made.index === sheetIndex);
  if (sheet === undefined) {
    return undefined;
  }
  const inSheet = index % perSheet;
  return {
    url: sheet.url,
    leftPx: (inSheet % columns) * pictures.pictureWidth,
    topPx: Math.floor(inSheet / columns) * pictures.pictureHeight,
  };
}

/** The narrowest a scrub-bar picture is shown, and the widest, in CSS pixels (the owner, 2026-11-28). */
const shownPictureWidthLeastPx = 160;
const shownPictureWidthMostPx = 240;

/**
 * How wide a scrub-bar picture is shown in a window windowWidthPx wide: a fifth of the window, no
 * narrower than 160 px nor wider than 240 px, as other players show theirs (a phone's 160, a
 * desktop's 240; the owner, 2026-11-28), and never wider than it was made (pictures.pictureWidth),
 * which is larger so that a dense screen shows it sharp.
 */
export function shownPictureWidthPx(pictures: ScrubPictures, windowWidthPx: number): number {
  const fifth = Math.round(windowWidthPx / 5);
  const wanted = Math.min(Math.max(fifth, shownPictureWidthLeastPx), shownPictureWidthMostPx);
  return Math.min(wanted, pictures.pictureWidth);
}

/**
 * The picture's box widthPx wide (shownPictureWidthPx), cut from its sheet by its background's
 * position, the sheet scaled as the picture is. Its place along the timeline is the caller's.
 */
export function ScrubPicture(props: {
  readonly pictures: ScrubPictures;
  readonly cell: ScrubCell;
  readonly widthPx: number;
}): JSX.Element {
  const { pictures, cell, widthPx } = props;
  const scale = pictures.pictureWidth > 0 ? widthPx / pictures.pictureWidth : 1;
  const px = (length: number): string => `${String(Math.round(length * scale))}px`;
  const style: CSSProperties = {
    width: px(pictures.pictureWidth),
    height: px(pictures.pictureHeight),
    backgroundImage: `url("${cell.url}")`,
    backgroundSize: `${px(pictures.pictureWidth * pictures.columns)} ${px(pictures.pictureHeight * pictures.rows)}`,
    backgroundPosition: `${px(-cell.leftPx)} ${px(-cell.topPx)}`,
  };
  return <span className="timeline__picture" style={style} />;
}

/** No pictures: none made, a film not read yet, or a cast's receiver. */
const noPictures = null;

/**
 * How often an open player asks again for its film's pictures while not every sheet is made: the
 * node makes them in the background, a sheet at a time, so a film opened before they are gains them
 * as it plays (the owner, 2026-11-27).
 */
export const scrubPicturesReadAgainEveryMs = 30_000;

/**
 * The poller of a film's pictures: every scrubPicturesReadAgainEveryMs while the page shows and as
 * it is shown again, one ask at a time, none while it is hidden; a read that failed is asked again
 * at the next time due, not at once.
 */
const scrubPicturesPoller = shownAndEveryPoller(scrubPicturesReadAgainEveryMs);

/**
 * Whether every sheet pictures' count, columns and rows call for is made, so asking again would find
 * nothing more. A list that calls for none is whole.
 */
export function isEverySheetMade(pictures: ScrubPictures): boolean {
  const perSheet = pictures.columns * pictures.rows;
  if (perSheet <= 0 || pictures.pictureCount <= 0) {
    return true;
  }
  const calledFor = Math.ceil(pictures.pictureCount / perSheet);
  const made = new Set(
    pictures.sheets.map((sheet) => sheet.index).filter((index) => index >= 0 && index < calledFor),
  );
  return made.size >= calledFor;
}

/** One film's reads of its pictures: the last it read, and the poller asking until it is whole. */
interface PicturesReading {
  readonly keep: (pictures: ScrubPictures) => void;
  readonly kept: () => ScrubPictures | undefined;
  readonly poller: Poller | undefined;
}

/** The reads of fileId's pictures; none asked again where there is no film (a cast's receiver). */
function picturesReading(fileId: number | undefined): PicturesReading {
  let last: ScrubPictures | undefined;
  const poller =
    fileId === undefined
      ? undefined
      : pollerUntil(scrubPicturesPoller, () => last !== undefined && isEverySheetMade(last));
  return {
    keep: (pictures) => {
      last = pictures;
    },
    kept: () => last,
    poller,
  };
}

/**
 * The scrub-bar pictures of fileId, read as the player opens and again every
 * scrubPicturesReadAgainEveryMs until every sheet is made (isEverySheetMade) or the player closes;
 * undefined while they are read, where none is made, and where fileId is undefined (a cast's
 * receiver), which asks nothing. A read that fails keeps the pictures read before it. Their sheets
 * are loaded once video first plays, or at once where there is no video here (the cast remote: the
 * film plays on the television), so a sheet never holds back the film's first seconds.
 */
export function useScrubPictures(
  fileId: number | undefined,
  video: HTMLVideoElement | null | "elsewhere",
): ScrubPictures | undefined {
  const { api } = useServices();
  const reading = useMemo(() => picturesReading(fileId), [fileId]);
  const [read] = useResource(
    `trickplay/${String(fileId ?? 0)}`,
    async () => {
      if (fileId === undefined) {
        return noPictures;
      }
      try {
        const pictures = (await readScrubPictures(api, fileId)) ?? noPictures;
        if (pictures !== noPictures) {
          reading.keep(pictures);
        }
        return pictures;
      } catch (cause) {
        const last = reading.kept();
        if (last === undefined) {
          throw cause;
        }
        reportFailure(cause);
        return last;
      }
    },
    { poller: reading.poller },
  );
  const pictures = read.state === "loaded" && read.value !== null ? read.value : undefined;
  useSheetsLoadedOncePlaying(pictures, video);
  return pictures;
}

/**
 * Loads pictures' sheets into the browser's cache, low in its order, once video plays: at once
 * where it plays already or elsewhere, else at its first "playing".
 */
function useSheetsLoadedOncePlaying(
  pictures: ScrubPictures | undefined,
  video: HTMLVideoElement | null | "elsewhere",
): void {
  useEffect(() => {
    if (pictures === undefined || video === null) {
      return undefined;
    }
    const held: HTMLImageElement[] = [];
    const load = (): void => {
      for (const sheet of pictures.sheets) {
        const image = new Image();
        image.fetchPriority = "low";
        image.src = sheet.url;
        held.push(image); // kept until the player closes, so its load is not dropped
      }
    };
    if (
      video === "elsewhere" ||
      (!video.paused && video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA)
    ) {
      load();
      return () => {
        held.length = 0;
      };
    }
    video.addEventListener("playing", load, { once: true });
    return () => {
      video.removeEventListener("playing", load);
      held.length = 0;
    };
  }, [pictures, video]);
}
