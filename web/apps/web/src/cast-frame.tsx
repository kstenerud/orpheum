// SPDX-License-Identifier: MPL-2.0

import "./cast-frame.css";

import { useEffect, useState, type CSSProperties, type JSX, type ReactNode } from "react";

import { scrubCellAt, type ScrubCell, type ScrubPictures } from "./scrub-pictures.tsx";

/** What a cast's remote shows where the video would be. */
export interface CastFrameProps {
  /** The film's scrub-bar pictures (useScrubPictures); undefined while none is made or read. */
  pictures: ScrubPictures | undefined;
  /** The receiver's place in the film, in milliseconds. */
  atMs: number;
  className?: string;
  /** What shows until the first picture's sheet has loaded: the film's still. */
  fallback: ReactNode;
}

/** The sheets loaded, by address: each one's whole width in pixels. */
type SheetWidths = ReadonlyMap<string, number>;

/** One picture drawn: its cell, its sheet's width and the pictures' own size. */
interface Drawn {
  readonly cell: ScrubCell;
  readonly sheetWidth: number;
  readonly pictureWidth: number;
  readonly pictureHeight: number;
}

const isSameDrawn = (a: Drawn, b: Drawn | null): boolean =>
  b !== null &&
  a.cell.url === b.cell.url &&
  a.cell.leftPx === b.cell.leftPx &&
  a.cell.topPx === b.cell.topPx &&
  a.sheetWidth === b.sheetWidth;

/**
 * The film's picture at the receiver's place (D571 §1): the scrub-bar picture whose interval holds
 * it (scrubCellAt, the timeline's), cut from its sheet and drawn as the video is, contained in the
 * whole box. Its sheet is loaded apart and the picture shown once it has loaded, the one before
 * staying meanwhile, so nothing flashes or goes blank; `fallback` until the first has, and while the
 * film has no pictures (D571 §2). It asks for nothing else: the pictures are the page's
 * (useScrubPictures), read again as they are made, so they appear during the cast.
 */
export function CastFrame({ pictures, atMs, className, fallback }: CastFrameProps): JSX.Element {
  const drawable = useDrawable(pictures, atMs);
  // The picture last drawn stays while the next one's sheet loads (state kept from a render before).
  const [shown, setShown] = useState<Drawn | null>(null);
  if (drawable !== null && !isSameDrawn(drawable, shown)) {
    setShown(drawable);
  }
  const drawn = drawable ?? (pictures === undefined ? null : shown);
  return (
    <div className={["cast-frame", className].filter(Boolean).join(" ")}>
      {drawn === null ? fallback : <CastPicture drawn={drawn} />}
    </div>
  );
}

/** The picture at atMs (scrubCellAt, the timeline's), once its sheet has loaded; else null. */
function useDrawable(pictures: ScrubPictures | undefined, atMs: number): Drawn | null {
  const cell = pictures === undefined ? undefined : scrubCellAt(pictures, atMs);
  const sheetWidth = useSheetWidth(cell?.url);
  return pictures === undefined || cell === undefined || sheetWidth === undefined
    ? null
    : {
        cell,
        sheetWidth,
        pictureWidth: pictures.pictureWidth,
        pictureHeight: pictures.pictureHeight,
      };
}

/** The width of the sheet at wanted once it has loaded, loaded apart when first asked for. */
function useSheetWidth(wanted: string | undefined): number | undefined {
  const [widths, setWidths] = useState<SheetWidths>(() => new Map());
  const sheetWidth = wanted === undefined ? undefined : widths.get(wanted);
  useEffect(() => {
    if (wanted === undefined || sheetWidth !== undefined) {
      return undefined;
    }
    let isAsked = true;
    const image = new Image();
    image.onload = () => {
      if (isAsked) {
        setWidths((before) => new Map(before).set(wanted, image.naturalWidth));
      }
    };
    image.src = wanted;
    return () => {
      isAsked = false;
    };
  }, [wanted, sheetWidth]);
  return sheetWidth;
}

/**
 * One picture cut from its sheet: a box of the picture's shape, as large as the frame contains, the
 * sheet in it scaled so one cell fills the box and moved by the cell's place.
 */
function CastPicture({ drawn }: { readonly drawn: Drawn }): JSX.Element {
  const { cell, sheetWidth, pictureWidth, pictureHeight } = drawn;
  const box: CSSProperties & { readonly "--cast-picture-ratio": string } = {
    "--cast-picture-ratio": `${String(pictureWidth)} / ${String(pictureHeight)}`,
  };
  const sheet: CSSProperties = {
    width: `${String((sheetWidth / pictureWidth) * 100)}%`,
    left: `${String((-cell.leftPx / pictureWidth) * 100)}%`,
    top: `${String((-cell.topPx / pictureHeight) * 100)}%`,
  };
  return (
    <span className="cast-frame-picture" style={box}>
      <img className="cast-frame-image" src={cell.url} style={sheet} alt={""} />
    </span>
  );
}
