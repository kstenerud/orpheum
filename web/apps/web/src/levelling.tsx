// SPDX-License-Identifier: MPL-2.0

import "./levelling.css";

import { canSetVolume, gainAfterVolume } from "@orpheum/player";
import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

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
 * The sound a playback starts with: the original's main track, or a prepared copy's first mix,
 * whose boosted twin plays at the same gain (D226 §3).
 */
export function startingSoundLevel(
  decision: Pick<
    components["schemas"]["StartPlaybackOutputBody"],
    "delivery" | "levelling" | "audioRenditions"
  >,
): SoundLevel {
  const firstMix = decision.audioRenditions?.[0];
  if (decision.delivery === "hls" && firstMix !== undefined) {
    return { levelling: firstMix.levelling, carriedGainDb: firstMix.carriedGainDb };
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
  readonly level: SoundLevel;
  /** Whether levelling starts on; the switch keeps its own state after that. */
  readonly isInitiallyOn: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const [isOn, setIsOn] = useState(props.isInitiallyOn);
  const [canApplyGain] = useState(pageCanSetVolume);
  const addedDb = addedGainDb(props.level, isOn);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null && canApplyGain) {
      gainAfterVolume(video).setDb(addedDb);
    }
  }, [addedDb, canApplyGain, props.videoRef]);
  const note = levelNote(props.level, isOn, canApplyGain);
  const dbText = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(note.db);
  return (
    <>
      <OwnChoiceSwitch
        choice="isLevellingOn"
        label={t("play.level.switch")}
        isInitiallyOn={isOn}
        onSwitched={setIsOn}
      />
      <p className="levelling__note">{t(`play.level.${note.key}`, { db: dbText })}</p>
    </>
  );
}
