// SPDX-License-Identifier: MPL-2.0

import "./poster.css";

import { artUrl, type ArtSize } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { suspectWorker } from "./worker-bypassed-note.tsx";

/**
 * A poster, or a named blank where there is none, or where its image does not load: one not yet
 * made in the sizes for screens answers "no such image" (the owner, Fire TV, 2026-10-25: a series'
 * seasons showed broken images while their art waited its turn). The card's own text names the item.
 * A blank for an image that did not load asks for it again (useArtShown), and shows it once made.
 * A film or series with no poster but a still, its backdrop or a frame of its video, shows the still
 * in the blank's top half, the blank's own mark below it (the owner, 2026-10-27; D362 §2); a poster
 * shown takes its place.
 */
export function Poster(props: {
  readonly sourceSha256: string | undefined;
  readonly stillSha256?: string | undefined;
  /**
   * Whether a still shown in the poster's place stands for it with no "No poster" caption: a season's
   * still is a picture of the season itself, and the caption is for a card with nothing (D379 §1).
   */
  readonly isStillEnough?: boolean;
  /**
   * Whether it is drawn at a poster tile's width (posterTileSizes), so the browser picks the smallest
   * size that fills it; a title page's larger poster, not one, asks for portrait-m alone.
   */
  readonly isTile?: boolean;
}): JSX.Element {
  const shown = useArtShown(props.sourceSha256, "portrait-m");
  if (shown.url === undefined) {
    const hasStill = props.stillSha256 !== undefined;
    return (
      <span
        className={hasStill ? "poster poster--blank poster--with-still" : "poster poster--blank"}
        aria-hidden="true"
      >
        {hasStill ? <Still sourceSha256={props.stillSha256} className="poster__still" /> : null}
        <Clapperboard />
        {hasStill && props.isStillEnough === true ? null : <NoPosterCaption />}
      </span>
    );
  }
  const image = (
    <img
      className="poster"
      src={shown.url}
      {...(props.isTile === true
        ? { srcSet: shown.srcSetOf(tileSizes), sizes: posterTileSizes }
        : {})}
      alt={""}
      loading="lazy"
      decoding="async"
      width={240}
      height={360}
      onError={shown.onError}
      {...(props.isTile === true ? { onLoad: shown.onDrawn } : {})}
    />
  );
  // A tile's image is drawn over a clock until it loads, then the clock goes: a list read whole lays
  // out every card at once, and the images arrive after. Gone, not covered: the faded clock is its
  // own layer, painted over the image (the owner, 2026-11-11), and a dimmed image is seen through.
  // The image stays the one element when the clock goes, so it is not drawn again.
  return props.isTile === true ? (
    <>
      {shown.isDrawn ? null : (
        <span className="poster-waiting" aria-hidden="true">
          <PosterClock />
        </span>
      )}
      {image}
    </>
  ) : (
    image
  );
}

/**
 * "No poster", under a blank poster. Its own, so a poster that has one mounts no translation: each
 * builds react-i18next a copy of the i18n object, a third of a page of cards' render on the Fire TV
 * (2026-11-11).
 */
function NoPosterCaption(): JSX.Element {
  const { t } = useTranslation();
  return <span className="poster__caption">{t("common.noPoster")}</span>;
}

/**
 * A season's or an episode's still, or a blank of its size where there is none or its image does
 * not load, as a poster's (Poster).
 */
export function Still(props: {
  readonly sourceSha256: string | undefined;
  readonly className: string;
}): JSX.Element {
  const shown = useArtShown(props.sourceSha256, "landscape-s");
  if (shown.url === undefined) {
    return <span className={props.className} aria-hidden="true" />;
  }
  return (
    <img
      className={props.className}
      src={shown.url}
      alt={""}
      loading="lazy"
      decoding="async"
      onError={shown.onError}
    />
  );
}

/**
 * A title page's wide art, its backdrop or its still, in size, and nothing while it is not made yet:
 * the page shows no broken image (the owner, 2026-10-26: "the banner is a broken link"), and the art
 * is asked for again and shown once made, as a poster is.
 */
export function WideArt(props: {
  readonly sourceSha256: string | undefined;
  readonly size: ArtSize;
  readonly className: string;
}): JSX.Element | null {
  const shown = useArtShown(props.sourceSha256, props.size);
  if (shown.url === undefined) {
    return null;
  }
  return (
    <img
      className={props.className}
      src={shown.url}
      alt={""}
      decoding="async"
      onError={shown.onError}
    />
  );
}

/**
 * The widths a poster tile is drawn at: --size-poster-min at each of tokens.css's breakpoints, the
 * wider of it and browse.css's. A row's poster is that wide, a grid's a little wider at most (its
 * columns share out what is left of the row). A justified constant, as phoneWidthQuery is: a
 * stylesheet's lengths, which an img's sizes cannot read.
 */
export const posterTileSizes = "(min-width: 1200px) 10.5rem, (min-width: 600px) 8.5rem, 6.75rem";

/**
 * Asks for a tile's art before its card is drawn, at a low priority, as the card will ask for it: a
 * poster by the same srcset and sizes, so the browser picks the very size the card's img will and
 * finds it kept; a still at landscape-s. The image is never put on the page. Dropping it (src "")
 * stops a load still under way.
 */
export function tileArtAhead(
  sourceSha256: string,
  kind: "poster" | "still",
  done: () => void,
): HTMLImageElement {
  const image = new Image();
  image.fetchPriority = "low";
  image.decoding = "async";
  image.onload = done;
  image.onerror = done;
  if (kind === "poster") {
    image.sizes = posterTileSizes;
    image.srcset = sourceSetOf(sourceSha256, tileSizes, 0);
    image.src = againUrl(sourceSha256, "portrait-m", 0);
  } else {
    image.src = againUrl(sourceSha256, "landscape-s", 0);
  }
  return image;
}

/** A poster tile's sizes to choose from, by their widths in pixels (D212; nodeproto/artwork.go). */
const tileSizes: readonly (readonly [ArtSize, number])[] = [
  ["portrait-s", 240],
  ["portrait-m", 480],
];

/**
 * The address an image is shown by, undefined for its blank, what its img does when it fails, and
 * whether it has loaded.
 */
interface ArtShown {
  readonly url: string | undefined;
  /**
   * An img's srcSet of the image in each of sizes, by its width, asked for again as url is. The
   * server answers a size not yet made with the nearest one it holds, and no size while it holds
   * none (D212 §3), so each loads or fails as url does, and the same ask brings them back.
   */
  readonly srcSetOf: (sizes: readonly (readonly [ArtSize, number])[]) => string;
  readonly onError: () => void;
  /** Whether the image at url has loaded, so it is drawn; what its img does when it loads. */
  readonly isDrawn: boolean;
  readonly onDrawn: () => void;
}

/**
 * The address of sourceSha256's image in size, undefined while it is blank. An image that does not
 * load is asked for again on the background changes' schedule (D237), backing off while it is still
 * not made, each ask by a new address so that no answer the browser kept is shown again: the server
 * makes an image's sizes in their turn, and its asking for one with none made queues them (D212 §3),
 * so a poster once blank shows its art soon after it is made, with no reload. It stops asking once
 * the image loads or the poster leaves the page.
 */
function useArtShown(sourceSha256: string | undefined, size: ArtSize): ArtShown {
  const poller = useBackgroundChangesPoller();
  const [state, setState] = useState<{
    readonly sha256?: string;
    readonly isFailed: boolean;
    readonly isDrawn: boolean;
    readonly again: number;
  }>({ isFailed: false, isDrawn: false, again: 0 });
  const askedRef = useRef(0);
  const current =
    state.sha256 === sourceSha256
      ? state
      : { sha256: sourceSha256, isFailed: false, isDrawn: false, again: 0 };
  const isFailed = sourceSha256 !== undefined && current.isFailed;
  useEffect(() => {
    if (!isFailed) {
      return undefined;
    }
    let isCurrent = true;
    const stop = poller.start((answered, signal) => {
      askedRef.current += 1;
      const again = askedRef.current;
      void imageLoads(againUrl(sourceSha256, size, again), signal).then((isLoaded) => {
        if (isLoaded && isCurrent) {
          setState({ sha256: sourceSha256, isFailed: false, isDrawn: false, again });
        }
        answered(isLoaded ? "changed" : "unchanged");
      });
    });
    return () => {
      isCurrent = false;
      stop();
    };
  }, [isFailed, poller, sourceSha256, size]);
  if (sourceSha256 === undefined || isFailed) {
    return {
      url: undefined,
      srcSetOf: () => "",
      onError: () => undefined,
      isDrawn: false,
      onDrawn: () => undefined,
    };
  }
  return {
    url: againUrl(sourceSha256, size, current.again),
    srcSetOf: (sizes) => sourceSetOf(sourceSha256, sizes, current.again),
    onError: () => {
      setState({ sha256: sourceSha256, isFailed: true, isDrawn: false, again: current.again });
      suspectWorker(); // a bypassed worker 404s every image, and the page says so (A51)
    },
    isDrawn: current.isDrawn,
    onDrawn: () => {
      setState({ sha256: sourceSha256, isFailed: false, isDrawn: true, again: current.again });
    },
  };
}

/** The address of an image's size, asked for again the again-th time: a new address each time. */
function againUrl(sourceSha256: string, size: ArtSize, again: number): string {
  const url = artUrl(sourceSha256, size);
  return again === 0 ? url : `${url}?again=${String(again)}`;
}

/** An img's srcset of an image in each of sizes, by its width, asked for the again-th time. */
function sourceSetOf(
  sourceSha256: string,
  sizes: readonly (readonly [ArtSize, number])[],
  again: number,
): string {
  return sizes
    .map(([each, width]) => `${againUrl(sourceSha256, each, again)} ${String(width)}w`)
    .join(", ");
}

/** Whether the image at url loads, false if signal aborts first. */
function imageLoads(url: string, signal?: AbortSignal): Promise<boolean> {
  return new Promise((resolve) => {
    const image = new Image();
    image.onload = () => {
      resolve(true);
    };
    image.onerror = () => {
      resolve(false);
    };
    signal?.addEventListener(
      "abort",
      () => {
        image.src = "";
        resolve(false);
      },
      { once: true },
    );
    image.src = url;
  });
}

/**
 * A clock, faded and still, where a poster is still to come: behind a tile's image until it is
 * drawn, and on a blank whose entry has not been read (the owner, 2026-11-11: "a faded,
 * non-animating clock").
 */
export function PosterClock(): JSX.Element {
  return (
    <Icon className="poster__clock">
      <g
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <circle cx="12" cy="12" r="9" />
        {/* The hands meet a little left of and below the middle, so their weight, up and to the
            right, looks centred (the owner, 2026-11-11). */}
        <path d="M11.5 6.6v5.9l3.5 2.7" />
      </g>
    </Icon>
  );
}

/**
 * A film's clapperboard, open, drawn in the text's colour: the poster of a title that has none
 * yet, as one not identified has (the owner, 2026-10-06: "a subdued clapboard").
 */
export function Clapperboard(): JSX.Element {
  return (
    <Icon className="poster__clapperboard" viewBox="0 0 64 64">
      <g fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round">
        <rect x="9" y="30" width="46" height="26" rx="3" />
        <path d="M9 38h46" />
        <path d="M9 29 52.5 17.5l2 7.2L11 36.2" />
      </g>
      <g fill="currentColor">
        <path d="M14 30h7l-4 8h-7z" />
        <path d="M28 30h7l-4 8h-7z" />
        <path d="M42 30h7l-4 8h-7z" />
        <path d="m20 27.4 6.8-1.8-1.5 8.6-6.8 1.8z" />
        <path d="m34 23.7 6.8-1.8-1.5 8.6-6.8 1.8z" />
        <path d="m48 20 4.4-1.2 1.4 5-5.3 1.4z" />
      </g>
    </Icon>
  );
}
