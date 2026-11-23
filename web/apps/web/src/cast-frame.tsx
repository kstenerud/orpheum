// SPDX-License-Identifier: MPL-2.0

import "./cast-frame.css";

import { apiPrefix } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type ReactNode } from "react";

/** How often a cast's remote asks for the film's picture at the receiver's place (D545 §2). */
export const castFrameEveryMs = 3000;

/**
 * The key frame of a file at a place: the keyframe at or before it, made by its node (D545 §4).
 * Under `/bytes/`, so the image's request carries the media cookie, as artwork's does.
 */
export function castFrameUrl(fileId: number, atMs: number): string {
  return `${apiPrefix}/bytes/files/${String(fileId)}/frame?atMs=${String(Math.max(0, Math.round(atMs)))}`;
}

/** What a cast's remote shows where the video would be. */
export interface CastFrameProps {
  /** The file the receiver plays. */
  fileId: number;
  /** The receiver's place in it, in milliseconds. */
  atMs: number;
  className?: string;
  /** What shows until the first frame has loaded: the film's still. */
  fallback: ReactNode;
}

/**
 * The film's picture at the receiver's place (D545): every 3 s, only if the place moved since the
 * frame last asked for, and never while the page is hidden, the key frame there is loaded apart and
 * shown once it has loaded, so nothing flashes or goes blank; `fallback` until the first has. It is
 * drawn as the video is, contained in the whole box.
 */
export function CastFrame({ fileId, atMs, className, fallback }: CastFrameProps): JSX.Element {
  const [shown, setShown] = useState<{ fileId: number; url: string } | null>(null);
  const placeRef = useRef(atMs);
  useEffect(() => {
    placeRef.current = atMs;
  }, [atMs]);
  useEffect(() => {
    let askedAt: number | null = null;
    let loading: HTMLImageElement | null = null;
    const ask = (): void => {
      if (document.visibilityState === "hidden") {
        return;
      }
      const at = Math.max(0, Math.round(placeRef.current));
      if (at === askedAt) {
        return;
      }
      askedAt = at;
      const url = castFrameUrl(fileId, at);
      const image = new Image();
      loading = image;
      image.onload = () => {
        if (loading === image) {
          setShown({ fileId, url });
        }
      };
      image.src = url;
    };
    ask();
    const timer = setInterval(ask, castFrameEveryMs);
    return () => {
      loading = null;
      clearInterval(timer);
    };
  }, [fileId]);
  const frame = shown?.fileId === fileId ? shown.url : null;
  return (
    <div className={["cast-frame", className].filter(Boolean).join(" ")}>
      {frame === null ? fallback : <img className="cast-frame-image" src={frame} alt={""} />}
    </div>
  );
}
