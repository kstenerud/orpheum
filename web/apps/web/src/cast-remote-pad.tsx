// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";
import "./cast-words.ts";

import {
  useEffect,
  useRef,
  type JSX,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import { caretOf } from "./focus-engine.ts";
import { IconButton, type IconButtonSize } from "./icon-button.tsx";
import { useCommands } from "./input-layer.tsx";
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
import { seekStepMs } from "./seek-ahead.ts";
import { remoteVolumeStep, volumeAfter } from "./volume-step.ts";

/** How long Volume down or up is held before it repeats, in ms (D548 §6). A justified constant. */
export const heldRepeatsAfterMs = 500;

/** How often a held Volume down or up repeats, in ms (D548 §6). A justified constant. */
export const heldRepeatsEveryMs = 300;

/** What the remote's volume does: a step down or up, and Mute; each shows the volume's Pill. */
export interface RemoteSoundMoves {
  readonly isMuted: boolean;
  readonly step: (direction: -1 | 1) => void;
  readonly toggleMute: () => void;
}

/**
 * The remote's volume over the film's level on the receiver, sound (D548 §6): a step of
 * remoteVolumeStep down or up, within 0 and 1, a step up turning a muted film's sound on first, as
 * the player's volume key does; Mute turning it off or on. Each tells onShown, which shows the
 * volume's Pill. Undefined where the receiver reports no level.
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
 * The cast remote's pad (D548 §5): three equal columns, each button centred in its cell. Previous,
 * an empty cell, Next; Back 10 s, Play or Pause, Forward 10 s; Volume down, Mute, Volume up. A
 * button the receiver cannot do leaves its cell empty and out of the focus order, nothing else
 * moving: Previous where an episode plays before, Next where one plays next (moves); the volume's
 * row is gone where the receiver reports no level (sound). While the receiver is not reached
 * (isDisabled) every button is aria-disabled, a press doing nothing, and so is every key (§9). The
 * remote's keys are the pad's (useRemoteKeys, §10).
 */
export function RemotePad(props: {
  readonly source: PlayerSource;
  readonly moves: SequenceMoves;
  readonly sound: RemoteSoundMoves | undefined;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { source, moves, sound, isDisabled } = props;
  useRemoteKeys(source, sound, isDisabled);
  return (
    <div className="cast-remote__pad" data-focus-container="">
      <MovesRow moves={moves} isDisabled={isDisabled} />
      <PlayRow source={source} isDisabled={isDisabled} />
      {sound === undefined ? null : <VolumeRow sound={sound} isDisabled={isDisabled} />}
    </div>
  );
}

/** The pad's first row: Previous, an empty cell, Next, each cell empty where it cannot move. */
function MovesRow(props: {
  readonly moves: SequenceMoves;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { moves, isDisabled } = props;
  return (
    <>
      <PadCell>
        {moves.previous === undefined ? null : (
          <PadButton
            size={64}
            label={t("playerControls.previous")}
            icon={<PreviousIcon />}
            isDisabled={isDisabled}
            onPress={moves.previous}
          />
        )}
      </PadCell>
      <PadCell>{null}</PadCell>
      <PadCell>
        {moves.next === undefined ? null : (
          <PadButton
            size={64}
            label={t("playerControls.next")}
            icon={<NextIcon />}
            isDisabled={isDisabled}
            onPress={moves.next}
          />
        )}
      </PadCell>
    </>
  );
}

/** The pad's second row: Back 10 s, Play or Pause, Forward 10 s. */
function PlayRow(props: {
  readonly source: PlayerSource;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { source, isDisabled } = props;
  return (
    <>
      <PadCell>
        <PadButton
          size={72}
          label={t("playerControls.back")}
          icon={<SeekBackIcon />}
          isDisabled={isDisabled}
          onPress={() => {
            seekStepped(source, -seekStepMs);
          }}
        />
      </PadCell>
      <PadCell>
        <PadButton
          size={96}
          label={source.isPaused ? t("playerControls.play") : t("playerControls.pause")}
          icon={source.isPaused ? <PlayIcon /> : <PauseIcon />}
          isDisabled={isDisabled}
          onPress={source.playOrPause}
        />
      </PadCell>
      <PadCell>
        <PadButton
          size={72}
          label={t("playerControls.forward")}
          icon={<SeekForwardIcon />}
          isDisabled={isDisabled}
          onPress={() => {
            seekStepped(source, seekStepMs);
          }}
        />
      </PadCell>
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
  return <div className="cast-remote__pad-cell">{props.children}</div>;
}

/** A pad's round button on the film's card ground (D548 §5). */
function PadButton(props: {
  readonly size: IconButtonSize;
  readonly label: string;
  readonly icon: ReactNode;
  readonly isDisabled: boolean;
  readonly isPressed?: boolean;
  readonly onPress: (event: MouseEvent<HTMLButtonElement>) => void;
  readonly onPointerDown?: (event: PointerEvent<HTMLButtonElement>) => void;
}): JSX.Element {
  return (
    <IconButton
      size={props.size}
      label={props.label}
      icon={props.icon}
      isOverVideo
      isOnCard
      isPressed={props.isPressed}
      isUnavailable={props.isDisabled}
      onClick={props.onPress}
      onPointerDown={props.onPointerDown}
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

/** Whether focus is in a text field, whose keys are its own. */
function isInTextField(): boolean {
  const focused = document.activeElement;
  return focused !== null && caretOf(focused) !== undefined;
}

/** Whether nothing has focus: the page itself, its body. */
function isNothingFocused(): boolean {
  const focused = document.activeElement;
  return focused === null || focused === document.body;
}

/** Whether Space is a button's where focus is, which Space presses. */
function isOnAButton(): boolean {
  const focused = document.activeElement;
  return focused !== null && focused.closest('button, [role="button"]') !== null;
}

/** What the remote's keys act on now. */
interface RemoteKeysNow {
  readonly source: PlayerSource;
  readonly sound: RemoteSoundMoves | undefined;
  readonly isDisabled: boolean;
}

/** Whether the remote's keys are its own now: not while disabled, nor in a text field. */
function isKeysTime(now: RemoteKeysNow): boolean {
  return !now.isDisabled && !isInTextField();
}

/** The input layer's command, where it is the remote's: the volume's step, or a seek. */
function commandHeard(now: RemoteKeysNow, command: string, event: Event): void {
  if ((command === "volumeUp" || command === "volumeDown") && now.sound !== undefined) {
    event.preventDefault();
    now.sound.step(command === "volumeUp" ? 1 : -1);
  } else if ((command === "left" || command === "right") && isNothingFocused()) {
    event.preventDefault();
    seekStepped(now.source, command === "right" ? seekStepMs : -seekStepMs);
  }
}

/** A key the input layer names no command for, where it is the remote's: Space, and M. */
function keyHeard(now: RemoteKeysNow, event: KeyboardEvent): void {
  const isPlain = !event.ctrlKey && !event.altKey && !event.metaKey;
  if (event.defaultPrevented || !isPlain) {
    return;
  }
  if (event.key === " " && !isOnAButton()) {
    event.preventDefault();
    now.source.playOrPause();
  } else if ((event.key === "m" || event.key === "M") && now.sound !== undefined) {
    event.preventDefault();
    now.sound.toggleMute();
  }
}

/**
 * The remote's keys (D548 §10), the player's where it has them, none while the pad is disabled or
 * focus is in a text field: Space plays or pauses unless a button has focus, which Space presses;
 * Left and Right seek 10 s while nothing has focus (on the timeline they move it); + and − (the
 * input layer's volumeUp and volumeDown: +, =, the numpad's) step the volume; M is Mute, not with
 * Ctrl, Alt or Cmd. No media keys: a browser sends them only to a page playing sound itself.
 */
function useRemoteKeys(
  source: PlayerSource,
  sound: RemoteSoundMoves | undefined,
  isDisabled: boolean,
): void {
  const latestRef = useLatest<RemoteKeysNow>({ source, sound, isDisabled });
  useCommands(
    "document",
    (detail, event) => {
      if (isKeysTime(latestRef.current)) {
        commandHeard(latestRef.current, detail.command, event);
      }
    },
    { isCapture: true },
  );
  useEffect(() => {
    const pressed = (event: KeyboardEvent): void => {
      if (isKeysTime(latestRef.current)) {
        keyHeard(latestRef.current, event);
      }
    };
    document.addEventListener("keydown", pressed, true);
    return () => {
      document.removeEventListener("keydown", pressed, true);
    };
  }, [latestRef]);
}
