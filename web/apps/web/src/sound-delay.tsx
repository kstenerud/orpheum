// SPDX-License-Identifier: MPL-2.0

import { longestSoundDelayMs, soundDelay } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage } from "./browser-storage.ts";

/** The browser's key for how long this device's sound is delayed after its picture, in ms. */
export const soundDelayKey = "orpheum.soundDelayMs";

/** How much each press moves the delay, in milliseconds. */
export const soundDelayStepMs = 50;

/** This device's sound delay as the browser keeps it: 0 when none is kept, or what is not a delay. */
export function keptSoundDelayMs(): number {
  const kept = Number(browserStorage().getItem(soundDelayKey) ?? "0");
  return Number.isInteger(kept) && kept >= 0 && kept <= longestSoundDelayMs ? kept : 0;
}

/**
 * The delay of this device's sound after its picture, under the video, which a person sets once on
 * a TV that shows its picture late and the browser cannot tell: the owner's TCL TV, in TV Bro,
 * showed every film's picture half a second after its sound (2026-10-24). It is kept by this
 * browser, the device, and applies at once; the player counts Web Audio's own latency toward it.
 */
export function SoundDelayControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const { t } = useTranslation();
  const [delayMs, setDelayMs] = useState(keptSoundDelayMs);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      soundDelay(video).setMs(delayMs);
    }
  }, [delayMs, props.videoRef]);
  const move = (byMs: number): void => {
    const next = Math.min(Math.max(delayMs + byMs, 0), longestSoundDelayMs);
    browserStorage().setItem(soundDelayKey, String(next));
    setDelayMs(next);
  };
  return (
    <div className="play__sound-delay">
      <span>{t("play.soundDelay.label", { ms: delayMs })}</span>{" "}
      <button
        type="button"
        onClick={() => {
          move(-soundDelayStepMs);
        }}
      >
        {t("play.soundDelay.less", { ms: soundDelayStepMs })}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          move(soundDelayStepMs);
        }}
      >
        {t("play.soundDelay.more", { ms: soundDelayStepMs })}
      </button>
    </div>
  );
}
