// SPDX-License-Identifier: MPL-2.0

import { longestSoundDelayMs, soundDelay } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { setMyDeviceSoundDelay } from "./accounts.ts";
import { useServices } from "./services.tsx";

/** How much each press moves the delay, in milliseconds. */
export const soundDelayStepMs = 50;

/**
 * The delay this tab last set, which a playback asked for before it, as a film page's warm-up is,
 * did not hear of: the owner set 0 during one film and the next started at the 50 before it
 * (2026-10-24). Undefined until the tab sets one.
 */
const setInThisTab: { ms: number | undefined } = { ms: undefined };

/** Forgets the delay this tab set, for a test. */
export function forgetSoundDelaySetInThisTab(): void {
  setInThisTab.ms = undefined;
}

/**
 * The delay of this device's sound after its picture, under the video, which a person sets once on
 * a TV that shows its picture late and the browser cannot tell: the owner's TCL TV, in TV Bro,
 * showed every film's picture half a second after its sound (2026-10-24). It is the device's own,
 * kept by the core with the device and given with each playback (initialMs), or as this tab last set
 * it, which a playback asked for earlier did not hear of; it applies at once, and
 * the player counts Web Audio's own latency toward it. A viewer with no device keeps none.
 */
export function SoundDelayControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly initialMs: number;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [delayMs, setDelayMs] = useState(setInThisTab.ms ?? props.initialMs);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      soundDelay(video).setMs(delayMs);
    }
  }, [delayMs, props.videoRef]);
  const deviceId = session.deviceId();
  if (deviceId === undefined) {
    return null;
  }
  const move = (byMs: number): void => {
    const next = Math.min(Math.max(delayMs + byMs, 0), longestSoundDelayMs);
    setDelayMs(next);
    setInThisTab.ms = next;
    void setMyDeviceSoundDelay(api, deviceId, next).catch(() => undefined);
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
