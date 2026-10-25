// SPDX-License-Identifier: MPL-2.0

/**
 * The input layer in the app (input-commands.ts): started once around every screen, signed in or
 * not, with Back going back in the app's history as the page's own Back does, and focus found again
 * on a page Back returns to.
 */

import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";

import { asFocusNote, noteOf, restoreWhenDrawn } from "./focus-memory.ts";
import { focusAndShow } from "./focus-navigation.ts";
import type { Command } from "./input-buttons.ts";
import {
  hearMediaSeeks,
  onCommand,
  startInput,
  type CommandDetail,
  type CommandSource,
  type Input,
} from "./input-commands.ts";
import type { Navigation } from "./navigation.ts";
import { routeHref, type Route } from "./router.ts";
import { useServices } from "./services.tsx";

/** What a screen tells the input layer. */
interface InputTold {
  /** Where the page's own Back goes up to where the history has no entry of this app's before. */
  readonly backUpRef: RefObject<Route | undefined>;
  /** Turns the app's arrow navigation off until the returned function turns it on again. */
  readonly letBrowserNavigate: () => () => void;
  /** Offers a command as though pressed: the Media Session's seeking. */
  readonly dispatch: (command: Command, source: CommandSource) => boolean;
}

/** The input layer's ways, where one runs; a screen rendered alone in a test has none. */
const InputContext = createContext<InputTold | undefined>(undefined);

/**
 * Notes on the history entry what has focus as go leaves it, and gives focus back to the element so
 * noted when Back returns to the entry, once its page draws it (focus-memory.ts), shown as a key's
 * is where a controller pressed Back.
 */
function keepFocusOnEntries(navigation: Navigation, input: Input): () => void {
  const stopNoting = navigation.beforeLeaving(() => {
    const focused = document.activeElement;
    if (focused instanceof HTMLElement && focused !== document.body) {
      navigation.noteOnEntry(noteOf(focused, document.body));
    }
  });
  let shown = routeHref(navigation.route());
  let stopRestoring = (): void => undefined;
  const stopListening = navigation.subscribe(() => {
    const now = routeHref(navigation.route());
    if (now === shown) {
      return;
    }
    shown = now;
    stopRestoring();
    const note = asFocusNote(navigation.entryNote());
    if (note !== undefined) {
      stopRestoring = restoreWhenDrawn(note, document.body, (element) => {
        focusAndShow(element, input.lastSource() === "pad");
      });
    }
  });
  return () => {
    stopNoting();
    stopListening();
    stopRestoring();
  };
}

/**
 * Runs the input layer around children: keys, controllers, the app's Back and focus kept with each
 * history entry.
 */
export function InputLayer(props: { readonly children: ReactNode }): JSX.Element {
  // One layer a page: a test's services give one, around an app that starts its own.
  const outer = use(InputContext);
  return outer === undefined ? (
    <RunningInputLayer>{props.children}</RunningInputLayer>
  ) : (
    <>{props.children}</>
  );
}

/** The input layer's one run, with what screens tell it. */
function RunningInputLayer(props: { readonly children: ReactNode }): JSX.Element {
  const { navigation } = useServices();
  const backUpRef = useRef<Route | undefined>(undefined);
  const offCountRef = useRef(0);
  const inputRef = useRef<Input | undefined>(undefined);
  useEffect(() => {
    const started = startInput({
      goBack: () => navigation.back(backUpRef.current),
      isNavigating: () => offCountRef.current === 0,
    });
    const stopKeeping = keepFocusOnEntries(navigation, started);
    inputRef.current = started;
    return () => {
      inputRef.current = undefined;
      stopKeeping();
      started.stop();
    };
  }, [navigation]);
  const told = useMemo<InputTold>(
    () => ({
      backUpRef,
      letBrowserNavigate: () => {
        offCountRef.current += 1;
        return () => {
          offCountRef.current -= 1;
        };
      },
      dispatch: (command, source) => inputRef.current?.dispatch(command, source) ?? false,
    }),
    [],
  );
  return <InputContext value={told}>{props.children}</InputContext>;
}

/**
 * Tells the input layer where the page's Back goes up to (the BackButton's up), so a remote's Back
 * goes where the page's own does while the page shows.
 */
export function useBackTarget(up: Route): void {
  const told = use(InputContext);
  const href = routeHref(up);
  const upRef = useRef(up);
  useEffect(() => {
    upRef.current = up;
  });
  useEffect(() => {
    if (told === undefined) {
      return undefined;
    }
    const { backUpRef } = told;
    backUpRef.current = upRef.current;
    return () => {
      if (backUpRef.current === upRef.current) {
        backUpRef.current = undefined;
      }
    };
  }, [told, href]);
}

/** Leaves the arrows to the browser's own navigation while isOn: the device lab's measure of it. */
export function useBrowsersNavigation(isOn: boolean): void {
  const told = use(InputContext);
  useEffect(
    () => (isOn && told !== undefined ? told.letBrowserNavigate() : undefined),
    [told, isOn],
  );
}

/** Offers the browser's media controls' seeking as commands while mounted (input-commands.ts). */
export function useMediaSeeks(): void {
  const told = use(InputContext);
  useEffect(() => (told === undefined ? undefined : hearMediaSeeks(told.dispatch)), [told]);
}

/**
 * Hears the commands offered at target (an element's ref, or the document) while mounted, the latest
 * handler each time; capture hears them before what has focus does.
 */
export function useCommands(
  target: RefObject<Element | null> | "document",
  handler: (detail: CommandDetail, event: Event) => void,
  options: { readonly isCapture?: boolean } = {},
): void {
  const handlerRef = useRef(handler);
  useEffect(() => {
    handlerRef.current = handler;
  });
  const isCapture = options.isCapture === true;
  useEffect(() => {
    const element = target === "document" ? document : target.current;
    if (element === null) {
      return undefined;
    }
    return onCommand(
      element,
      (detail, event) => {
        handlerRef.current(detail, event);
      },
      { isCapture },
    );
  }, [target, isCapture]);
}
