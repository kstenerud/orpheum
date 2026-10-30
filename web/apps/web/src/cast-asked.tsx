// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastToReceive, Press } from "@orpheum/sdk";
import { useEffect, useRef, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { acceptCast, castAnswerWithinMs, endCast } from "./cast.ts";
import { askPageFullScreen, leaveFullScreenAsked } from "./film-fullscreen.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { useServices, useToken } from "./services.tsx";
import { useTogether } from "./use-together.ts";

/** What a browser answered a request to put the page full screen. */
export type FullScreenAnswer = "entered" | "refused";

/**
 * The press that answered Use to a cast (D364 §2, D358 §2): how it was made, and what the browser
 * answered the full screen asked for in it, undefined where none was asked, the page full screen
 * already.
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

/**
 * What closes a cast's question with no answer: castAnswerWithinMs gone by, the sender told it went
 * unanswered, or the cast ended, its sender gone. It focuses Use, play, as the question opens.
 */
function useUnansweredCastCloses(
  cast: CastToReceive,
  onAnswered: (answer: CastAnswer | undefined) => void,
  playRef: RefObject<HTMLButtonElement | null>,
): void {
  const { api } = useServices();
  const token = useToken();
  const { snapshot } = useTogether({ credential: cast.membershipCredential, sessionToken: token });
  const isEnded = snapshot?.status === "ended";
  const answeredRef = useRef(onAnswered);
  useEffect(() => {
    answeredRef.current = onAnswered;
  });
  useEffect(() => {
    playRef.current?.focus();
    const timer = setTimeout(() => {
      endCast(api, cast.castId, "unanswered").catch(() => undefined);
      answeredRef.current(undefined);
    }, castAnswerWithinMs);
    return () => {
      clearTimeout(timer);
    };
  }, [api, cast.castId, playRef]);
  useEffect(() => {
    if (isEnded) {
      answeredRef.current(undefined);
    }
  }, [isEnded]);
}

/**
 * A cast arrived, asked about once for its whole session (D364 §2, D358 §1–§4): "‹Device› wants to use
 * this device for playback", with Use focused, so a remote's OK answers it, and Not now. Use's press
 * is the one that lets every video of the session play with sound (sticky activation) and puts the
 * page full screen (transient activation, so asked in the press itself), and the server is told, so
 * the sender's Plays come here; Not now, Back or Escape decline it, and no answer within
 * castAnswerWithinMs leaves it unanswered, the sender told either way. A cast that ends while asked,
 * its sender gone, closes it.
 */
export function CastAsked(props: {
  readonly cast: CastToReceive;
  readonly onAnswered: (answer: CastAnswer | undefined) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { cast, onAnswered } = props;
  const playRef = useRef<HTMLButtonElement>(null);
  useUnansweredCastCloses(cast, onAnswered, playRef);
  const decline = (): void => {
    endCast(api, cast.castId, "declined").catch(() => undefined);
    onAnswered(undefined);
  };
  const question = t("cast.asked.question", { device: cast.fromDeviceName });
  return (
    <ModalDialog
      label={question}
      onClose={decline}
      className="cast-asked"
      // Closing declines the cast: a press beside the question must not answer it.
      shouldStayOpenOnOutsidePress
    >
      <p className="cast-asked__question">{question}</p>
      <div className="cast-asked__buttons">
        <button
          ref={playRef}
          type="button"
          onClick={(event) => {
            const fullScreen = enterFullScreen(document);
            acceptCast(api, cast.castId).catch(() => undefined); // its sender waits on it, and says so
            onAnswered({ castId: cast.castId, by: pressOf(event.nativeEvent), fullScreen });
          }}
        >
          {t("cast.asked.use")}
        </button>
        <button type="button" onClick={decline}>
          {t("cast.asked.notNow")}
        </button>
      </div>
    </ModalDialog>
  );
}
