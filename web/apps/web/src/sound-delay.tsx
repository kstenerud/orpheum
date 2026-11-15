// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { TFunction } from "i18next";
import { canMoveSoundEarlier, newAudioContext, soundOffset } from "@orpheum/player";
import { useEffect, useState, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { Cluster, Stack } from "./layout.tsx";
import { soundLatencyMemory } from "./sound-latency.ts";
import { keepDeviceSoundOffsetMs, useDeviceSoundOffsetMs } from "./sound-offset.ts";
import { useSoundPart } from "./sound-panel.ts";

/** How much each press moves the sound, in milliseconds. */
export const soundOffsetStepMs = 50;

/**
 * The timing of this device's sound against its picture, under the video, which a person sets once
 * on a device that plays one of them late and the browser cannot tell: the owner's TCL TV, in TV
 * Bro, plays its sound half a second late (2026-10-24). Later holds the sound back; earlier moves a
 * streamed film's sound chunks forward, which a film playing its own file cannot, and says so. It is
 * the device's own, kept by its browser (sound-offset.ts, D492 §1), every control drawn showing the
 * one value; it applies at once, and the player counts Web Audio's own latency. A Watch Together
 * viewer sets it too, with or without a device of its own (the owner, 2026-11-06: "Put a control in
 * watch together"). As an entry of ⁝'s list (isInAList, player-menus.tsx), its text is on a line of
 * its own, its buttons in a row below.
 */
export function SoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly isInAList?: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const offsetMs = useDeviceSoundOffsetMs();
  // Whether the sound is wanted earlier than this film can play it: it plays as its own file.
  const [isEarlierRefused, setIsEarlierRefused] = useState(false);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      soundOffset(video, newAudioContext, soundLatencyMemory()).setMs(offsetMs);
    }
  }, [offsetMs, props.videoRef]);
  const setTo = (wanted: number): void => {
    keepDeviceSoundOffsetMs(wanted);
    const video = props.videoRef.current;
    setIsEarlierRefused(wanted < 0 && video !== null && !canMoveSoundEarlier(video));
  };
  useMovedByTheRemote(props.isInAList === true ? undefined : offsetMs, setTo);
  const buttons = (
    <>
      <button
        type="button"
        onClick={() => {
          setTo(offsetMs - soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.earlier", { ms: soundOffsetStepMs })}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          setTo(offsetMs + soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      </button>
    </>
  );
  return (
    <TimingRow
      isInAList={props.isInAList === true}
      text={timingText(t, offsetMs)}
      notes={<TimingNotes isEarlierRefused={isEarlierRefused} />}
    >
      {buttons}
    </TimingRow>
  );
}

/**
 * Has a cast's remote move the timing too, as the control's own press (sound-panel.ts): the control
 * under the video says how it stands, ms, and ⁝'s copy, which comes and goes, says nothing
 * (undefined).
 */
function useMovedByTheRemote(ms: number | undefined, setTo: (ms: number) => void): void {
  useSoundPart(ms === undefined ? undefined : { kind: "soundOffset", ms }, (choice) => {
    if (choice.kind === "soundOffset") {
      setTo(choice.ms);
    }
  });
}

/**
 * The timing's parts under the video, or as an entry of ⁝'s list (the cast remote's too, D444): there its text has a line of its
 * own above the buttons, as its length changes with each press, and in one row with them it moved
 * Earlier between the row and the next on the owner's TCL TV (2026-11-06).
 */
export function TimingRow(props: {
  readonly isInAList: boolean;
  readonly text: string;
  readonly notes: ReactNode;
  readonly children: ReactNode;
}): JSX.Element {
  return props.isInAList ? (
    <Stack space="1">
      <span>{props.text}</span>
      <Cluster space="2">{props.children}</Cluster>
      {props.notes}
    </Stack>
  ) : (
    <div className="play__sound-delay">
      <span>{props.text}</span> {props.children}
      {props.notes}
    </div>
  );
}

/** What the timing says beneath it: that the film cannot play its sound earlier. */
function TimingNotes(props: { readonly isEarlierRefused: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  return props.isEarlierRefused ? (
    <p className="play__reason">{t("play.soundOffset.cannotEarlier")}</p>
  ) : null;
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
