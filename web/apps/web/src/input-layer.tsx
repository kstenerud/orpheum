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
import { arriveWhenDrawn, focusAndShow, scrollIntoViewGently } from "./focus-navigation.ts";
import type { Command } from "./input-buttons.ts";
import {
  hearMediaSeeks,
  onCommand,
  startInput,
  type CommandDetail,
  type CommandSource,
  type Input,
  type MediaTracks,
} from "./input-commands.ts";
import type { Navigation } from "./navigation.ts";
import { routeHref, type Route } from "./router.ts";
import { useServices } from "./services.tsx";
import type { Pages } from "./use-pages.ts";

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
 * How many frames a focus found again is kept in view while the rows around it settle: a few
 * scrolls' worth of rows taking their own height, at most half a second. A justified constant.
 */
const settlingFrames = 30;

/**
 * Scrolls element into view again at each frame it has moved since the last, while it keeps focus,
 * for settlingFrames frames: a list's rows off the screen are laid out at an estimate
 * until they near it (`content-visibility: auto`, browse.css), so a far tile scrolled to on a list Back
 * returned to moved as the rows around it took their own height, and again after each scroll, in
 * Firefox some 90 pixels below the screen's edge (the browser test, 2026-10-29); Chromium's scroll
 * anchoring hides most of it.
 */
function showAgainOnceLaidOut(element: HTMLElement): void {
  let last = element.getBoundingClientRect().top;
  let frames = 0;
  const check = (): void => {
    frames += 1;
    if (document.activeElement !== element || frames > settlingFrames) {
      return;
    }
    const top = element.getBoundingClientRect().top;
    if (top !== last) {
      scrollIntoViewGently(element);
      last = element.getBoundingClientRect().top;
    }
    requestAnimationFrame(check);
  };
  requestAnimationFrame(check);
}

/**
 * Focuses a page's arrival: shown as a key's focus is where a controller last pressed, and scrolled
 * gently into view where any key or controller did; for a pointer neither, so a page opened by a
 * click shows no ring (the browser's :focus-visible draws none after a pointer) and does not move.
 */
function focusArrival(element: HTMLElement, input: Input): void {
  const source = input.lastSource();
  if (source === undefined) {
    element.focus({ preventScroll: true });
  } else {
    focusAndShow(element, source === "pad");
  }
}

/**
 * Notes on the history entry what has focus as go leaves it, and gives focus back to the element so
 * noted when Back returns to the entry, once its page draws it (focus-memory.ts), shown as a key's
 * is where a controller pressed Back. A page go opens starts at its top: the window kept the scroll
 * of the page left, so a series chosen far down the Series list opened at its last episodes, past
 * what had focus (the owner's Fire TV, 2026-10-29); and focus is put on its arrival as it is drawn
 * (focus-navigation.ts arriveWhenDrawn), as on the page the app opens on.
 */
function keepFocusOnEntries(navigation: Navigation, input: Input): () => void {
  let isOpeningNew = false;
  const stopNoting = navigation.beforeLeaving(() => {
    isOpeningNew = true;
    const focused = document.activeElement;
    if (focused instanceof HTMLElement && focused !== document.body) {
      navigation.noteOnEntry(noteOf(focused, document.body));
    }
  });
  let shown = routeHref(navigation.route());
  const arrive = (): (() => void) =>
    arriveWhenDrawn(
      () => document.querySelector("main"),
      (element) => {
        focusArrival(element, input);
      },
    );
  let stopRestoring = arrive();
  const stopListening = navigation.subscribe(() => {
    const now = routeHref(navigation.route());
    const isNew = isOpeningNew;
    isOpeningNew = false;
    if (now === shown) {
      return;
    }
    shown = now;
    stopRestoring();
    if (isNew) {
      window.scrollTo({ top: 0, left: 0 });
      stopRestoring = arrive();
      return;
    }
    const note = asFocusNote(navigation.entryNote());
    if (note !== undefined) {
      stopRestoring = restoreWhenDrawn(note, document.body, (element) => {
        focusAndShow(element, input.lastSource() === "pad");
        showAgainOnceLaidOut(element);
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
    // The browser's own Back at the base laid under a page opened directly goes where its Back does.
    const stopBase = navigation.onBase(() => {
      navigation.replace(backUpRef.current ?? { screen: "home" });
    });
    inputRef.current = started;
    return () => {
      inputRef.current = undefined;
      stopBase();
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

/**
 * Offers the browser's media controls' seeking as commands while mounted, and their previous and
 * next track while tracks has them (input-commands.ts).
 */
export function useMediaSeeks(tracks: MediaTracks = {}): void {
  const told = use(InputContext);
  const { hasPrevious, hasNext } = tracks;
  useEffect(
    () =>
      told === undefined
        ? undefined
        : hearMediaSeeks(told.dispatch, {
            hasPrevious: hasPrevious === true,
            hasNext: hasNext === true,
          }),
    [told, hasPrevious, hasNext],
  );
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

/**
 * Loads a list's pages, once its first is drawn, until it holds the entry whose link had focus when
 * its page was left, where Back returns to it, so the input layer's watch for that link
 * (keepFocusOnEntries) finds it and focuses it: a list returned to draws its first page alone, and
 * a series on the Series list's second page left Back at the list's top, nothing focused (the
 * owner's Fire TV, 2026-10-29). hrefOf is an entry's link's address.
 */
export function useListLoadsToFocusNoted<T>(pages: Pages<T>, hrefOf: (item: T) => string): void {
  const { navigation } = useServices();
  const { loadedSoFar, loadUntil } = pages;
  const isLookedRef = useRef(false);
  useEffect(() => {
    if (isLookedRef.current || loadedSoFar.state !== "loaded") {
      return;
    }
    isLookedRef.current = true;
    const href = asFocusNote(navigation.entryNote())?.href;
    if (href !== undefined) {
      void loadUntil((item) => hrefOf(item) === href);
    }
  }, [loadedSoFar, loadUntil, hrefOf, navigation]);
}
