// SPDX-License-Identifier: MPL-2.0

import "./sound-menu.css";
import "./play-words.ts";

import type { JSX, RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useAudioChoices } from "./audio-tracks.tsx";
import { DropDown } from "./drop-down.tsx";
import { FileSoundOffsetEntries } from "./file-sound-offset.tsx";
import { LevelLoudnessEntry } from "./levelling.tsx";
import { ListEntry, ListNote, ListOption, ListSubmenu } from "./list-box.tsx";
import { AudioIcon } from "./player-icons.tsx";
import type { PlayerTracks } from "./player-tracks.tsx";
import type { SuggestedSoundOffset } from "./problem-report.tsx";
import { currentLabelOf } from "./route-chooser.tsx";
import { BoostBox } from "./sound.tsx";
import { SoundOffsetControl, type FileOffsetOf } from "./sound-delay.tsx";

/** What the Sound menu's timing starts from, where the page plays a film of its own (D537). */
export interface SoundMenuTiming {
  /** The device's offset the playback started with. */
  readonly deviceInitialMs: number;
  /** The file playing and its own offset as the playback started; undefined where not given. */
  readonly file: FileOffsetOf | undefined;
  /** Opens Report a problem suggesting the person's own offset for the file (D537 §5). */
  readonly onTellTheAdmin: (suggested: SuggestedSoundOffset) => void;
}

/**
 * Sound (🎧, D538 §8): everything about the film's sound in one menu, in this order: Sound track,
 * the audio track playing, a submenu of the film's tracks where it has more than one; Level
 * loudness, the gain it adds in brackets; Dialogue boost, where the sound offers it; and, under the
 * Timing heading, This device, the device's timing, and This file, the person's own for the file,
 * with Make it everyone's or Tell the admin beneath it (file-sound-offset.tsx). The two timings are
 * shown apart, never as a total; the sound plays with both (D537 §1). The timing's steppers keep it
 * open; its other choices close it. Where the page gives no timing (timing undefined, a cast's
 * receiver), it holds the tracks and switches alone.
 */
export function SoundMenu(props: {
  readonly tracks: PlayerTracks;
  readonly timing: SoundMenuTiming | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { tracks, timing, videoRef } = props;
  const trackChoices = useAudioChoices(tracks.audio);
  return (
    <DropDown
      label={t("play.audio.label")}
      opener={<AudioIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("play.audio.label") }}
      isFromItsTop
      onOpenChange={props.onOpenChange}
    >
      {trackChoices.length > 1 ? (
        <ListSubmenu name={t("play.audio.menu")} value={currentLabelOf(trackChoices)}>
          {trackChoices.map((choice) => (
            <ListOption key={choice.key} isCurrent={choice.isCurrent} onChoose={choice.onChoose}>
              {choice.label}
            </ListOption>
          ))}
        </ListSubmenu>
      ) : null}
      <ListEntry>
        <LevelLoudnessEntry
          level={tracks.level}
          measuring={tracks.measuring}
          state={tracks.levelling}
        />
      </ListEntry>
      {tracks.boost === undefined ? null : (
        <ListEntry>
          <BoostBox state={tracks.boost} />
        </ListEntry>
      )}
      {timing === undefined ? null : <TimingEntries timing={timing} videoRef={videoRef} />}
    </DropDown>
  );
}

/** The Sound menu's Timing: its heading, then This device and This file, each a row of its own. */
function TimingEntries(props: {
  readonly timing: SoundMenuTiming;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const { t } = useTranslation();
  const { timing, videoRef } = props;
  return (
    <>
      <ListNote>
        <span className="sound-menu__heading">{t("play.soundOffset.timing")}</span>
      </ListNote>
      <ListEntry isKeptOpen isInsetAsChoices>
        <SoundOffsetControl
          videoRef={videoRef}
          initialMs={timing.deviceInitialMs}
          file={timing.file}
          isInAList
        />
      </ListEntry>
      {timing.file === undefined ? null : (
        <FileSoundOffsetEntries
          videoRef={videoRef}
          file={timing.file}
          deviceInitialMs={timing.deviceInitialMs}
          onTellTheAdmin={timing.onTellTheAdmin}
        />
      )}
    </>
  );
}
