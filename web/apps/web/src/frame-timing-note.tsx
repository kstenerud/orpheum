// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { timeFrames, type FrameTiming } from "./frame-timing.ts";

/**
 * A line under the video saying how late its picture reaches the screen and how far behind its
 * sound (FrameTiming), for a person to read out: the owner's TCL TV showed the picture half a second
 * after its sound (2026-10-24). Nothing until the first timing; a browser that cannot say says so.
 */
export function FrameTimingNote(props: { readonly video: HTMLVideoElement | null }): JSX.Element {
  const { t } = useTranslation();
  const [timing, setTiming] = useState<FrameTiming | undefined | "unknown">(undefined);
  useEffect(() => {
    const { video } = props;
    if (video === null) {
      return undefined;
    }
    return timeFrames(video, (timed) => {
      setTiming(timed ?? "unknown");
    });
  }, [props]);
  if (timing === undefined) {
    return <p className="play__reason">{t("play.frameTiming.waiting")}</p>;
  }
  if (timing === "unknown") {
    return <p className="play__reason">{t("play.frameTiming.unknown")}</p>;
  }
  return <p className="play__reason">{t("play.frameTiming.timed", { ...timing })}</p>;
}
