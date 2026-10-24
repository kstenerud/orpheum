// SPDX-License-Identifier: MPL-2.0

import { longestSoundDelayMs, soundDelay } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { setMyDeviceSoundDelay } from "./accounts.ts";
import { useServices } from "./services.tsx";

/** How much each press moves the delay, in milliseconds. */
export const soundDelayStepMs = 50;

/**
 * The delay of this device's sound after its picture, under the video, which a person sets once on
 * a TV that shows its picture late and the browser cannot tell: the owner's TCL TV, in TV Bro,
 * showed every film's picture half a second after its sound (2026-10-24). It is the device's own,
 * kept by the core with the device and given with each playback (initialMs); it applies at once, and
 * the player counts Web Audio's own latency toward it. A viewer with no device keeps none.
 */
export function SoundDelayControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly initialMs: number;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [delayMs, setDelayMs] = useState(props.initialMs);
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
