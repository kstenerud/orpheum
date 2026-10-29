// SPDX-License-Identifier: MPL-2.0

/**
 * The app's input layer (controller-input-2026-10-25.md §6.1 item 1): every key, controller and
 * Media Session press becomes a command (input-buttons.ts), offered first to what has focus and what
 * holds it, as a DOM event that bubbles from the focused element, so an open menu, a dialog or the
 * player answers it; one none answers does what it does everywhere: an arrow moves focus
 * (focus-navigation.ts), OK presses what has focus, Back goes back in the app's history. Pages hear
 * commands, never keys.
 */

import { caretOf, isArrowTheElements, isFocusTarget } from "./focus-engine.ts";
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

/** What a command event carries. */
export interface CommandDetail {
  readonly command: Command;
  readonly source: CommandSource;
  /**
   * Whether it is a held key's repeat (KeyboardEvent.repeat), not a press of its own: absent for a
   * press. A controller's buttons never repeat; its stick's and D-pad's arrows do, as presses.
   */
  readonly isRepeat?: true;
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

/** Whether a key event is one the layer leaves alone: a shortcut with a modifier, or answered. */
function isLeftAlone(event: KeyboardEvent): boolean {
  return event.defaultPrevented || event.ctrlKey || event.altKey || event.metaKey;
}

/**
 * The command a key asks for where the layer takes it: none for one a text field or a range keeps (an
 * arrow inside its value, every key while it composes) or one with a modifier.
 */
function keysCommand(event: KeyboardEvent): Command | undefined {
  if (isLeftAlone(event) || event.isComposing) {
    return undefined;
  }
  const focused = document.activeElement;
  const input: HeardInput = {
    kind: "key",
    key: event.key,
    keyCode: keyCodeOf(event),
    isInField: focused !== null && caretOf(focused) !== undefined,
  };
  const command = commandOf(input);
  if (command === undefined) {
    return undefined;
  }
  if (isDirection(command) && focused !== null && isArrowTheElements(focused, command, false)) {
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
  const heardKey = (event: KeyboardEvent): void => {
    const command = keysCommand(event);
    if (command === undefined) {
      return;
    }
    const detail: CommandDetail = event.repeat
      ? { command, source: "key", isRepeat: true }
      : { command, source: "key" };
    if (offer(detail)) {
      event.preventDefault();
    }
  };
  const focused = (event: FocusEvent): void => {
    if (event.target instanceof Element) {
      memory.noteFocused(event.target);
    }
  };
  document.addEventListener("keydown", heardKey);
  document.addEventListener("focusin", focused);
  const stopPads = listenToPads({
    onPress: (press) => {
      const command = padsCommand(press);
      if (command !== undefined) {
        dispatch(command, "pad");
      }
    },
    onArrow: (direction) => {
      dispatch(direction, "pad");
    },
  });
  return {
    dispatch,
    lastSource: () => lastSource,
    memory,
    stop: () => {
      document.removeEventListener("keydown", heardKey);
      document.removeEventListener("focusin", focused);
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
 * Offers the browser's media controls' seeking (a keyboard's media keys, a headset's, the desktop's
 * controls) as commands while a film plays; their play and pause are the player's own
 * (`watchMediaSession` in @orpheum/player), which reports a pause they made. Returns the stopping.
 */
export function hearMediaSeeks(
  dispatch: (command: Command, source: CommandSource) => boolean,
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
  for (const [action, command] of seekActions) {
    handle(action, () => {
      dispatch(command, "media");
    });
  }
  return () => {
    for (const [action] of seekActions) {
      handle(action, null);
    }
  };
}
