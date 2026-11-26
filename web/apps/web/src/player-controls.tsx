// SPDX-License-Identifier: MPL-2.0

import "./player-controls.css";
import "./play-words.ts";

import { heardVolumeOf, setHeardVolume } from "@orpheum/player";
import { startGateWaitChangedEvent } from "@orpheum/sdk";
import {
  createContext,
  use,
  useEffect,
  useState,
  type Context,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { FullScreenButton } from "./full-screen-button.tsx";
import { isPausedAsAsked } from "./film-waits.tsx";
import { useIsVolumeSettable } from "./film-volume-slider.tsx";
import { IconButton } from "./icon-button.tsx";
import { useSideTapsOf } from "./phone-side-taps.tsx";
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
import type { PlaybackTarget, PlayerSound, PlayerSource } from "./playback-target.ts";
import { askedByOf, seekAheadOf, seekAimedEvent, seekStepMs } from "./seek-ahead.ts";
import { TargetTimeline } from "./target-timeline.tsx";
import { useVideoStanding, type StandingTaken, type VideoStanding } from "./video-standing.ts";
import { VolumeRange } from "./volume-range.tsx";

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
  /** Whether play and pause is larger than the other buttons: the overlay's (D555 §8). */
  readonly isPlayLarger?: boolean | undefined;
  /** What previous and next play, where the controls hold them. */
  readonly moves?: SequenceMoves | undefined;
  /** The overlay row's menus' openers, 💬 🎧 ⁝ (player-menus.tsx), before full screen. */
  readonly menus?: ReactNode | undefined;
  /** The film's video, whose stage full screen puts full screen; none, the stage the bar is in. */
  readonly videoRef?: RefObject<HTMLVideoElement | null> | undefined;
}): JSX.Element {
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
        isPlayLarger={props.isPlayLarger}
        moves={moves}
        menus={menus}
        videoRef={props.videoRef}
      />
      {controls.includes("timeline") ? (
        <TargetTimeline
          target={source}
          inputRef={props.timelineRef}
          hasArrowsAsCommands={props.hasArrowsAsCommands}
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
  readonly isPlayLarger: boolean | undefined;
  readonly moves: SequenceMoves | undefined;
  readonly menus: ReactNode;
  readonly videoRef: RefObject<HTMLVideoElement | null> | undefined;
}

/**
 * The bar's buttons among the controls given (PlayerControls): in the bar's order, or as the
 * overlay's row, previous, the steps with play between them and next in a leading group (a move
 * there is none of keeping its place), and full screen and the sound trailing.
 */
function ControlButtons(props: ButtonsParts): JSX.Element {
  const { source, controls, isOverVideo, aside, isOverlayRow, moves, menus } = props;
  const has = (control: PlayerControl): boolean => controls.includes(control);
  const move = (isForward: boolean): JSX.Element | null => {
    const go = isForward ? moves?.next : moves?.previous;
    if (has(isForward ? "next" : "previous") && go !== undefined) {
      return <SequenceMoveButton isOverVideo={isOverVideo} isForward={isForward} onMove={go} />;
    }
    // The overlay's row keeps a move's place where there is none, so the others stand where they
    // stand with both: "The center buttons should always behave in placement as if all 5 were
    // always available" (the owner, 2026-11-23).
    return isOverlayRow === true ? (
      <span className="player-controls__move-place" aria-hidden="true" />
    ) : null;
  };
  const play = has("play") ? <PlayButton {...props} /> : null;
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
        <span className="player-controls__centre">
          {step(false)}
          {play}
          {step(true)}
        </span>
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

/**
 * The overlay's play and pause, in px: the round size a step above the phone's centre buttons (56 px,
 * player-overlay.css), larger than every other button (D555 §8). A justified constant, a size.
 */
const largerPlayPx = 64;

/** Play or pause, as the film stands. */
/** Play or pause, larger than the other buttons where isPlayLarger, the overlay's (D555 §8). */
function PlayButton({
  source,
  isOverVideo,
  playRef,
  isPlayLarger,
}: Pick<ButtonsParts, "source" | "isOverVideo" | "playRef" | "isPlayLarger">): JSX.Element {
  const { t } = useTranslation();
  return (
    <IconButton
      ref={playRef}
      isOverVideo={isOverVideo}
      label={source.isPaused ? t("playerControls.play") : t("playerControls.pause")}
      icon={source.isPaused ? <PlayIcon /> : <PauseIcon />}
      isDisabled={!source.isReady}
      size={isPlayLarger === true ? largerPlayPx : undefined}
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
        seekStepped(source, byMs);
      }}
    />
  );
}

/**
 * Seeks source byMs from where it stands, kept within the film: the bar's ⟲ and ⟳, over the video
 * and over the castee (play-castee.tsx).
 */
export function seekStepped(source: PlayerSource, byMs: number): void {
  const mostMs = source.durationMs > 0 ? source.durationMs : Number.POSITIVE_INFINITY;
  source.seekTo(Math.min(Math.max(0, source.atMs + byMs), mostMs));
}

/**
 * Previous or next in the sequence, which plays that video (SequenceMoves): the bar's (D471).
 */
function SequenceMoveButton(props: {
  readonly isOverVideo: boolean | undefined;
  readonly isForward: boolean;
  readonly onMove: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { buttonRef, onClick } = useSideTapsOf(props.isForward, props.onMove);
  return (
    <IconButton
      ref={buttonRef}
      isOverVideo={props.isOverVideo}
      label={props.isForward ? t("playerControls.next") : t("playerControls.previous")}
      icon={props.isForward ? <NextIcon /> : <PreviousIcon />}
      onClick={onClick}
    />
  );
}

/**
 * The class of the box round SoundControls' mute, the speaker that 🔈's pop-up lays over its button
 * (player-menus.tsx): a name to find it by; the box is the button's own (player-controls.css).
 */
export const muteClass = "player-controls__mute";

/**
 * Mute, and the volume where the page can set it: beside it in the bar, or upright in the overlay's
 * 🔈 pop-up (isUpright; player-menus.tsx, D407 §10, Q11).
 */
export function SoundControls(props: {
  readonly sound: PlayerSound;
  readonly isOverVideo: boolean | undefined;
  readonly isUpright?: boolean | undefined;
  /** Whether focus can come to the slider (VolumeRange's isFocusable). */
  readonly isRangeFocusable?: boolean | undefined;
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
      <span className={muteClass}>
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
      </span>
      {sound.canSetVolume ? (
        <VolumeRange
          label={t("playerControls.volume")}
          value={sound.isMuted ? 0 : sound.volume}
          most={1}
          isUpright={props.isUpright}
          isFocusable={props.isRangeFocusable}
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

/**
 * Where a video stands, its length, and whether it is paused, as it says them at its own events;
 * where it stands is a seek's place while one is chosen (atMs), and the video's own (reportedAtMs).
 */
interface Standing {
  readonly atMs: number;
  readonly reportedAtMs: number;
  readonly durationMs: number;
  readonly isPaused: boolean;
}

/**
 * What the controls over the video here may command: play and pause, and seeks. Every page's video
 * takes both, but a Watch Together guest's, whose link's rights say which they have (D271 §4): their
 * Play page draws only the controls those give (D555 §8, §10), its group undoing any other change
 * (followGroup).
 */
export interface CommandRights {
  readonly canPlayPause: boolean;
  readonly canSeek: boolean;
}

/** The rights of the controls over the video here (CommandRights): every one, but a guest's. */
export const CommandRightsContext: Context<CommandRights> = createContext<CommandRights>({
  canPlayPause: true,
  canSeek: true,
});

/**
 * The video in videoRef as the playback target here (PlaybackTarget, D555 §8): where it stands, read
 * at its own events, and at each timeupdate only while the bar shows (isHidden false), or where a
 * seek chosen on it sends it (seekAheadOf); play or pause and seeks made on it at once; its sound;
 * and its choices, the film's tracks around its stage (HereChoices).
 */
export function useVideoTarget(
  videoRef: RefObject<HTMLVideoElement | null>,
  isHidden = false,
): PlaybackTarget {
  const stand = useStanding(videoRef, isHidden);
  const sound = useVideoSound(videoRef);
  const rights = use(CommandRightsContext);
  return {
    ...stand,
    kind: "here",
    choices: { kind: "here", videoRef },
    isReady: true,
    canSeek: rights.canSeek,
    canPlayPause: rights.canPlayPause,
    sound,
    playOrPause: () => {
      const video = videoRef.current;
      if (video !== null && rights.canPlayPause) {
        answerPlayerCommand(video, "playPause");
      }
    },
    seekTo: (atMs) => {
      const video = videoRef.current;
      if (video !== null && rights.canSeek) {
        seekAheadOf(video).aim(atMs, 0, askedByOf("timeline", "bar"));
      }
    },
  };
}

/**
 * The video's events at which a bar reads where the film stands, hidden or shown: the first as soon
 * as it knows its length, a seek chosen on it as it moves (seekAimedEvent), and a failed load, which
 * ends a wait to play (isPausedAsAsked).
 */
const standingEvents = [
  "loadedmetadata",
  "error",
  "durationchange",
  "play",
  "pause",
  "seeked",
  seekAimedEvent,
  startGateWaitChangedEvent,
];

/** The events at which a shown bar reads where the film stands: its hidden ones and each timeupdate. */
const shownStandingEvents = [...standingEvents, "timeupdate"];

/** The bar's standing from the video's reading: where it stands is a seek's place while one is chosen. */
function barStandingOf(standing: VideoStanding, video: HTMLVideoElement): Standing {
  return {
    atMs: seekAheadOf(video).shownMs(standing.atMs),
    reportedAtMs: standing.atMs,
    durationMs: standing.durationMs,
    // A film meant to play that waits for enough to be loaded shows Pause (isPausedAsAsked).
    isPaused: standing.isPaused && isPausedAsAsked(video),
  };
}

function isSameStanding(one: Standing, other: Standing): boolean {
  return (
    one.atMs === other.atMs &&
    one.reportedAtMs === other.reportedAtMs &&
    one.durationMs === other.durationMs &&
    one.isPaused === other.isPaused
  );
}

/** The bar's taking of the video's reading while it shows, read at once as it shows. */
const shownTaking: StandingTaken<Standing> = {
  take: barStandingOf,
  isSame: isSameStanding,
  initial: { atMs: 0, reportedAtMs: 0, durationMs: 0, isPaused: true },
  isReadAtOnce: true,
};

/** The bar's taking while it is hidden: at its own events alone. */
const hiddenTaking: StandingTaken<Standing> = { ...shownTaking, isReadAtOnce: false };

/**
 * Where the film stands, its length, and whether it is paused, as the video says them, from the one
 * reading every hearer of the video takes (useVideoStanding): read at its own events, and at each
 * timeupdate, four a second, only while the bar shows (not isHidden); where the film stands is a
 * seek's place while it is chosen and on its way, until a seeked says it landed (seekAheadOf). A bar
 * hidden over a playing film rendered again at every timeupdate for the film's length, 1 ms each on a
 * Fire TV (the efficiency audit's finding 7); shown again, it reads where the film stands at once.
 */
function useStanding(videoRef: RefObject<HTMLVideoElement | null>, isHidden: boolean): Standing {
  return useVideoStanding(
    videoRef,
    isHidden ? standingEvents : shownStandingEvents,
    isHidden ? hiddenTaking : shownTaking,
  );
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
      // The volume the person set, not the element's, which a lowering by levelling is in (D436).
      setHeard({ volume: heardVolumeOf(video), isMuted: video.muted });
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
        setHeardVolume(video, volume);
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
