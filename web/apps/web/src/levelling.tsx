// SPDX-License-Identifier: MPL-2.0

import "./levelling.css";

import { canSetVolume, gainAfterVolume, newAudioContext } from "@orpheum/player";
import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useFileWaits, waitText, type FileWait } from "./file-waits.ts";
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
  /** Whether levelling starts on; the switch keeps its own state after that. */
  readonly isInitiallyOn: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const state = useOwnChoice("isLevellingOn", {
    isInitiallyOn: props.isInitiallyOn,
    onSwitched: () => undefined,
  });
  useLevelGain(props.level, state.isOn, props.videoRef);
  return <LevellingControl fileId={props.fileId} level={props.level} state={state} />;
}

/**
 * Applies levelling's gain after the volume, where the page may set the volume, as the switch, isOn,
 * stands: whoever holds the switch, drawn or not (D226 §4).
 */
export function useLevelGain(
  level: SoundLevel,
  isOn: boolean,
  videoRef: RefObject<HTMLVideoElement | null>,
): void {
  const addedDb = addedGainDb(level, isOn);
  useEffect(() => {
    const video = videoRef.current;
    if (video !== null) {
      gainAfterVolume(video, newAudioContext, soundLatencyMemory()).setDb(addedDb);
    }
  }, [addedDb, videoRef]);
}

/** Levelling's switch, held by whoever holds the choice, and the note saying what was done (D226 §5). */
export function LevellingControl(props: {
  /** The file playing, whose read-through measures its loudness. */
  readonly fileId: number;
  readonly level: SoundLevel;
  readonly state: OwnChoiceState;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  // The note says what the player does: it applies no gain where the volume cannot be set.
  const [canApplyGain] = useState(pageCanSetVolume);
  const isUnmeasured = canApplyGain && !props.level.levelling.isMeasured;
  const measuring = useFileWaits(props.fileId, isUnmeasured)?.readThrough;
  const note = measuringNote(levelNote(props.level, props.state.isOn, canApplyGain), measuring);
  const dbText = tenthsText(note.db, i18n.language);
  const wait = note.seconds === undefined ? "" : waitText(t, note.seconds);
  return (
    <>
      <OwnChoiceBox label={t("play.level.switch")} state={props.state} />
      <p className="levelling__note">{t(`play.level.${note.key}`, { db: dbText, wait })}</p>
    </>
  );
}
