// SPDX-License-Identifier: MPL-2.0

/**
 * A film's scrub-bar pictures (trickplay.md §5, phase F3): the pictures its nodes made, every
 * interval of the film, in sheets of columns × rows, which a timeline shows above itself at the
 * place pointed at, dragged to or stepped to (Timeline). They are read as the player opens
 * (GET /files/{id}/trickplay), and their sheets loaded only once the film has started playing, so
 * they never compete with its first seconds of buffering; over WebRTC each sheet waits on the
 * cosmetic rung besides, below the film's bytes (rungOf, D479). A film with none made, or a stretch
 * whose sheet is not made yet, shows the time alone, as before.
 */

import { readScrubPictures, type ScrubPictures } from "@orpheum/sdk";
import { createContext, useEffect, type Context, type CSSProperties, type JSX } from "react";

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

/**
 * The picture's box, cut from its sheet by its background's position, its size the picture's. Its
 * place along the timeline is the caller's.
 */
export function ScrubPicture(props: {
  readonly pictures: ScrubPictures;
  readonly cell: ScrubCell;
}): JSX.Element {
  const { pictures, cell } = props;
  const style: CSSProperties = {
    width: `${String(pictures.pictureWidth)}px`,
    height: `${String(pictures.pictureHeight)}px`,
    backgroundImage: `url("${cell.url}")`,
    backgroundPosition: `${String(-cell.leftPx)}px ${String(-cell.topPx)}px`,
  };
  return <span className="timeline__picture" style={style} />;
}

/** No pictures: none made, a film not read yet, or a cast's receiver. */
const noPictures = null;

/**
 * The scrub-bar pictures of fileId, read as the player opens; undefined while they are read, where
 * none is made, and where fileId is undefined (a cast's receiver). Their sheets are loaded once
 * video first plays, or at once where there is no video here (the cast remote: the film plays on
 * the television), so a sheet never holds back the film's first seconds.
 */
export function useScrubPictures(
  fileId: number | undefined,
  video: HTMLVideoElement | null | "elsewhere",
): ScrubPictures | undefined {
  const { api } = useServices();
  const [read] = useResource(`trickplay/${String(fileId ?? 0)}`, async () =>
    fileId === undefined ? noPictures : ((await readScrubPictures(api, fileId)) ?? noPictures),
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
