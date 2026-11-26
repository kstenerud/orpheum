// SPDX-License-Identifier: MPL-2.0

/*
 * The remote's layout's pieces (D548 §1, §4–§6), drawn by the castee's Play page and by the Play
 * page under the film held upright (the owner, 2026-11-26: "the one with the remote-control looking
 * layout (3x3 pad). That should be the layout for portrait and landscape mode when casting, and the
 * layout in portrait for local playback"): the timeline with the time under it at the start
 * (PadTimeline), the 3×3 pad (RemotePad), and the menus' row (PadMenus). Each page lays them out
 * (play-castee.css, page-controls.css); the keys are the page's own (useStageCommands,
 * usePlayersControls), not the pad's.
 */

import "./remote-pad.css";
import "./play-words.ts";
import "./cast-words.ts";

import {
  useEffect,
  useRef,
  type JSX,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { FullScreenButton } from "./full-screen-button.tsx";
import { IconButton, type IconButtonSize } from "./icon-button.tsx";
import { useLatest } from "./latest.ts";
import type { PlayerSound, PlayerSource } from "./playback-target.ts";
import { seekStepped, type SequenceMoves } from "./player-controls.tsx";
import {
  MutedIcon,
  NextIcon,
  PauseIcon,
  PlayIcon,
  PreviousIcon,
  SeekBackIcon,
  SeekForwardIcon,
  VolumeDownIcon,
  VolumeUpIcon,
} from "./player-icons.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { seekStepMs } from "./seek-ahead.ts";
import { isSeekable, TargetTimeline } from "./target-timeline.tsx";
import { remoteVolumeStep, volumeAfter } from "./volume-step.ts";

/** How long Volume down or up is held before it repeats, in ms (D548 §6). A justified constant. */
export const heldRepeatsAfterMs = 500;

/** How often a held Volume down or up repeats, in ms (D548 §6). A justified constant. */
export const heldRepeatsEveryMs = 300;

/** What the pad's volume does: a step down or up, and Mute; each shows the volume's Pill. */
export interface RemoteSoundMoves {
  readonly isMuted: boolean;
  readonly step: (direction: -1 | 1) => void;
  readonly toggleMute: () => void;
}

/**
 * The pad's volume over a film's level, sound (D548 §6): a step of remoteVolumeStep down or up,
 * within 0 and 1, a step up turning a muted film's sound on first, as the player's volume key does;
 * Mute turning it off or on. Each tells onShown, which shows the volume's Pill. Undefined where the
 * target says no level.
 */
export function remoteSoundMovesOf(
  sound: PlayerSound | undefined,
  onShown: () => void,
): RemoteSoundMoves | undefined {
  if (sound === undefined) {
    return undefined;
  }
  return {
    isMuted: sound.isMuted,
    step: (direction) => {
      if (direction === 1 && sound.isMuted) {
        sound.setMuted(false);
      }
      sound.setVolume(volumeAfter(sound.volume, direction, remoteVolumeStep));
      onShown();
    },
    toggleMute: () => {
      sound.setMuted(!sound.isMuted);
      onShown();
    },
  };
}

/**
 * The timeline the whole width, the time under it at the start (D548 §4), its slider's arrows the
 * player's seeks where hasArrowsAsCommands; disabled as isDisabled says, else while the target takes
 * no seek (TargetTimeline).
 */
export function PadTimeline(props: {
  readonly source: PlayerSource;
  readonly isDisabled?: boolean | undefined;
  readonly hasArrowsAsCommands: boolean;
}): JSX.Element {
  return (
    <div className="remote-pad__timeline" data-focus-container="">
      <TargetTimeline
        target={props.source}
        isDisabled={props.isDisabled}
        hasArrowsAsCommands={props.hasArrowsAsCommands}
        timesClassName="remote-pad__time"
      />
    </div>
  );
}

/**
 * The pad (D548 §5): three equal columns, each button centred in its cell. Previous, an empty cell,
 * Next; Back 10 s, Play or Pause, Forward 10 s; Volume down, Mute, Volume up. A button shows only
 * where pressing it can do something (the owner, 2026-11-26), its cell left empty and out of the
 * focus order, nothing else moving: Previous where something plays before, Next where something
 * plays next (moves), each as the page shows them (shown, controlsShown: a guest's rights, D555
 * §10); the volume's row is gone where the page sets no level (sound): a castee's is its film's level
 * there, a phone playing itself has none (D555 §10). While the target is not reached (isDisabled)
 * every button is aria-disabled, a press doing nothing (§9). Play is playRef, which the page focuses
 * as full screen ends.
 */
export function RemotePad(props: {
  readonly source: PlayerSource;
  readonly moves: SequenceMoves | undefined;
  readonly shown: readonly OverlayControl[];
  readonly sound: RemoteSoundMoves | undefined;
  readonly isDisabled: boolean;
  readonly playRef?: RefObject<HTMLButtonElement | null> | undefined;
}): JSX.Element {
  const { source, moves, shown, sound, isDisabled } = props;
  return (
    <div className="remote-pad" data-focus-container="">
      <MovesRow
        previous={shown.includes("previous") ? moves?.previous : undefined}
        next={shown.includes("next") ? moves?.next : undefined}
        isDisabled={isDisabled}
      />
      <PlayRow
        source={source}
        canSeek={shown.includes("seekBack") && isSeekable(source)}
        canPlay={shown.includes("play")}
        isDisabled={isDisabled}
        playRef={props.playRef}
      />
      {sound === undefined ? null : <VolumeRow sound={sound} isDisabled={isDisabled} />}
    </div>
  );
}

/** The pad's first row: Previous, an empty cell, Next, each cell empty where it cannot move. */
function MovesRow(props: {
  readonly previous: (() => void) | undefined;
  readonly next: (() => void) | undefined;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { previous, next, isDisabled } = props;
  return (
    <>
      <PadCell>
        {previous === undefined ? null : (
          <PadButton
            size={64}
            label={t("playerControls.previous")}
            icon={<PreviousIcon />}
            isDisabled={isDisabled}
            onPress={previous}
          />
        )}
      </PadCell>
      <PadCell>{null}</PadCell>
      <PadCell>
        {next === undefined ? null : (
          <PadButton
            size={64}
            label={t("playerControls.next")}
            icon={<NextIcon />}
            isDisabled={isDisabled}
            onPress={next}
          />
        )}
      </PadCell>
    </>
  );
}

/** The pad's second row: Back 10 s, Play or Pause, Forward 10 s, each cell empty where not shown. */
function PlayRow(props: {
  readonly source: PlayerSource;
  readonly canSeek: boolean;
  readonly canPlay: boolean;
  readonly isDisabled: boolean;
  readonly playRef: RefObject<HTMLButtonElement | null> | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, canSeek, isDisabled } = props;
  const step = (isForward: boolean): JSX.Element | null =>
    canSeek ? (
      <PadButton
        size={72}
        label={isForward ? t("playerControls.forward") : t("playerControls.back")}
        icon={isForward ? <SeekForwardIcon /> : <SeekBackIcon />}
        isDisabled={isDisabled}
        onPress={() => {
          seekStepped(source, isForward ? seekStepMs : -seekStepMs);
        }}
      />
    ) : null;
  return (
    <>
      <PadCell>{step(false)}</PadCell>
      <PadCell>
        {props.canPlay ? (
          <PadButton
            buttonRef={props.playRef}
            size={96}
            label={source.isPaused ? t("playerControls.play") : t("playerControls.pause")}
            icon={source.isPaused ? <PlayIcon /> : <PauseIcon />}
            isDisabled={isDisabled}
            onPress={source.playOrPause}
          />
        ) : null}
      </PadCell>
      <PadCell>{step(true)}</PadCell>
    </>
  );
}

/** The pad's third row: Volume down, held repeating; Mute, a toggle; Volume up, held repeating. */
function VolumeRow(props: {
  readonly sound: RemoteSoundMoves;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound, isDisabled } = props;
  const down = useHeldRepeats(() => {
    sound.step(-1);
  });
  const up = useHeldRepeats(() => {
    sound.step(1);
  });
  return (
    <>
      <PadCell>
        <PadButton
          size={64}
          label={t("cast.pad.volumeDown")}
          icon={<VolumeDownIcon />}
          isDisabled={isDisabled}
          {...down}
        />
      </PadCell>
      <PadCell>
        <PadButton
          size={64}
          label={t("playerControls.mute")}
          icon={<MutedIcon />}
          isDisabled={isDisabled}
          isPressed={sound.isMuted}
          onPress={sound.toggleMute}
        />
      </PadCell>
      <PadCell>
        <PadButton
          size={64}
          label={t("cast.pad.volumeUp")}
          icon={<VolumeUpIcon />}
          isDisabled={isDisabled}
          {...up}
        />
      </PadCell>
    </>
  );
}

/** One of the pad's nine cells, its button centred in it; empty where it has none. */
function PadCell(props: { readonly children: ReactNode }): JSX.Element {
  return <div className="remote-pad__cell">{props.children}</div>;
}

/** A pad's round button on the film's card ground (D548 §5). */
function PadButton(props: {
  readonly size: IconButtonSize;
  readonly label: string;
  readonly icon: ReactNode;
  readonly isDisabled: boolean;
  readonly isPressed?: boolean;
  readonly buttonRef?: RefObject<HTMLButtonElement | null> | undefined;
  readonly onPress: (event: MouseEvent<HTMLButtonElement>) => void;
  readonly onPointerDown?: (event: PointerEvent<HTMLButtonElement>) => void;
}): JSX.Element {
  const { buttonRef, size, label, icon, isPressed, isDisabled, onPress, onPointerDown } = props;
  return (
    <IconButton
      ref={buttonRef}
      size={size}
      label={label}
      icon={icon}
      isOverVideo
      isOnCard
      isPressed={isPressed}
      isUnavailable={isDisabled}
      onClick={onPress}
      onPointerDown={onPointerDown}
    />
  );
}

/**
 * A button's press that repeats while held (D548 §6): once at a click, as a keyboard's Enter or a
 * short tap presses it; held down, again after heldRepeatsAfterMs and every heldRepeatsEveryMs
 * after, until the pointer lifts, leaves the button or is cancelled. A pointer's click after a hold
 * that repeated is its end, not one press more; a keyboard's click (detail 0) is always a press.
 */
function useHeldRepeats(press: () => void): {
  readonly onPress: (event: MouseEvent<HTMLButtonElement>) => void;
  readonly onPointerDown: (event: PointerEvent<HTMLButtonElement>) => void;
} {
  const pressRef = useLatest(press);
  const stopRef = useRef<(() => void) | undefined>(undefined);
  const hasRepeatedRef = useRef(false);
  useEffect(
    () => () => {
      stopRef.current?.();
    },
    [],
  );
  return {
    onPress: (event) => {
      if (hasRepeatedRef.current && event.detail > 0) {
        hasRepeatedRef.current = false;
        return;
      }
      pressRef.current();
    },
    onPointerDown: (event) => {
      stopRef.current?.();
      hasRepeatedRef.current = false;
      const button = event.currentTarget;
      let repeats: ReturnType<typeof setInterval> | undefined;
      const wait = setTimeout(() => {
        hasRepeatedRef.current = true;
        pressRef.current();
        repeats = setInterval(() => {
          pressRef.current();
        }, heldRepeatsEveryMs);
      }, heldRepeatsAfterMs);
      const stop = (): void => {
        clearTimeout(wait);
        clearInterval(repeats);
        button.removeEventListener("pointerleave", stop);
        window.removeEventListener("pointerup", stop, true);
        window.removeEventListener("pointercancel", stop, true);
        stopRef.current = undefined;
      };
      button.addEventListener("pointerleave", stop);
      window.addEventListener("pointerup", stop, true);
      window.addEventListener("pointercancel", stop, true);
      stopRef.current = stop;
    },
  };
}

/**
 * The menus' row at the end (D548 §7), right-aligned as every bottom row of buttons is (the owner,
 * 2026-11-26): the menus given, then, where fullScreenOf is given, ⤢, which puts the stage it holds,
 * or the one it is in, full screen, where the overlay is the controls (D555 §9). A castee's has none:
 * casting is never full screen (the owner, 2026-11-26).
 */
export function PadMenus(props: {
  readonly menus: ReactNode;
  readonly fullScreenOf?: RefObject<HTMLElement | null> | undefined;
}): JSX.Element {
  return (
    <div className="remote-pad__menus" data-focus-container="">
      {props.menus}
      {props.fullScreenOf === undefined ? null : (
        <FullScreenButton inStageRef={props.fullScreenOf} />
      )}
    </div>
  );
}
