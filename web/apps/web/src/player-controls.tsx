// SPDX-License-Identifier: MPL-2.0

import "./player-controls.css";

import { seekTo } from "@orpheum/player";
import { useEffect, useState, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { FullScreenButton } from "./full-screen-button.tsx";
import { useIsVolumeSettable } from "./film-volume-slider.tsx";
import { IconButton } from "./icon-button.tsx";
import { answerPlayerCommand, seekStepMs } from "./player-commands.ts";
import {
  MutedIcon,
  PauseIcon,
  PlayIcon,
  SeekBackIcon,
  SeekForwardIcon,
  SoundIcon,
} from "./player-icons.tsx";
import { Timeline } from "./timeline.tsx";
import { VolumeRange } from "./volume-range.tsx";

/**
 * What a bar of player controls plays and commands (D369 §1): a video on this page
 * (useVideoSource), or a film another device plays, as a cast's remote commands its receiver.
 */
export interface PlayerSource {
  /** Where the film stands, in ms. */
  readonly atMs: number;
  /** The film's length, in ms; 0 while not known. */
  readonly durationMs: number;
  readonly isPaused: boolean;
  /** Whether it takes commands now: a remote's waits until it knows where the film is. */
  readonly isReady: boolean;
  readonly playOrPause: () => void;
  readonly seekTo: (atMs: number) => void;
  /** The film's sound here, for a video on this page; none for a film another device plays. */
  readonly sound?: PlayerSound | undefined;
}

/** A video's sound as the bar shows and sets it. */
export interface PlayerSound {
  /** From 0 to 1. */
  readonly volume: number;
  readonly isMuted: boolean;
  /** Whether the page can set the volume: an iPhone's is its buttons' alone (idiosyncrasies.md). */
  readonly canSetVolume: boolean;
  readonly setVolume: (volume: number) => void;
  readonly setMuted: (isMuted: boolean) => void;
}

/**
 * The controls a bar offers: play or pause; a step back and forward (seekStepMs); the timeline;
 * the sound (mute, and the volume where the page can set it); full screen.
 */
export type PlayerControl = "play" | "steps" | "timeline" | "sound" | "fullScreen";

/**
 * The player's bar (D369 §1), one for every device and every source: over a film on this page, a
 * television's or a computer's (their overlays decide when it shows), or a page of its own that
 * commands a film elsewhere (a cast's remote). It draws the controls it is given, in one order: the
 * buttons, whatever aside the page adds, full screen, then the sound, its volume last in the row, as a
 * slider keeps Left and Right for itself and an arrow could not pass it to what came after; then the
 * timeline.
 */
export function PlayerControls(props: {
  readonly source: PlayerSource;
  readonly controls: readonly PlayerControl[];
  /** Whether the bar is drawn over a film, its buttons on the film's control ground. */
  readonly isOverVideo?: boolean | undefined;
  /** Play or pause's button, which a television's overlay moves focus to as it opens. */
  readonly playRef?: RefObject<HTMLButtonElement | null> | undefined;
  /** The timeline's slider, which a television's overlay asks whether it has focus. */
  readonly timelineRef?: RefObject<HTMLInputElement | null> | undefined;
  /** How long a drag along the timeline rests before it seeks (Timeline's settleMs). */
  readonly settleMs?: number | undefined;
  /** Whether the timeline's Left and Right are the player's commands (a television's). */
  readonly hasArrowsAsCommands?: boolean | undefined;
  /** What the page adds at the buttons' end: a television's "Ends at". */
  readonly aside?: ReactNode | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, controls, isOverVideo, playRef } = props;
  const has = (control: PlayerControl): boolean => controls.includes(control);
  return (
    <div className="player-controls">
      <div className="player-controls__buttons">
        {has("play") ? (
          <IconButton
            ref={playRef}
            isOverVideo={isOverVideo}
            label={source.isPaused ? t("playerControls.play") : t("playerControls.pause")}
            icon={source.isPaused ? <PlayIcon /> : <PauseIcon />}
            isDisabled={!source.isReady}
            onClick={source.playOrPause}
          />
        ) : null}
        {has("steps") ? <Steps source={source} isOverVideo={isOverVideo} /> : null}
        <span className="player-controls__aside">{props.aside}</span>
        {has("fullScreen") ? <FullScreenButton /> : null}
        {has("sound") && source.sound !== undefined ? (
          <SoundControls sound={source.sound} isOverVideo={isOverVideo} />
        ) : null}
      </div>
      {has("timeline") ? (
        <Timeline
          inputRef={props.timelineRef}
          label={t("playerControls.where")}
          atMs={source.atMs}
          durationMs={source.durationMs}
          isDisabled={!source.isReady}
          settleMs={props.settleMs}
          hasArrowsAsCommands={props.hasArrowsAsCommands}
          onSeek={source.seekTo}
        />
      ) : null}
    </div>
  );
}

/** A step back and a step forward, seekStepMs each, never before the start nor past the end. */
function Steps(props: {
  readonly source: PlayerSource;
  readonly isOverVideo: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, isOverVideo } = props;
  const stepBy = (byMs: number): void => {
    const mostMs = source.durationMs > 0 ? source.durationMs : Number.POSITIVE_INFINITY;
    source.seekTo(Math.min(Math.max(0, source.atMs + byMs), mostMs));
  };
  return (
    <>
      <IconButton
        isOverVideo={isOverVideo}
        label={t("playerControls.back")}
        icon={<SeekBackIcon />}
        isDisabled={!source.isReady}
        onClick={() => {
          stepBy(-seekStepMs);
        }}
      />
      <IconButton
        isOverVideo={isOverVideo}
        label={t("playerControls.forward")}
        icon={<SeekForwardIcon />}
        isDisabled={!source.isReady}
        onClick={() => {
          stepBy(seekStepMs);
        }}
      />
    </>
  );
}

/** Mute, and the volume where the page can set it. */
function SoundControls(props: {
  readonly sound: PlayerSound;
  readonly isOverVideo: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound, isOverVideo } = props;
  const isSilent = sound.isMuted || sound.volume === 0;
  return (
    <span className="player-controls__sound">
      <IconButton
        isOverVideo={isOverVideo}
        label={isSilent ? t("playerControls.unmute") : t("playerControls.mute")}
        icon={isSilent ? <MutedIcon /> : <SoundIcon />}
        onClick={() => {
          if (sound.isMuted || sound.volume > 0) {
            sound.setMuted(!sound.isMuted);
          } else {
            sound.setVolume(1); // turned all the way down: sound on is the volume back up
          }
        }}
      />
      {sound.canSetVolume ? (
        <VolumeRange
          label={t("playerControls.volume")}
          value={sound.isMuted ? 0 : sound.volume}
          most={1}
          onChange={(volume) => {
            sound.setVolume(volume);
            if (volume > 0 && sound.isMuted) {
              sound.setMuted(false);
            }
          }}
        />
      ) : null}
    </span>
  );
}

/** Where a video stands, its length, and whether it is paused, as it says them at its own events. */
interface Standing {
  readonly atMs: number;
  readonly durationMs: number;
  readonly isPaused: boolean;
}

/**
 * The video in videoRef as a bar's source: where it stands, read at its own events, play or pause
 * and seeks made on it, and its sound.
 */
export function useVideoSource(videoRef: RefObject<HTMLVideoElement | null>): PlayerSource {
  const stand = useStanding(videoRef);
  const sound = useVideoSound(videoRef);
  return {
    ...stand,
    isReady: true,
    sound,
    playOrPause: () => {
      const video = videoRef.current;
      if (video !== null) {
        answerPlayerCommand(video, "playPause");
      }
    },
    seekTo: (atMs) => {
      const video = videoRef.current;
      if (video !== null) {
        seekTo(video, atMs);
      }
    },
  };
}

/** Where the film stands, its length, and whether it is paused, as the video says them. */
function useStanding(videoRef: RefObject<HTMLVideoElement | null>): Standing {
  const [stand, setStand] = useState<Standing>({ atMs: 0, durationMs: 0, isPaused: true });
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const heard = (): void => {
      setStand({
        atMs: video.currentTime * 1000,
        durationMs: Number.isFinite(video.duration) ? video.duration * 1000 : 0,
        isPaused: video.paused,
      });
    };
    // Read at the video's own events, the first as soon as it knows its length.
    const events = ["loadedmetadata", "timeupdate", "durationchange", "play", "pause", "seeked"];
    for (const each of events) {
      video.addEventListener(each, heard);
    }
    return () => {
      for (const each of events) {
        video.removeEventListener(each, heard);
      }
    };
  }, [videoRef]);
  return stand;
}

/** The video's sound, read at its volumechange, set on the video itself. */
function useVideoSound(videoRef: RefObject<HTMLVideoElement | null>): PlayerSound {
  const canSetVolume = useIsVolumeSettable();
  const [heard, setHeard] = useState({ volume: 1, isMuted: false });
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const read = (): void => {
      setHeard({ volume: video.volume, isMuted: video.muted });
    };
    // Read at each change: a video starts at full volume, unmuted (film-player.tsx).
    video.addEventListener("volumechange", read);
    return () => {
      video.removeEventListener("volumechange", read);
    };
  }, [videoRef]);
  return {
    ...heard,
    canSetVolume,
    setVolume: (volume) => {
      const video = videoRef.current;
      if (video !== null) {
        video.volume = Math.min(1, Math.max(0, volume));
      }
    },
    setMuted: (isMuted) => {
      const video = videoRef.current;
      if (video !== null) {
        video.muted = isMuted;
      }
    },
  };
}
