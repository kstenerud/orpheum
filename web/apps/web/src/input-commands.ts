// SPDX-License-Identifier: MPL-2.0

/**
 * The app's input layer (controller-input-2026-10-25.md §6.1 item 1): every key, controller and
 * Media Session press becomes a command (input-buttons.ts), offered first to what has focus and what
 * holds it, as a DOM event that bubbles from the focused element, so an open menu, a dialog or the
 * player answers it; one none answers does what it does everywhere: an arrow moves focus
 * (focus-navigation.ts), OK presses what has focus, Back goes back in the app's history. Each command
 * a key or a controller's button or stick asks says whether it starts a hold or continues one, and
 * each hold's end is told as a release (CommandHold), the same from every device: the keys' driver
 * here, the controllers' in input-pad-poll.ts. Pages hear commands and releases, never keys.
 */

import { noteForTheLog } from "@orpheum/sdk";

import { caretOf, isArrowTheElements, isFocusTarget } from "./focus-engine.ts";
import { guardFormsOk } from "./form-ok.ts";
import { createContainerMemory, type ContainerMemory } from "./focus-memory.ts";
import { moveFocus, pressFocused } from "./focus-navigation.ts";
import {
  commandOf,
  isDirection,
  keyCodeOf,
  type Command,
  type HeardInput,
} from "./input-buttons.ts";
import { listenToPads, type PadButtonPress, type StickPress } from "./input-pad-poll.ts";

/** Where a command came from: a key, a game controller, or the browser's media controls. */
export type CommandSource = "key" | "pad" | "media";

/**
 * Where a command stands in the holding of the key or button that asks it: "start" as it goes down,
 * "continue" at each repeat while it stays down. Each device's driver tells it alike, whatever the
 * device sends (the owner, 2026-11-04: the UI knows nothing of a controller's quirks): a key's repeat
 * whether the browser marks it or not (Silk on a Fire TV sends a held key's repeats unmarked, VQ122),
 * a controller's stick and D-pad as their arrows repeat while held, and its seek buttons (rewind and
 * fast forward) at the same delay and interval; its other buttons start a hold but never repeat, so
 * a held OK acts once. Each hold ends in a release (onCommandRelease).
 */
export type CommandHold = "start" | "continue";

/** What a command event carries. */
export interface CommandDetail {
  readonly command: Command;
  readonly source: CommandSource;
  /** How it is held (CommandHold); absent for a press nothing holds, a media control's or a click's. */
  readonly hold?: CommandHold;
}

/** What a release carries: the command its key or button asked as it went down, and from where. */
export interface CommandRelease {
  readonly command: Command;
  readonly source: CommandSource;
}

/** The name of the DOM event a command is offered as. */
export const commandEventName = "orpheum-command";

/** Whether event is a command event, and so carries a command. */
function isCommandEvent(event: Event): event is CustomEvent<CommandDetail> {
  return event instanceof CustomEvent && event.type === commandEventName;
}

/**
 * Hears every command offered at or inside target, or at the document, that nothing answered yet;
 * handler answers one by calling preventDefault on its event, which keeps it from every handler after
 * and from what it does everywhere. Capture hears it before what has focus does: an open menu, over
 * everything. Returns the stopping.
 */
export function onCommand(
  target: EventTarget,
  handler: (detail: CommandDetail, event: Event) => void,
  options: { readonly isCapture?: boolean } = {},
): () => void {
  const hear = (event: Event): void => {
    if (isCommandEvent(event) && !event.defaultPrevented) {
      handler(event.detail, event);
    }
  };
  const isCapture = options.isCapture === true;
  target.addEventListener(commandEventName, hear, isCapture);
  return () => {
    target.removeEventListener(commandEventName, hear, isCapture);
  };
}

/** The name of the DOM event a hold's release is told as, at the window (onCommandRelease). */
export const commandReleaseEventName = "orpheum-command-release";

/** Whether event is a release event, and so carries a release. */
function isReleaseEvent(event: Event): event is CustomEvent<CommandRelease> {
  return event instanceof CustomEvent && event.type === commandReleaseEventName;
}

/**
 * Hears each release of a held command (CommandHold) told at target, the window:
 * its key or button went up, or the page lost the keys while it was down. Returns the stopping.
 */
export function onCommandRelease(
  target: EventTarget,
  handler: (release: CommandRelease) => void,
): () => void {
  const hear = (event: Event): void => {
    if (isReleaseEvent(event)) {
      handler(event.detail);
    }
  };
  target.addEventListener(commandReleaseEventName, hear);
  return () => {
    target.removeEventListener(commandReleaseEventName, hear);
  };
}

/** Tells release at the window. */
function tellRelease(release: CommandRelease): void {
  window.dispatchEvent(new CustomEvent(commandReleaseEventName, { detail: release }));
}

/**
 * The keyboard's and a remote's driver of holds: which keys are down, each with the command it went
 * down as, so that a key's every keydown after its first is its repeat, marked by the browser or not
 * (Silk's are not, VQ122), and its keyup, or the page losing the keys, is its release.
 */
interface KeyHolds {
  /** How a keydown asking command holds it: its key's start, or its repeat. */
  readonly down: (event: KeyboardEvent, command: Command) => CommandHold;
  /** The command a keyup releases, where its key was held. */
  readonly up: (event: KeyboardEvent) => Command | undefined;
  /** Releases every key held: the page lost the keys, and will hear none of their keyups. */
  readonly upAll: () => Command[];
}

/** A key's name for its holding: its key and keyCode, the same at its keydown and keyup. */
function heldKeyOf(event: KeyboardEvent): string {
  return `${event.key}/${String(keyCodeOf(event))}`;
}

/**
 * How long after a held key's last keydown its next is still its repeat, in ms: past it, its keyup
 * was lost and the key is pressed again. A remote's first repeat comes 400–500 ms after its press
 * (starts-chunks-remote-2026-10-29.md §3.3), so a second is well past any. A justified constant.
 */
export const keyRepeatsWithinMs = 1000;

/** A key held: the command it went down as, and when it last went down, in performance.now's ms. */
interface HeldKey {
  readonly command: Command;
  readonly lastDownMs: number;
}

/** Holds no key yet. */
function createKeyHolds(): KeyHolds {
  const held = new Map<string, HeldKey>();
  return {
    down: (event, command) => {
      const key = heldKeyOf(event);
      const nowMs = performance.now();
      const lastDownMs = held.get(key)?.lastDownMs;
      const isHeld = lastDownMs !== undefined && nowMs - lastDownMs <= keyRepeatsWithinMs;
      held.set(key, { command, lastDownMs: nowMs });
      return isHeld || event.repeat ? "continue" : "start";
    },
    up: (event) => {
      const key = heldKeyOf(event);
      const command = held.get(key)?.command;
      held.delete(key);
      return command;
    },
    upAll: () => {
      const commands = [...held.values()].map((each) => each.command);
      held.clear();
      return commands;
    },
  };
}

/** What the input layer does with a command no one answered. */
export interface InputWays {
  /** Goes back in the app's history as the page's own Back does; whether it went anywhere. */
  readonly goBack: () => boolean;
  /**
   * Whether the app's navigation moves focus on arrows: off while the device lab measures the
   * browser's own.
   */
  readonly isNavigating: () => boolean;
}

/** The input layer running, as the app's shell uses it. */
export interface Input {
  /** Offers command to what has focus, then does what it does everywhere; whether anything did. */
  readonly dispatch: (command: Command, source: CommandSource) => boolean;
  /** Where the last command came from, for focus a page restores to be shown as a key's is. */
  readonly lastSource: () => CommandSource | undefined;
  readonly memory: ContainerMemory;
  readonly stop: () => void;
}

/** What a command no one answered does: whether it did anything. */
function actByDefault(detail: CommandDetail, ways: InputWays, memory: ContainerMemory): boolean {
  const { command, source } = detail;
  if (isDirection(command)) {
    return ways.isNavigating() && moveFocus(command, { memory }, source === "pad");
  }
  if (command === "select") {
    return pressFocused(source === "pad");
  }
  return command === "back" ? ways.goBack() : false;
}

/** The element a command is offered at: what has focus, else the page's body. */
function commandTarget(): Element {
  const focused = document.activeElement;
  return focused?.isConnected === true ? focused : document.body;
}

/**
 * Whether a key event is one the layer leaves alone: answered, a shortcut with a modifier (AltGr
 * apart from Ctrl and Alt, which Windows sends it as and a Mac's browser need not), or one an input
 * method is composing, which an old Android browser marks only by keyCode 229 (D407 §10).
 */
function isLeftAlone(event: KeyboardEvent): boolean {
  return (
    event.defaultPrevented ||
    event.ctrlKey ||
    event.altKey ||
    event.metaKey ||
    event.getModifierState("AltGraph") ||
    event.isComposing ||
    keyCodeOf(event) === 229
  );
}

/** Whether typing into element writes: a text field, a number field or editable content. */
function isEditable(element: Element): boolean {
  return (
    caretOf(element) !== undefined ||
    (element instanceof HTMLInputElement && element.type === "number") ||
    (element instanceof HTMLElement && element.isContentEditable)
  );
}

/**
 * Whether command is one the focused element keeps: an arrow inside a text field's or a range's
 * value, or + and - typed into a field (D407 §10).
 */
function isFocusedOwn(command: Command, focused: Element | null): boolean {
  if (focused === null) {
    return false;
  }
  if (isDirection(command)) {
    return isArrowTheElements(focused, command, false);
  }
  return (command === "volumeUp" || command === "volumeDown") && isEditable(focused);
}

/**
 * The command a key asks for where the layer takes it: none for one a text field or a range keeps (an
 * arrow inside its value, + and - typed, every key while it composes) or one with a modifier.
 */
function keysCommand(event: KeyboardEvent): Command | undefined {
  if (isLeftAlone(event)) {
    return undefined;
  }
  const focused = document.activeElement;
  const input: HeardInput = {
    kind: "key",
    key: event.key,
    keyCode: keyCodeOf(event),
    isInField: focused !== null && caretOf(focused) !== undefined,
    code: event.code,
    isShifted: event.shiftKey,
  };
  const command = commandOf(input);
  if (command === undefined || isFocusedOwn(command, focused)) {
    return undefined;
  }
  return command;
}

/** The command a controller's button asks for, but its D-pad's, which comes as an arrow. */
function padsCommand(press: PadButtonPress | StickPress): Command | undefined {
  if (press.kind !== "pad") {
    return undefined;
  }
  const command = commandOf({
    kind: "pad",
    button: press.button,
    isStandard: press.isStandard,
    okButton: press.okButton,
  });
  return command === undefined || isDirection(command) ? undefined : command;
}

/**
 * Whether command is Up or Down on a focused range: never the range's own, so where no focus moves
 * that way the browser must not take it either, which steps the range's value: Down at the bottom of
 * the Fire TV's player controls, from its scrub bar, put the film back a second at each press (the
 * owner, staging, 2026-10-29).
 */
function isRangesVertical(command: Command): boolean {
  const focused = document.activeElement;
  return (
    (command === "up" || command === "down") &&
    focused instanceof HTMLInputElement &&
    focused.type === "range"
  );
}

/** Offers detail at what has focus; whether something acted on it, the layer by default included. */
type Offer = (detail: CommandDetail) => boolean;

/**
 * The key driver: offers each key's command as offer says, its hold started or continued
 * (createKeyHolds), and tells its release at its keyup, or when the page loses the keys. Returns the
 * stopping.
 */
function listenToKeys(offer: Offer): () => void {
  const keyHolds = createKeyHolds();
  const heardKey = (event: KeyboardEvent): void => {
    const command = keysCommand(event);
    if (command === undefined) {
      return;
    }
    const hold = keyHolds.down(event, command);
    if (offer({ command, source: "key", hold }) || isRangesVertical(command)) {
      event.preventDefault();
    }
  };
  const keyUp = (event: KeyboardEvent): void => {
    const command = keyHolds.up(event);
    if (command !== undefined) {
      tellRelease({ command, source: "key" });
    }
  };
  const keysLost = (): void => {
    for (const command of keyHolds.upAll()) {
      tellRelease({ command, source: "key" });
    }
  };
  document.addEventListener("keydown", heardKey);
  // A keyup is a release whatever answered it: captured, so none is kept from the driver.
  document.addEventListener("keyup", keyUp, true);
  window.addEventListener("blur", keysLost);
  return () => {
    document.removeEventListener("keydown", heardKey);
    document.removeEventListener("keyup", keyUp, true);
    window.removeEventListener("blur", keysLost);
  };
}

/**
 * The controllers' driver: offers each button's command and each arrow of a stick or D-pad, a
 * button's hold started at its press and continued at its repeats (the seeking buttons', as an arrow's), and tells each release
 * as the poll sees it (input-pad-poll.ts). Returns the stopping.
 */
function listenToPadHolds(offer: Offer): () => void {
  return listenToPads({
    onPress: (press) => {
      const command = padsCommand(press);
      if (command !== undefined) {
        offer({ command, source: "pad", hold: "start" });
      }
    },
    onButtonRepeat: (press) => {
      const command = padsCommand(press);
      if (command !== undefined) {
        offer({ command, source: "pad", hold: "continue" });
      }
    },
    onButtonUp: (press) => {
      const command = padsCommand(press);
      if (command !== undefined) {
        tellRelease({ command, source: "pad" });
      }
    },
    onArrow: (direction, isRepeat) => {
      offer({ command: direction, source: "pad", hold: isRepeat ? "continue" : "start" });
    },
    onArrowUp: (direction) => {
      tellRelease({ command: direction, source: "pad" });
    },
  });
}

/**
 * Starts the input layer: keys, controllers and each focus noted in its container's memory. A key the
 * layer acted on has the browser's own handling prevented, so on a TV's browser our navigation alone
 * moves focus (Silk's spatial mode stops its own move then, idiosyncrasies.md); one it did nothing
 * with is the browser's.
 */
export function startInput(ways: InputWays): Input {
  const memory = createContainerMemory(isFocusTarget);
  let lastSource: CommandSource | undefined;
  const offer = (detail: CommandDetail): boolean => {
    lastSource = detail.source;
    const event = new CustomEvent<CommandDetail>(commandEventName, {
      bubbles: true,
      cancelable: true,
      detail,
    });
    commandTarget().dispatchEvent(event);
    return event.defaultPrevented || actByDefault(event.detail, ways, memory);
  };
  const dispatch = (command: Command, source: CommandSource): boolean => offer({ command, source });
  const focused = (event: FocusEvent): void => {
    if (event.target instanceof Element) {
      memory.noteFocused(event.target);
    }
  };
  const stopKeys = listenToKeys(offer);
  document.addEventListener("focusin", focused);
  // OK on a form's checkbox ticks it before the layer hears it, and never submits (form-ok.ts).
  const stopFormsOk = guardFormsOk(
    { target: window, focused: () => document.activeElement },
    noteForTheLog,
  );
  const stopPads = listenToPadHolds(offer);
  return {
    dispatch,
    lastSource: () => lastSource,
    memory,
    stop: () => {
      stopKeys();
      document.removeEventListener("focusin", focused);
      stopFormsOk();
      stopPads();
    },
  };
}

/** The Media Session actions that seek, by the command each is. */
const seekActions: readonly (readonly [MediaSessionAction, Command])[] = [
  ["seekbackward", "seekBack"],
  ["seekforward", "seekForward"],
];

/**
 * Which of the browser's previous and next track buttons to take: each only while there is a video
 * to go to, since Chrome shows a button once its action is registered (D407 §10).
 */
export interface MediaTracks {
  readonly hasPrevious?: boolean;
  readonly hasNext?: boolean;
}

/** The Media Session actions hearMediaSeeks registers: the seeks, and the tracks asked for. */
function mediaActionsOf(tracks: MediaTracks): (readonly [MediaSessionAction, Command])[] {
  return [
    ...seekActions,
    ...(tracks.hasPrevious === true ? [["previoustrack", "previous"] as const] : []),
    ...(tracks.hasNext === true ? [["nexttrack", "next"] as const] : []),
  ];
}

/**
 * Offers the browser's media controls' seeking (a keyboard's media keys, a headset's, the desktop's
 * controls) as commands while a film plays, and their previous and next track where tracks asks for
 * them; their play and pause are the player's own (`watchMediaSession` in @orpheum/player), which
 * reports a pause they made. Returns the stopping.
 */
export function hearMediaSeeks(
  dispatch: (command: Command, source: CommandSource) => boolean,
  tracks: MediaTracks = {},
): () => void {
  if (!("mediaSession" in navigator)) {
    return () => undefined;
  }
  const session = navigator.mediaSession;
  const handle = (action: MediaSessionAction, handler: (() => void) | null): void => {
    try {
      session.setActionHandler(action, handler);
    } catch {
      // A browser that knows no such action keeps its own answer.
    }
  };
  const actions = mediaActionsOf(tracks);
  for (const [action, command] of actions) {
    handle(action, () => {
      dispatch(command, "media");
    });
  }
  return () => {
    for (const [action] of actions) {
      handle(action, null);
    }
  };
}
