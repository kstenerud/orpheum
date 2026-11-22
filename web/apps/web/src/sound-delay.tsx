// SPDX-License-Identifier: MPL-2.0

import "./devices-words.ts";
import "./play-words.ts";

import type { TFunction } from "i18next";
import {
  canMoveSoundEarlier,
  longestSoundOffsetMs,
  newAudioContext,
  soundOffset,
} from "@orpheum/player";
import { reportFailure, type components } from "@orpheum/sdk";
import {
  useEffect,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { setMyDeviceSoundOffset } from "./accounts.ts";
import { latestWrite } from "./latest-write.ts";
import { Stack } from "./layout.tsx";
import { problemText } from "./problem.ts";
import { soundLatencyMemory } from "./sound-latency.ts";
import { Stepper } from "./stepper.tsx";
import { useSoundPart } from "./sound-panel.ts";
import { useServices } from "./services.tsx";
import { useMe } from "./signed-in-me.ts";

/** How much each press moves the sound, in milliseconds. */
export const soundOffsetStepMs = 50;

/**
 * How long after the last press the device keeps the timing: a run of presses is one write, of
 * where it ended. A justified constant: past a quick run of presses, short beside a person's wait
 * to hear the change.
 */
export const soundOffsetKeptAfterMs = 500;

/**
 * A value this tab last set, which every control drawn hears (useSyncExternalStore): the hidden one
 * under the film and the player's menu alike (D407 §8), so neither moves from a stale one, and a
 * playback asked for before it was set, as a film page's warm-up is, takes it: the owner set 0
 * during one film and the next started at the 50 before it (2026-10-24).
 */
export interface TabValue<T> {
  readonly get: () => T;
  readonly set: (value: T) => void;
  readonly hear: (listener: () => void) => () => void;
}

/** A value this tab sets, starting at initial (TabValue). */
export function tabValue<T>(initial: T): TabValue<T> {
  let value = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => value,
    set: (next) => {
      value = next;
      for (const listener of listeners) {
        listener();
      }
    },
    hear: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

/** The device's offset this tab last set; undefined until it sets one. */
const deviceInThisTab = tabValue<number | undefined>(undefined);

/** The device's offset as this tab last set it, else as the playback started (initialMs). */
export function useDeviceSoundOffsetMs(initialMs: number): number {
  return (
    useSyncExternalStore(deviceInThisTab.hear, deviceInThisTab.get, deviceInThisTab.get) ??
    initialMs
  );
}

/** Forgets the offsets this tab set, the device's and a file's, for a test. */
export function forgetSoundOffsetSetInThisTab(): void {
  deviceInThisTab.set(undefined);
  forgetFileSoundOffsetSetInThisTab();
}

/** A sound offset held to the furthest the player moves the sound either way. */
export function heldToLongest(ms: number): number {
  return Math.min(Math.max(ms, -longestSoundOffsetMs), longestSoundOffsetMs);
}

/** A file's sound offset as a playback's start says it (D537 §4). */
export type FileSoundOffset = components["schemas"]["FileSoundOffsetBody"];

/** The file playing, and its sound offset as its playback started. */
export interface FileOffsetOf {
  readonly fileId: number;
  readonly initial: FileSoundOffset;
}

/**
 * A file's sound offset as it stands for the person (D537): every person's, an admin's, and their own,
 * undefined while they keep none, which overrides it for them.
 */
export interface FileOffsetNow {
  readonly fileId: number;
  readonly everyonesMs: number;
  readonly ownMs: number | undefined;
}

/** The file's offset this tab last set, for the file it was set for; undefined until it sets one. */
const fileInThisTab = tabValue<FileOffsetNow | undefined>(undefined);

/** Sets a file's offset as it now stands for the person, telling every control. */
export function setFileSoundOffsetInThisTab(now: FileOffsetNow): void {
  fileInThisTab.set(now);
}

/** Forgets the file's offset this tab set, for a test. */
export function forgetFileSoundOffsetSetInThisTab(): void {
  fileInThisTab.set(undefined);
}

/**
 * The file's sound offset as it stands now (FileOffsetNow), as this tab last set it for the file or
 * else as its playback started, and ms, the one it plays with: the person's own, else every
 * person's; 0 for no file.
 */
export function useFileSoundOffset(
  file: FileOffsetOf | undefined,
): FileOffsetNow & { readonly ms: number } {
  const set = useSyncExternalStore(fileInThisTab.hear, fileInThisTab.get, fileInThisTab.get);
  if (file === undefined) {
    return { fileId: 0, everyonesMs: 0, ownMs: undefined, ms: 0 };
  }
  const now: FileOffsetNow =
    set?.fileId === file.fileId
      ? set
      : { fileId: file.fileId, everyonesMs: file.initial.everyonesMs, ownMs: file.initial.ownMs };
  return { ...now, ms: now.ownMs ?? now.everyonesMs };
}

/**
 * The timing of this device's sound against its picture, under the video, which a person sets once
 * on a device that plays one of them late and the browser cannot tell: the owner's TCL TV, in TV
 * Bro, plays its sound half a second late (2026-10-24). Later holds the sound back; earlier moves a
 * streamed film's sound chunks forward, which a film playing its own file cannot, and says so. It is
 * the device's own, kept by the core with the device and given with each playback (initialMs), or
 * as this tab last set it, every control drawn showing the one value; it applies at once, and the
 * player counts Web Audio's own latency. A Watch Together viewer with no device of its own still
 * sets it, kept by the tab alone (the owner, 2026-11-06: "Put a control in watch together"). It is
 * typed, or stepped by − and +, in a stepper (SoundOffsetInput), its label "Sound on this device"
 * leading it. As an entry of ⁝'s list (isInAList, player-menus.tsx), it is a row of the list, not a
 * button that opens a pop-up (the owner, 2026-11-22).
 */
export function SoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly initialMs: number;
  /** The file playing and its own offset as its playback started (D537), which plays with this. */
  readonly file?: FileOffsetOf | undefined;
  readonly isInAList?: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { session } = useServices();
  const offsetMs = useDeviceSoundOffsetMs(props.initialMs);
  const fileMs = useFileSoundOffset(props.file).ms;
  const { keep, failure } = useKeptOnDevice();
  // Whether the sound is wanted earlier than this film can play it: it plays as its own file.
  const [isEarlierRefused, setIsEarlierRefused] = useState(false);
  useSoundOffsetApplied(props.videoRef, offsetMs + fileMs);
  const deviceId = session.deviceId();
  const setTo = (wanted: number): void => {
    const next = heldToLongest(wanted);
    deviceInThisTab.set(next);
    setIsEarlierRefused(isEarlierRefusedFor(props.videoRef, next + fileMs));
    if (deviceId !== undefined) {
      keep({ deviceId, ms: next });
    }
  };
  const isInAList = props.isInAList === true;
  useMovedByTheRemote(isInAList ? undefined : offsetMs, setTo);
  return (
    <TimingRow
      isInAList={isInAList}
      notes={<TimingNotes isEarlierRefused={isEarlierRefused} failure={failure} />}
    >
      {isInAList ? (
        <SoundOffsetInput
          label={t("play.soundOffset.thisDevice")}
          fieldLabel={t("play.soundOffset.field")}
          valueText={deviceTimingText(t, offsetMs)}
          ms={offsetMs}
          stepLabels={{
            earlier: t("play.soundOffset.thisDeviceEarlier"),
            later: t("play.soundOffset.thisDeviceLater"),
          }}
          onChange={setTo}
        />
      ) : (
        <SoundOffsetInput
          label={t("play.soundOffset.row")}
          fieldLabel={t("play.soundOffset.field")}
          valueText={timingText(t, offsetMs)}
          ms={offsetMs}
          onChange={setTo}
        />
      )}
    </TimingRow>
  );
}

/**
 * Plays the sound of the video in videoRef ms from its picture, the device's and the file's added
 * (D537 §1), held to the furthest the player plays either way: whichever control is drawn applies
 * the one sum, as each hears the same values.
 */
function useSoundOffsetApplied(videoRef: RefObject<HTMLVideoElement | null>, ms: number): void {
  const heldMs = heldToLongest(ms);
  useEffect(() => {
    const video = videoRef.current;
    if (video !== null) {
      soundOffset(video, newAudioContext, soundLatencyMemory()).setMs(heldMs);
    }
  }, [heldMs, videoRef]);
}

/** Whether a sound wanted ms from its picture is earlier than the film can play it: its own file's. */
export function isEarlierRefusedFor(
  videoRef: RefObject<HTMLVideoElement | null>,
  ms: number,
): boolean {
  const video = videoRef.current;
  return ms < 0 && video !== null && !canMoveSoundEarlier(video);
}

/** What a playback's start says of the sound's timing (D537 §4): the device's and the file's own. */
interface PlaybackSoundOffsets {
  readonly soundOffsetMs: number;
  readonly fileSoundOffset: FileSoundOffset;
}

/** The sound's timings a playback started with, as the overlay's menus are given them. */
export function soundOffsetsOf(decision: PlaybackSoundOffsets): PlaybackSoundOffsets {
  return { soundOffsetMs: decision.soundOffsetMs, fileSoundOffset: decision.fileSoundOffset };
}

/**
 * The sound's timing under a film playing a file of its own: the device's, starting where the
 * playback did, played with the file's (D537 §1), which the Sound menu sets.
 */
export function FilmTiming(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly fileId: number;
  readonly decision: PlaybackSoundOffsets;
}): JSX.Element | null {
  const { decision } = props;
  return (
    <SoundOffsetControl
      videoRef={props.videoRef}
      initialMs={decision.soundOffsetMs}
      file={{ fileId: props.fileId, initial: decision.fileSoundOffset }}
    />
  );
}

/**
 * A sound timing's stepper (stepper.tsx), the player's and a cast's remote's alike: the milliseconds
 * later the sound plays, below zero earlier, typed or stepped by soundOffsetStepMs, from the longest
 * earlier to the longest later the player plays; − earlier, + later. Its label leads its row, which
 * a second timing's row, a file's own, takes as this device's does.
 */
export function SoundOffsetInput(props: {
  readonly label: string;
  readonly fieldLabel: string;
  readonly valueText: string;
  readonly ms: number;
  /** What − and + are named where its row names them: the Sound menu's "This file earlier". */
  readonly stepLabels?: { readonly earlier: string; readonly later: string } | undefined;
  readonly onChange: (ms: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { stepLabels } = props;
  return (
    <Stepper
      label={props.label}
      fieldLabel={props.fieldLabel}
      valueText={props.valueText}
      value={props.ms}
      min={-longestSoundOffsetMs}
      max={longestSoundOffsetMs}
      step={soundOffsetStepMs}
      unit={t("play.soundOffset.unit")}
      stepDownLabel={
        stepLabels?.earlier ?? t("play.soundOffset.earlier", { ms: soundOffsetStepMs })
      }
      stepUpLabel={stepLabels?.later ?? t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      onChange={props.onChange}
    />
  );
}

/**
 * The sound timing starting from the offset kept with the device this tab is signed in on, and from
 * in step for a tab on none, where no playback gives one: a Watch Together viewer's under the film
 * (the owner, 2026-11-06: "Make it respect the sound offset. Put a control in watch together"), as
 * the server gives a guest's playback none, and the device lab's sound timing test's (lab-sound.tsx).
 */
export function DeviceSoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element | null {
  return <SoundOffsetControl videoRef={props.videoRef} initialMs={useThisDeviceSoundOffsetMs()} />;
}

/**
 * The sound offset kept with the device this tab is signed in on, as who is signed in says it (GET
 * /me's device, D480 §5), read for no list of devices; 0 for none, or until read.
 */
function useThisDeviceSoundOffsetMs(): number {
  const { session } = useServices();
  const { me } = useMe(session.deviceId() !== undefined);
  return me.state === "loaded" ? (me.value.device?.soundOffsetMs ?? 0) : 0;
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
 * The timing's parts under the video, or as an entry of ⁝'s list, its notes below its row: there its
 * label never changes, so its field and arrows stay where they are with each press, where words that
 * changed in one row with them moved the buttons between the row and the next on the owner's TCL TV
 * (2026-11-06).
 */
function TimingRow(props: {
  readonly isInAList: boolean;
  readonly notes: ReactNode;
  readonly children: ReactNode;
}): JSX.Element {
  return props.isInAList ? (
    <Stack space="1">
      {props.children}
      {props.notes}
    </Stack>
  ) : (
    <div className="play__sound-delay">
      {props.children}
      {props.notes}
    </div>
  );
}

/**
 * Keeps the timing with the device as presses settle (the audit of 2026-10-25, W9): the last of a
 * run, soundOffsetKeptAfterMs after it, one write at a time, so a slower earlier write over the
 * relay cannot land after a later one and leave the core an older value than the page shows. A
 * press still waiting is kept as the control goes. A write that failed is reported, and its cause
 * held until one is kept, for the control to say so.
 */
function useKeptOnDevice(): {
  readonly keep: (offset: { readonly deviceId: number; readonly ms: number }) => void;
  readonly failure: { readonly cause: unknown } | undefined;
} {
  const { api } = useServices();
  const [failure, setFailure] = useState<{ readonly cause: unknown } | undefined>(undefined);
  const [writes] = useState(() =>
    latestWrite<{ readonly deviceId: number; readonly ms: number }>(
      async ({ deviceId, ms }) => {
        await setMyDeviceSoundOffset(api, deviceId, ms);
      },
      soundOffsetKeptAfterMs,
      {
        kept: () => {
          setFailure(undefined);
        },
        failed: (cause) => {
          reportFailure(cause);
          setFailure({ cause });
        },
      },
    ),
  );
  useEffect(
    () => () => {
      writes.flush();
    },
    [writes],
  );
  return { keep: writes.write, failure };
}

/**
 * What a timing says beneath it: that the film cannot play its sound earlier, or a timing not kept,
 * the device's, or a file's (notKeptKey).
 */
export function TimingNotes(props: {
  readonly isEarlierRefused: boolean;
  readonly failure: { readonly cause: unknown } | undefined;
  readonly notKeptKey?: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { isEarlierRefused, failure } = props;
  return (
    <>
      {isEarlierRefused ? (
        <p className="play__reason">{t("play.soundOffset.cannotEarlier")}</p>
      ) : null}
      {failure === undefined ? null : (
        <p className="play__reason">
          {t(props.notKeptKey ?? "thisDevice.soundOffsetNotKept", {
            problem: problemText(t, failure.cause),
          })}
        </p>
      )}
    </>
  );
}

/** What the Sound menu's "This device" says while not 0: how much later or earlier the sound plays. */
function deviceTimingText(t: TFunction, offsetMs: number): string {
  return offsetMs > 0
    ? t("play.soundOffset.deviceLater", { ms: offsetMs })
    : t("play.soundOffset.deviceEarlier", { ms: -offsetMs });
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
