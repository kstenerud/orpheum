// SPDX-License-Identifier: MPL-2.0

import "./poster.css";

import { artUrl, type ArtSize } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useBackgroundChangesPoller } from "./polling.ts";

/**
 * A poster, or a named blank where there is none, or where its image does not load: one not yet
 * made in the sizes for screens answers "no such image" (the owner, Fire TV, 2026-10-25: Copper Kettle's
 * seasons showed broken images while their art waited its turn). The card's own text names the item.
 * A blank for an image that did not load asks for it again (useArtShown), and shows it once made.
 * A film or series with no poster but a still, its backdrop or a frame of its video, shows the still
 * in the blank's top half, the blank's own mark below it (the owner, 2026-10-27; D362 §2); a poster
 * shown takes its place.
 */
export function Poster(props: {
  readonly sourceSha256: string | undefined;
  readonly stillSha256?: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
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
        <span className="poster__caption">{t("common.noPoster")}</span>
      </span>
    );
  }
  return (
    <img
      className="poster"
      src={shown.url}
      alt={""}
      loading="lazy"
      width={240}
      height={360}
      onError={shown.onError}
    />
  );
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
  return <img className={props.className} src={shown.url} alt={""} onError={shown.onError} />;
}

/** The address an image is shown by, undefined for its blank, and what its img does when it fails. */
interface ArtShown {
  readonly url: string | undefined;
  readonly onError: () => void;
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
    readonly again: number;
  }>({ isFailed: false, again: 0 });
  const askedRef = useRef(0);
  const current =
    state.sha256 === sourceSha256 ? state : { sha256: sourceSha256, isFailed: false, again: 0 };
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
          setState({ sha256: sourceSha256, isFailed: false, again });
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
    return { url: undefined, onError: () => undefined };
  }
  return {
    url: againUrl(sourceSha256, size, current.again),
    onError: () => {
      setState({ sha256: sourceSha256, isFailed: true, again: current.again });
    },
  };
}

/** The address of an image's size, asked for again the again-th time: a new address each time. */
function againUrl(sourceSha256: string, size: ArtSize, again: number): string {
  const url = artUrl(sourceSha256, size);
  return again === 0 ? url : `${url}?again=${String(again)}`;
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
 * A film's clapperboard, open, drawn in the text's colour: the poster of a title that has none
 * yet, as one not identified has (the owner, 2026-10-06: "a subdued clapboard").
 */
function Clapperboard(): JSX.Element {
  return (
    <svg className="poster__clapperboard" viewBox="0 0 64 64" focusable="false">
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
    </svg>
  );
}
