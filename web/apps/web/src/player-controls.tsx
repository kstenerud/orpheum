// SPDX-License-Identifier: MPL-2.0

import "./player-controls.css";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { FullScreenButton } from "./full-screen-button.tsx";
import { useIsVolumeSettable } from "./film-volume-slider.tsx";
import { IconButton } from "./icon-button.tsx";
import { answerPlayerCommand } from "./player-commands.ts";
import {
  MutedIcon,
  NextIcon,
  PauseIcon,
  PlayIcon,
  PreviousIcon,
  SeekBackIcon,
  SeekForwardIcon,
  SoundIcon,
} from "./player-icons.tsx";
import { askedByOf, seekAheadOf, seekAimedEvent, seekStepMs } from "./seek-ahead.ts";
import { Timeline } from "./timeline.tsx";
import { VolumeRange } from "./volume-range.tsx";

/**
 * What a bar of player controls plays and commands (D369 §1): a video on this page
 * (useVideoSource), or a film another device plays, as a cast's remote commands its receiver.
 */
export interface PlayerSource {
  /**
   * Where the film stands, in ms, or where the person sent it while that seek is chosen or on its way
   * (seek-ahead.ts): a timeline's marker moves at once and stays (the owner, 2026-10-27).
   */
  readonly atMs: number;
  /** The film's length, in ms; 0 while not known. */
  readonly durationMs: number;
  readonly isPaused: boolean;
  /** Whether it takes commands now: a remote's waits until it knows where the film is. */
  readonly isReady: boolean;
  readonly playOrPause: () => void;
  /** Seeks to atMs: at once, or, a remote's, once its moves rest (seek-ahead.ts). */
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
 * The controls a bar offers: play or pause; a step back and forward (seekStepMs); the previous and
 * the next in the sequence, where the bar is given them (SequenceMoves); the timeline; the sound
 * (mute, and the volume where the page can set it); full screen.
 */
export type PlayerControl =
  "play" | "steps" | "previous" | "next" | "timeline" | "sound" | "fullScreen";

/**
 * What the bar's previous and next play (D407 §1–§2): the video before and after this one in its
 * sequence, each absent where there is none to go to, and its button with it.
 */
export interface SequenceMoves {
  readonly previous?: (() => void) | undefined;
  readonly next?: (() => void) | undefined;
}

/**
 * The player's bar (D369 §1), one for every device and every source: over a film on this page, a
 * television's or a computer's (their overlays decide when it shows), or a page of its own that
 * commands a film elsewhere (a cast's remote). It draws the controls it is given, in one order: the
 * buttons, whatever aside the page adds, full screen, then the sound, its volume last in the row, as a
 * slider keeps Left and Right for itself and an arrow could not pass it to what came after; then the
 * timeline. The full-screen overlay's row (isOverlayRow, D407 §1) puts play between the steps, in a
 * leading group, and its menus, full screen and the sound in a trailing one, the aside between.
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
  /** Whether the timeline's Left and Right are the player's commands (a television's). */
  readonly hasArrowsAsCommands?: boolean | undefined;
  /** What the page adds at the buttons' end: a television's "Ends at". */
  readonly aside?: ReactNode | undefined;
  /** Whether the buttons are the full-screen overlay's row, play between the steps. */
  readonly isOverlayRow?: boolean | undefined;
  /** What previous and next play, where the controls hold them. */
  readonly moves?: SequenceMoves | undefined;
  /** The overlay row's menus' openers, 💬 🎧 ⁝ (player-menus.tsx), before full screen. */
  readonly menus?: ReactNode | undefined;
  /** The film's video, whose stage full screen puts full screen; none, the stage the bar is in. */
  readonly videoRef?: RefObject<HTMLVideoElement | null> | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, controls, isOverVideo, playRef, aside, isOverlayRow, moves, menus } = props;
  return (
    <div className="player-controls">
      <ControlButtons
        source={source}
        controls={controls}
        isOverVideo={isOverVideo}
        playRef={playRef}
        aside={aside}
        isOverlayRow={isOverlayRow}
        moves={moves}
        menus={menus}
        videoRef={props.videoRef}
      />
      {controls.includes("timeline") ? (
        <Timeline
          inputRef={props.timelineRef}
          label={t("playerControls.where")}
          atMs={source.atMs}
          durationMs={source.durationMs}
          isDisabled={!source.isReady}
          hasArrowsAsCommands={props.hasArrowsAsCommands}
          onSeek={source.seekTo}
        />
      ) : null}
    </div>
  );
}

/** What the bar's buttons are drawn from (ControlButtons): PlayerControls' props but the timeline's. */
interface ButtonsParts {
  readonly source: PlayerSource;
  readonly controls: readonly PlayerControl[];
  readonly isOverVideo: boolean | undefined;
  readonly playRef: RefObject<HTMLButtonElement | null> | undefined;
  readonly aside: ReactNode;
  readonly isOverlayRow: boolean | undefined;
  readonly moves: SequenceMoves | undefined;
  readonly menus: ReactNode;
  readonly videoRef: RefObject<HTMLVideoElement | null> | undefined;
}

/**
 * The bar's buttons among the controls given (PlayerControls): in the bar's order, or as the
 * overlay's row, previous, the steps with play between them and next in a leading group, and full
 * screen and the sound trailing.
 */
function ControlButtons(props: ButtonsParts): JSX.Element {
  const { source, controls, isOverVideo, playRef, aside, isOverlayRow, moves, menus } = props;
  const has = (control: PlayerControl): boolean => controls.includes(control);
  const move = (isForward: boolean): JSX.Element | null => {
    const go = isForward ? moves?.next : moves?.previous;
    return has(isForward ? "next" : "previous") && go !== undefined ? (
      <MoveButton isOverVideo={isOverVideo} isForward={isForward} onMove={go} />
    ) : null;
  };
  const play = has("play") ? (
    <PlayButton source={source} isOverVideo={isOverVideo} playRef={playRef} />
  ) : null;
  const step = (isForward: boolean): JSX.Element | null =>
    has("steps") ? (
      <StepButton source={source} isOverVideo={isOverVideo} isForward={isForward} />
    ) : null;
  const ending = (
    <>
      {has("fullScreen") ? <FullScreenButton videoRef={props.videoRef} /> : null}
      {has("sound") && source.sound !== undefined ? (
        <SoundControls sound={source.sound} isOverVideo={isOverVideo} />
      ) : null}
    </>
  );
  const asideSpan = <span className="player-controls__aside">{aside}</span>;
  return isOverlayRow === true ? (
    <div className="player-controls__buttons">
      <span className="player-controls__leading">
        {move(false)}
        {step(false)}
        {play}
        {step(true)}
        {move(true)}
      </span>
      {asideSpan}
      <span className="player-controls__trailing">
        {menus}
        {ending}
      </span>
    </div>
  ) : (
    <div className="player-controls__buttons">
      {move(false)}
      {play}
      {step(false)}
      {step(true)}
      {move(true)}
      {asideSpan}
      {ending}
    </div>
  );
}

/** Play or pause, as the film stands. */
function PlayButton({
  source,
  isOverVideo,
  playRef,
}: {
  readonly source: PlayerSource;
  readonly isOverVideo: boolean | undefined;
  readonly playRef: RefObject<HTMLButtonElement | null> | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <IconButton
      ref={playRef}
      isOverVideo={isOverVideo}
      label={source.isPaused ? t("playerControls.play") : t("playerControls.pause")}
      icon={source.isPaused ? <PlayIcon /> : <PauseIcon />}
      isDisabled={!source.isReady}
      onClick={source.playOrPause}
    />
  );
}

/**
 * A step back or forward, seekStepMs from where the film shows, never before the start nor past the
 * end: presses in a row add up, as the source shows each one's place at once.
 */
function StepButton(props: {
  readonly source: PlayerSource;
  readonly isOverVideo: boolean | undefined;
  readonly isForward: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, isForward } = props;
  const byMs = isForward ? seekStepMs : -seekStepMs;
  return (
    <IconButton
      isOverVideo={props.isOverVideo}
      label={isForward ? t("playerControls.forward") : t("playerControls.back")}
      icon={isForward ? <SeekForwardIcon /> : <SeekBackIcon />}
      isDisabled={!source.isReady}
      onClick={() => {
        const mostMs = source.durationMs > 0 ? source.durationMs : Number.POSITIVE_INFINITY;
        source.seekTo(Math.min(Math.max(0, source.atMs + byMs), mostMs));
      }}
    />
  );
}

/** Previous or next in the sequence, which plays that video (SequenceMoves). */
function MoveButton(props: {
  readonly isOverVideo: boolean | undefined;
  readonly isForward: boolean;
  readonly onMove: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <IconButton
      isOverVideo={props.isOverVideo}
      label={props.isForward ? t("playerControls.next") : t("playerControls.previous")}
      icon={props.isForward ? <NextIcon /> : <PreviousIcon />}
      onClick={props.onMove}
    />
  );
}

/**
 * Mute, and the volume where the page can set it: beside it in the bar, or upright in the overlay's
 * 🔈 pop-up (isUpright; player-menus.tsx, D407 §10, Q11).
 */
export function SoundControls(props: {
  readonly sound: PlayerSound;
  readonly isOverVideo: boolean | undefined;
  readonly isUpright?: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound, isOverVideo } = props;
  const isSilent = sound.isMuted || sound.volume === 0;
  return (
    <span
      className={
        props.isUpright === true
          ? "player-controls__sound player-controls__sound--upright"
          : "player-controls__sound"
      }
    >
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
          isUpright={props.isUpright}
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
 * The video in videoRef as a bar's source: where it stands, read at its own events, and at each
 * timeupdate only while the bar shows (isHidden false), or where a seek chosen on it sends it
 * (seekAheadOf); play or pause and seeks made on it at once; and its sound.
 */
export function useVideoSource(
  videoRef: RefObject<HTMLVideoElement | null>,
  isHidden = false,
): PlayerSource {
  const stand = useStanding(videoRef, isHidden);
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
        seekAheadOf(video).aim(atMs, 0, askedByOf("timeline", "bar"));
      }
    },
  };
}

/**
 * The video's events at which a bar reads where the film stands, hidden or shown: the first as soon
 * as it knows its length, and a seek chosen on it as it moves (seekAimedEvent).
 */
const standingEvents = [
  "loadedmetadata",
  "durationchange",
  "play",
  "pause",
  "seeked",
  seekAimedEvent,
];

/**
 * Where the film stands, its length, and whether it is paused, as the video says them: read at its
 * own events, and at each timeupdate, four a second, only while the bar shows (not isHidden); where
 * the film stands is a seek's place while it is chosen and on its way, until a seeked says it landed
 * (seekAheadOf). A bar
 * hidden over a playing film rendered again at every timeupdate for the film's length, 1 ms each on a
 * Fire TV (the efficiency audit's finding 7); shown again, it reads where the film stands at once.
 */
function useStanding(videoRef: RefObject<HTMLVideoElement | null>, isHidden: boolean): Standing {
  const standRef = useRef<Standing>({ atMs: 0, durationMs: 0, isPaused: true });
  const subscribe = useCallback(
    (changed: () => void) => {
      const video = videoRef.current;
      if (video === null) {
        return () => undefined;
      }
      const ahead = seekAheadOf(video);
      const heard = (): void => {
        standRef.current = {
          atMs: ahead.shownMs(video.currentTime * 1000),
          durationMs: Number.isFinite(video.duration) ? video.duration * 1000 : 0,
          isPaused: video.paused,
        };
        changed();
      };
      const events = isHidden ? standingEvents : [...standingEvents, "timeupdate"];
      for (const each of events) {
        video.addEventListener(each, heard);
      }
      if (!isHidden) {
        heard();
      }
      return () => {
        for (const each of events) {
          video.removeEventListener(each, heard);
        }
      };
    },
    [videoRef, isHidden],
  );
  const read = useCallback(() => standRef.current, []);
  return useSyncExternalStore(subscribe, read, read);
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
