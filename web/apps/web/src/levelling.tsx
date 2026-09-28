// SPDX-License-Identifier: MPL-2.0

import { canSetVolume, gainAfterVolume } from "@orpheum/player";
import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { OwnSwitch } from "./sound.tsx";

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
export function soundLevel(decision: components["schemas"]["StartPlaybackOutputBody"]): SoundLevel {
  const first = decision.audio?.[0];
  if (decision.kind === "hls" && first !== undefined) {
    return { levelling: first.levelling, carriedGainDb: first.carriedGainDb };
  }
  return { levelling: decision.levelling, carriedGainDb: 0 };
}

/**
 * The gain the player adds after the volume: the levelling when it is on, less what the sound
 * already carries, so turning it off takes a copy's gain back out (D226 §4).
 */
export function addedGainDb(level: SoundLevel, isOn: boolean): number {
  const wanted = isOn && level.levelling.isMeasured ? level.levelling.gainDb : 0;
  return wanted - level.carriedGainDb;
}

/** What the note under the switch says, as a catalogue key and the decibels it names. */
export function levelNote(
  level: SoundLevel,
  isOn: boolean,
  canApply: boolean,
): { key: string; db: number } {
  if (!canApply) {
    const carried = level.carriedGainDb;
    return carried === 0 ? { key: "deviceVolume", db: 0 } : direction(carried);
  }
  if (!level.levelling.isMeasured) {
    return { key: "notMeasured", db: 0 };
  }
  return isOn ? direction(level.levelling.gainDb) : { key: "asMixed", db: 0 };
}

function direction(db: number): { key: string; db: number } {
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
  readonly isOn: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const [isOn, setIsOn] = useState(props.isOn);
  const [canApply] = useState(pageCanSetVolume);
  const added = addedGainDb(props.level, isOn);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null && canApply) {
      gainAfterVolume(video).setDb(added);
    }
  }, [added, canApply, props.videoRef]);
  const note = levelNote(props.level, isOn, canApply);
  const db = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(note.db);
  return (
    <>
      <OwnSwitch
        choice="isLevelling"
        label={t("play.level.switch")}
        isOn={isOn}
        switched={setIsOn}
      />
      <p className="play__level-note">{t(`play.level.${note.key}`, { db })}</p>
    </>
  );
}
