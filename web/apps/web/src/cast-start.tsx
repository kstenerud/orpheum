// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastToReceive, Press } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useRef, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { askPageFullScreen, leaveFullScreenAsked } from "./film-fullscreen.ts";
import { useLatest } from "./latest.ts";
import { useToken } from "./services.tsx";
import { isTelevision } from "./tv-device.ts";
import { useTogether } from "./use-together.ts";

/** What a browser answered a request to put the page full screen. */
export type FullScreenAnswer = "entered" | "refused";

/**
 * The press that started a cast on a receiver whose page had not been pressed since it loaded (D468
 * §2, D358 §2): how it was made, and what the browser answered the full screen asked for in it,
 * undefined where none was asked, the page full screen already.
 */
export interface CastAnswer {
  readonly castId: number;
  readonly by: Press;
  readonly fullScreen: Promise<FullScreenAnswer> | undefined;
}

/**
 * How a click was pressed: by a key (a click a key made has no count, as a remote's OK on a button
 * makes), a touch's tap, or a mouse's click.
 */
export function pressOf(event: MouseEvent): Press {
  if (event.detail === 0) {
    return "key";
  }
  return "pointerType" in event && event.pointerType === "touch" ? "tap" : "click";
}

/**
 * Asks for the whole page full screen, from a press: the Fullscreen API needs one made moments
 * before (transient activation), which a cast's own start never is (idiosyncrasies.md). The page,
 * not the player's stage, which the Play page has not made yet; the stage fills the page then
 * (cast-receiver.tsx). Undefined where the page is full screen already.
 */
export function enterFullScreen(page: Document): Promise<FullScreenAnswer> | undefined {
  if (page.fullscreenElement instanceof Element) {
    return undefined;
  }
  if (typeof page.documentElement.requestFullscreen !== "function") {
    return Promise.resolve("refused");
  }
  return askPageFullScreen(page).then((isEntered): FullScreenAnswer =>
    isEntered ? "entered" : "refused",
  );
}

/** Leaves the full screen enterFullScreen asked for, where the page holds it. */
export function leaveFullScreen(page: Document): void {
  if (page.fullscreenElement === page.documentElement) {
    leaveFullScreenAsked(page);
  }
}

/** A line asking for a press, in the words of each way a device presses. */
export interface PressWords {
  readonly television: string;
  readonly touch: string;
  readonly pointer: string;
}

/**
 * Of words, those for the way this device presses: a television's remote ("Press OK"), a touch
 * screen's tap, or a pointer's click.
 */
export function inThisDevicesWords(words: PressWords): string {
  if (isTelevision(navigator.userAgent)) {
    return words.television;
  }
  const isTouch = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches;
  return isTouch ? words.touch : words.pointer;
}

/** "Press OK to start", in this device's words for a press. */
function startText(t: TFunction): string {
  return inThisDevicesWords({
    television: t("cast.start.television"),
    touch: t("cast.start.touch"),
    pointer: t("cast.start.pointer"),
  });
}

/**
 * A cast taken on a page never pressed since it loaded, held until a press starts it (D468 §2):
 * "Press OK to start" over the whole window, one button, focused and kept focused, so a remote's OK
 * answers it, as Enter, a click or a tap anywhere do. No other answer, and no time limit. Its press
 * is the one that lets every video of the session play with sound (sticky activation) and puts the
 * page full screen (transient activation, so asked in the press itself); onPressed is told it. A
 * cast that ends while it waits, its sender gone, closes it (onGone).
 */
export function CastStartPress(props: {
  readonly cast: CastToReceive;
  readonly onPressed: (answer: CastAnswer) => void;
  readonly onGone: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { cast, onPressed } = props;
  const buttonRef = useRef<HTMLButtonElement>(null);
  useEndsWithTheCast(cast, props.onGone);
  useEffect(() => {
    // Focused, and focused again wherever the focus goes after: the arrows reach nothing behind
    // it, so OK always answers it. Taken back once the focus has moved, as a focus() in the blur
    // itself is not kept everywhere.
    const button = buttonRef.current;
    button?.focus();
    const takeBack = (event: FocusEvent): void => {
      if (button !== null && event.target !== button) {
        button.focus();
      }
    };
    document.addEventListener("focusin", takeBack);
    return () => {
      document.removeEventListener("focusin", takeBack);
    };
  }, []);
  return (
    <button
      ref={buttonRef}
      type="button"
      className="cast-start"
      onClick={(event) => {
        onPressed({
          castId: cast.castId,
          by: pressOf(event.nativeEvent),
          fullScreen: enterFullScreen(document),
        });
      }}
    >
      {startText(t)}
    </button>
  );
}

/** Tells onGone once the cast ends, as its sender stops it while its receiver waits for its press. */
function useEndsWithTheCast(cast: CastToReceive, onGone: () => void): void {
  const token = useToken();
  const { snapshot } = useTogether({ credential: cast.membershipCredential, sessionToken: token });
  const isEnded = snapshot?.status === "ended";
  const goneRef = useLatest(onGone);
  useEffect(() => {
    if (isEnded) {
      goneRef.current();
    }
  }, [goneRef, isEnded]);
}
