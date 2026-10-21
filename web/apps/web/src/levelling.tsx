// SPDX-License-Identifier: MPL-2.0

import "./levelling.css";

import { canSetVolume, gainAfterVolume } from "@orpheum/player";
import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useFileWaits, waitText, type FileWait } from "./file-waits.ts";
import { tenthsText } from "./number-text.ts";
import { OwnChoiceSwitch } from "./sound.tsx";

/** What brings a sound to the level, as the server measured it (D226). */
type Levelling = components["schemas"]["LevellingBody"];

/** A playback's sound: what levels it, and what its copy already carries. */
export interface SoundLevel {
  readonly levelling: Levelling;
  /** Already in the sound, put in when a copy was prepared; 0 for the original. */
  readonly carriedGainDb: number;
}

/**
 * The sound of one audio track of a playback: a prepared copy's standard mix of the track at
 * audioTrackIndex, whose boosted twin plays at the same gain (D226 §3); or, for the original or a
 * copy with no such track, the sound the playback starts with.
 */
export function soundLevelOf(
  decision: Pick<
    components["schemas"]["StartPlaybackOutputBody"],
    "delivery" | "levelling" | "audioRenditions"
  >,
  audioTrackIndex: number | undefined,
): SoundLevel {
  const mix = decision.audioRenditions?.find(
    (candidate) => candidate.audioTrackIndex === audioTrackIndex && !candidate.isDialogueBoosted,
  );
  if (decision.delivery === "hls" && mix !== undefined) {
    return { levelling: mix.levelling, carriedGainDb: mix.carriedGainDb };
  }
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
  const { t, i18n } = useTranslation();
  const [isOn, setIsOn] = useState(props.isInitiallyOn);
  // The note says what the player does: it applies no gain where the volume cannot be set.
  const [canApplyGain] = useState(pageCanSetVolume);
  const addedDb = addedGainDb(props.level, isOn);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      gainAfterVolume(video).setDb(addedDb);
    }
  }, [addedDb, props.videoRef]);
  const isUnmeasured = canApplyGain && !props.level.levelling.isMeasured;
  const measuring = useFileWaits(props.fileId, isUnmeasured)?.readThrough;
  const note = measuringNote(levelNote(props.level, isOn, canApplyGain), measuring);
  const dbText = tenthsText(note.db, i18n.language);
  const wait = note.seconds === undefined ? "" : waitText(t, note.seconds);
  return (
    <>
      <OwnChoiceSwitch
        choice="isLevellingOn"
        label={t("play.level.switch")}
        isInitiallyOn={isOn}
        onSwitched={setIsOn}
      />
      <p className="levelling__note">{t(`play.level.${note.key}`, { db: dbText, wait })}</p>
    </>
  );
}
