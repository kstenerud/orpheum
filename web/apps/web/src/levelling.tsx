// SPDX-License-Identifier: MPL-2.0

import "./levelling.css";

import {
  canSetVolume,
  gainAfterVolume,
  isWebAudioLate,
  newAudioContext,
  ownFileGainAfterVolume,
} from "@orpheum/player";
import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useFileWaits, waitText, type FileWait } from "./file-waits.ts";
import { useIsGuestReceiver } from "./guest-receiver.ts";
import { tenthsText } from "./number-text.ts";
import { soundLatencyMemory } from "./sound-latency.ts";
import { OwnChoiceBox, useOwnChoice, type OwnChoiceState } from "./sound.tsx";

/** What brings a sound to the level, as the server measured it (D226). */
type Levelling = components["schemas"]["LevellingBody"];

/** A playback's sound: what levels it, and what its copy already carries. */
export interface SoundLevel {
  readonly levelling: Levelling;
  /** Already in the sound, put in when its chunks were made; 0 for the original. */
  readonly carriedGainDb: number;
}

/** The sound of the original as it plays: what levels it, and nothing it carries already. */
export function soundLevelOf(
  decision: Pick<components["schemas"]["StartPlaybackOutputBody"], "levelling">,
): SoundLevel {
  return { levelling: decision.levelling, carriedGainDb: 0 };
}

/**
 * The gain the player adds after the volume: the levelling when it is on, less what the sound
 * already carries, so turning it off takes a copy's gain back out (D226 §4).
 */
export function addedGainDb(level: SoundLevel, isOn: boolean): number {
  const wantedDb = isOn && level.levelling.isMeasured ? level.levelling.gainDb : 0;
  return wantedDb - level.carriedGainDb;
}

/** What the note under the switch says, as a catalogue key and the decibels it names. */
export function levelNote(
  level: SoundLevel,
  isOn: boolean,
  canApplyGain: boolean,
): { key: string; db: number } {
  if (!canApplyGain) {
    const carriedDb = level.carriedGainDb;
    return carriedDb === 0 ? { key: "deviceVolume", db: 0 } : gainNote(carriedDb);
  }
  if (!level.levelling.isMeasured) {
    return { key: "notMeasured", db: 0 };
  }
  return isOn ? gainNote(level.levelling.gainDb) : { key: "asMixed", db: 0 };
}

/**
 * A note that the sound is not measured yet, saying instead about how many seconds measuring it
 * will take, or how long it waits to be, as the server says of its read-through (D324 §6); any other
 * note as it is.
 */
export function measuringNote(
  note: { key: string; db: number },
  measuring: FileWait | undefined,
): { key: string; db: number; seconds?: number } {
  const seconds = measuring?.secondsLeft;
  if (note.key !== "notMeasured" || measuring === undefined || seconds === undefined) {
    return note;
  }
  return { key: measuring.isRunning ? "measuring" : "waitingToMeasure", db: 0, seconds };
}

/** The note for a gain of db: at the level, raised or lowered, and by how much. */
function gainNote(db: number): { key: string; db: number } {
  if (db === 0) {
    return { key: "atLevel", db: 0 };
  }
  return db > 0 ? { key: "raised", db } : { key: "lowered", db: -db };
}

/**
 * Whether this page may set a media element's volume: the platform's answer, so a fresh element
 * gives it. A page rendered with no document, on a server, has no element to level.
 */
function pageCanSetVolume(): boolean {
  return typeof document === "undefined" || canSetVolume(document.createElement("video"));
}

/**
 * Levelling, under the video: the person's switch, the gain applied after the volume where the
 * page may set the volume, and a note saying what was done (D226 §4–§6).
 */
export function LevelSwitch(props: {
  /** The file playing, whose read-through measures its loudness. */
  readonly fileId: number;
  readonly level: SoundLevel;
  /** The audio track playing, by its place among the file's; undefined where none is known. */
  readonly audioTrackIndex?: number;
  /** Whether levelling starts on; the switch keeps its own state after that. */
  readonly isInitiallyOn: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const state = useOwnChoice("isLevellingOn", {
    isInitiallyOn: props.isInitiallyOn,
    onSwitched: () => undefined,
  });
  const heard = useMeasuredLevel(props.fileId, props.level, props.audioTrackIndex);
  useLevelGain(heard.level, state.isOn, { videoRef: props.videoRef });
  return <LevellingControl level={heard.level} measuring={heard.measuring} state={state} />;
}

/**
 * The level of a sound not measured when its playback started, taken from its file's levellings once
 * the server has measured its track: a film measured as it plays is levelled at once, not when it is
 * played again (D226 §6; the plan's item 8 of 2026-11-02, VQ50). A sound measured already, or
 * carrying a gain of its own, stands as it is.
 */
export function measuredLevel(
  level: SoundLevel,
  levellings: readonly Levelling[] | undefined,
  audioTrackIndex: number | undefined,
): SoundLevel {
  if (level.levelling.isMeasured || level.carriedGainDb !== 0 || audioTrackIndex === undefined) {
    return level;
  }
  const taken = levellings?.[audioTrackIndex];
  return taken?.isMeasured === true ? { levelling: taken, carriedGainDb: 0 } : level;
}

/** The sound playing as it is levelled, and, while it is not measured, its measuring's wait. */
export interface HeardLevel {
  readonly level: SoundLevel;
  readonly measuring: FileWait | undefined;
}

/**
 * The sound playing, level, as its file's waits say it once measured (measuredLevel), and how long
 * its measuring will take until then: the waits are asked for while the sound is not measured and
 * the page may set the volume, and no longer once it is.
 */
export function useMeasuredLevel(
  fileId: number,
  level: SoundLevel,
  audioTrackIndex: number | undefined,
): HeardLevel {
  const [canApplyGain] = useState(pageCanSetVolume);
  const [levellings, setLevellings] = useState<readonly Levelling[] | undefined>(undefined);
  const taken = measuredLevel(level, levellings, audioTrackIndex);
  // A guest receiver is refused its file's waits (D469 §4): it plays the level it was given.
  const isGuest = useIsGuestReceiver();
  const isWanted = canApplyGain && !taken.levelling.isMeasured && !isGuest;
  const waits = useFileWaits(fileId, isWanted);
  const said = waits?.levellings ?? undefined;
  if (said !== undefined && said !== levellings) {
    // Taken up as it is drawn, React's way of keeping what a render learned (react.dev, "Adjusting
    // some state when a prop changes"); the waits keep that answer once no longer asked for.
    setLevellings(said);
  }
  return { level: taken, measuring: isWanted ? waits?.readThrough : undefined };
}

/**
 * Applies levelling's gain after the volume, where the page may set the volume, as the switch, isOn,
 * stands: whoever holds the switch, drawn or not (D226 §4). A film playing its own file on a device
 * whose Web Audio is late takes none (D436).
 */
export function useLevelGain(
  level: SoundLevel,
  isOn: boolean,
  played: { readonly videoRef: RefObject<HTMLVideoElement | null>; readonly isOwnFile?: boolean },
): void {
  const addedDb = addedGainDb(level, isOn);
  const { videoRef, isOwnFile = false } = played;
  useEffect(() => {
    const video = videoRef.current;
    if (video !== null) {
      const gainOf = isOwnFile ? ownFileGainAfterVolume : gainAfterVolume;
      gainOf(video, newAudioContext, soundLatencyMemory()).setDb(addedDb);
    }
  }, [addedDb, videoRef, isOwnFile]);
}

/** The files this page has asked to raise on their ladders, each once (useRaiseLeftToTheLadder). */
const raisesAsked = new Set<number>();

/**
 * Asks for the film again, once, through askAgain, where levelling, on as the server keeps it, raises
 * the sound playing, and this device's sound through Web Audio is too late to raise it by
 * (isWebAudioLate): the ladder's sound, which the server makes with the gain, carries the raise, as
 * the dialogue boost's move does (D436, D336 §3). A film the server answers with its own file again,
 * having no ladder to give, is not asked for again. A device never measured levels through Web Audio
 * and learns, and is not moved during the film it learns in.
 */
export function useRaiseLeftToTheLadder(
  fileId: number,
  level: SoundLevel,
  asked: { readonly isLevellingKept: boolean; readonly askAgain: (() => void) | undefined },
): void {
  const { isLevellingKept, askAgain } = asked;
  const isWanted =
    isLevellingKept && addedGainDb(level, true) > 0 && isWebAudioLate(soundLatencyMemory());
  useEffect(() => {
    if (!isWanted || askAgain === undefined || raisesAsked.has(fileId)) {
      return;
    }
    raisesAsked.add(fileId);
    askAgain();
  }, [isWanted, fileId, askAgain]);
}

/** Levelling's switch, held by whoever holds the choice, and the note saying what was done (D226 §5). */
export function LevellingControl(props: {
  readonly level: SoundLevel;
  /** How long measuring the sound will take, while it is not measured (useMeasuredLevel). */
  readonly measuring: FileWait | undefined;
  readonly state: OwnChoiceState;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  // The note says what the player does: it applies no gain where the volume cannot be set.
  const [canApplyGain] = useState(pageCanSetVolume);
  const note = measuringNote(
    levelNote(props.level, props.state.isOn, canApplyGain),
    props.measuring,
  );
  const dbText = tenthsText(note.db, i18n.language);
  const wait = note.seconds === undefined ? "" : waitText(t, note.seconds);
  return (
    <>
      <OwnChoiceBox label={t("play.level.switch")} state={props.state} />
      <p className="levelling__note">{t(`play.level.${note.key}`, { db: dbText, wait })}</p>
    </>
  );
}
