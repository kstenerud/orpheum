// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import { canMoveSoundEarlier, longestSoundOffsetMs, soundOffset } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { setMyDeviceSoundOffset } from "./accounts.ts";
import { useServices } from "./services.tsx";

/** How much each press moves the sound, in milliseconds. */
export const soundOffsetStepMs = 50;

/**
 * The offset this tab last set, which a playback asked for before it, as a film page's warm-up is,
 * did not hear of: the owner set 0 during one film and the next started at the 50 before it
 * (2026-10-24). Undefined until the tab sets one.
 */
const setInThisTab: { ms: number | undefined } = { ms: undefined };

/** Forgets the offset this tab set, for a test. */
export function forgetSoundOffsetSetInThisTab(): void {
  setInThisTab.ms = undefined;
}

/**
 * The timing of this device's sound against its picture, under the video, which a person sets once
 * on a device that plays one of them late and the browser cannot tell: the owner's TCL TV, in TV
 * Bro, plays its sound half a second late (2026-10-24). Later holds the sound back; earlier moves a
 * streamed film's sound chunks forward, which a film playing its own file cannot, and says so. It is
 * the device's own, kept by the core with the device and given with each playback (initialMs), or
 * as this tab last set it; it applies at once, and the player counts Web Audio's own latency. A
 * viewer with no device keeps none.
 */
export function SoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly initialMs: number;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [offsetMs, setOffsetMs] = useState(setInThisTab.ms ?? props.initialMs);
  // Whether the sound is wanted earlier than this film can play it: it plays as its own file.
  const [isEarlierRefused, setIsEarlierRefused] = useState(false);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      soundOffset(video).setMs(offsetMs);
    }
  }, [offsetMs, props.videoRef]);
  const deviceId = session.deviceId();
  if (deviceId === undefined) {
    return null;
  }
  const move = (byMs: number): void => {
    const next = Math.min(Math.max(offsetMs + byMs, -longestSoundOffsetMs), longestSoundOffsetMs);
    setOffsetMs(next);
    setInThisTab.ms = next;
    const video = props.videoRef.current;
    setIsEarlierRefused(next < 0 && video !== null && !canMoveSoundEarlier(video));
    void setMyDeviceSoundOffset(api, deviceId, next).catch(() => undefined);
  };
  return (
    <div className="play__sound-delay">
      <span>{timingText(t, offsetMs)}</span>{" "}
      <button
        type="button"
        onClick={() => {
          move(-soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.earlier", { ms: soundOffsetStepMs })}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          move(soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      </button>
      {isEarlierRefused ? (
        <p className="play__reason">{t("play.soundOffset.cannotEarlier")}</p>
      ) : null}
    </div>
  );
}

/** What the timing says: in step, or how much later or earlier the sound plays. */
function timingText(t: TFunction, offsetMs: number): string {
  if (offsetMs === 0) {
    return t("play.soundOffset.inStep");
  }
  return offsetMs > 0
    ? t("play.soundOffset.laterBy", { ms: offsetMs })
    : t("play.soundOffset.earlierBy", { ms: -offsetMs });
}
