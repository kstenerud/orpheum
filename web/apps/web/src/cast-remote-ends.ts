// SPDX-License-Identifier: MPL-2.0

/**
 * How a cast's remote meets its cast's end (D552 §4, D553, D446 §2): the way, the film taken over at
 * once where this tab asked the end, or where it hosts Watch Together and the receiver is gone, the
 * remote kept with its note for any other, or the page sent home where nothing was on; the note, why
 * it ended; and the host's notice.
 */

import "./cast-words.ts";

import { useMemo, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { endAskedOf } from "./cast.ts";
import type { CastAgain } from "./cast-remote-again.ts";
import type { RemoteEnded } from "./cast-remote-offer.tsx";
import { showNotice } from "./notice.ts";
import type { ReopenAt } from "./reopen.ts";

/**
 * The ends of a cast with nothing on after which the page goes home with no word, as the film would
 * have played here (D446 §2): any other, the server knowing the cast no more, goes home saying so
 * (D386 §2), or with its Watch Together session, with no word (D389 §1).
 */
const nothingOnTakenOverAt: ReadonlySet<string> = new Set(["stopped", "takenBack", "receiverGone"]);

/**
 * How the remote meets its cast's end (D552 §4, D553 §1–3): the film played here at once where this
 * tab asked the end, its Stop casting (asked, "stopped"; and a cast with nothing on sends the page
 * home the same way, nothingOnTakenOverAt, D446 §2), and where this tab hosts Watch Together and the
 * receiver is gone (isHosting, "receiverGone"), the session an obligation its host's player goes on
 * with (D553 §3); with a film on, any other end, the receiver's take-back, its loss alone, the
 * server's or another tab's, keeps the remote, its note saying why, Play here the way on (ended);
 * with nothing on, the page sent home (lost: D386 §2, D389 §1).
 */
function endWayOf(
  end: { readonly reason: string; readonly fileId: number; readonly isHosting: boolean },
  asked: string | undefined,
): "takenOver" | "ended" | "lost" {
  const { reason, fileId, isHosting } = end;
  if (fileId === 0) {
    return nothingOnTakenOverAt.has(reason) ? "takenOver" : "lost";
  }
  if (isHosting && reason === "receiverGone") {
    return "takenOver";
  }
  return reason === "stopped" && asked === "stopped" ? "takenOver" : "ended";
}

/**
 * How the remote meets its cast's end, as endWayOf, read once as the end arrives: whether this tab
 * asked it is forgotten as the film is taken over (takeEndAnswerOf), and the way must not change.
 */
export function useEndWay(
  castId: number,
  end: {
    readonly reason: string | undefined;
    readonly fileId: number;
    readonly isHosting: boolean;
  },
): "takenOver" | "ended" | "lost" | undefined {
  const { reason, fileId, isHosting } = end;
  return useMemo(
    () =>
      reason === undefined
        ? undefined
        : endWayOf({ reason, fileId, isHosting }, endAskedOf(castId)),
    [castId, reason, fileId, isHosting],
  );
}

/**
 * Plays the film here as the take-over does (playHere), and, where this tab hosts Watch Together
 * (isHosting) and the cast ended with its receiver gone, says over the Play page why it plays here
 * (D553 §3): "‹TV› unreachable. Playing here so your session goes on", a reminder of the session its
 * host is in, shown after the play, so a page moved to the cast's film shows it too. A notice takes
 * no action (notice.ts): Cast again is not in it, and Leave session is the Watch Together panel's.
 */
export function useTellsTheHost(
  playHere: (atMs: number | undefined, isPaused?: boolean) => void,
  end: { readonly reason: string | undefined; readonly isHosting: boolean; readonly name: string },
): (atMs: number | undefined, isPaused?: boolean) => void {
  const { t } = useTranslation();
  const isTold = end.isHosting && end.reason === "receiverGone";
  const text = t("cast.over.goesOn", { name: end.name });
  return (atMs, isPaused) => {
    playHere(atMs, isPaused);
    if (isTold) {
      showNotice(text);
    }
  };
}

/**
 * Plays the film here as an ordinary play from where the receiver last stood (stoodRef), or, not
 * heard, where the cast stood as it ended (over), playing or paused as it was: Play here after an end
 * this tab did not ask (D552 §2, §4); and from, that place, which Cast again casts from (D553 §2).
 */
export function playsFromWhereItStood(
  over: { readonly stood: ReopenAt | undefined } | undefined,
  where: {
    readonly stoodRef: RefObject<ReopenAt | undefined>;
    readonly playHere: (atMs: number | undefined, isPaused?: boolean) => void;
  },
): { readonly play: () => void; readonly from: () => ReopenAt | undefined } {
  const from = (): ReopenAt | undefined => where.stoodRef.current ?? over?.stood;
  return {
    from,
    play: () => {
      const at = from();
      where.playHere(at?.positionMs, at?.isPaused);
    },
  };
}

/**
 * The remote's note for an end this tab did not ask (D552 §4, D553 §2): the receiver took the film
 * back, "‹TV› took control"; it is gone, the server's end, "‹TV› unreachable"; any other, "Casting
 * ended". The owner, 2026-11-25: "Let's keep the reasoning text short". Play here plays the film here
 * as an ordinary play (ways.playHere), and Cast again, once the receiver is back (ways.castAgain),
 * casts it there anew; undefined while the cast lasts or ends another way (end: its way, useEndWay,
 * and the end itself, over).
 */
export function useRemoteEnded(
  end: {
    readonly way: "takenOver" | "ended" | "lost" | undefined;
    readonly over: { readonly reason: string } | undefined;
  },
  receiverName: string,
  ways: { readonly playHere: () => void; readonly castAgain: CastAgain | undefined },
): RemoteEnded | undefined {
  const { t } = useTranslation();
  if (end.way !== "ended" || end.over === undefined) {
    return undefined;
  }
  const { reason } = end.over;
  const name = receiverName;
  let text = t("cast.over.ended");
  if (reason === "takenBack") {
    text = t("cast.over.takenBack", { name });
  } else if (reason === "receiverGone") {
    text = t("cast.receiverGone", { name });
  }
  return { text, ...ways };
}
