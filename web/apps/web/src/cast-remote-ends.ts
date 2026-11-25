// SPDX-License-Identifier: MPL-2.0

/**
 * How a cast's remote meets its cast's end (D552 §4, D446 §2): the way, the film taken over at once
 * where this tab asked the end, the remote kept with its note for any other, or the page sent home
 * where nothing was on; and the note, why it ended.
 */

import "./cast-words.ts";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { endAskedOf } from "./cast.ts";
import type { RemoteEnded } from "./cast-remote-offer.tsx";

/**
 * The ends of a cast with nothing on after which the page goes home with no word, as the film would
 * have played here (D446 §2): any other, the server knowing the cast no more, goes home saying so
 * (D386 §2), or with its Watch Together session, with no word (D389 §1).
 */
const nothingOnTakenOverAt: ReadonlySet<string> = new Set(["stopped", "takenBack", "receiverGone"]);

/**
 * How the remote meets its cast's end (D552 §4): the film played here at once where this tab asked
 * the end, its Stop casting (asked, "stopped"; and a cast with nothing on sends the page home the
 * same way, nothingOnTakenOverAt, D446 §2); with a film on, any other end, the receiver's take-back,
 * its loss, the server's or another tab's, keeps the remote, its note saying why, Play here the one
 * way on (ended); with nothing on, the page sent home (lost: D386 §2, D389 §1).
 */
function endWayOf(
  reason: string,
  fileId: number,
  asked: string | undefined,
): "takenOver" | "ended" | "lost" {
  if (fileId === 0) {
    return nothingOnTakenOverAt.has(reason) ? "takenOver" : "lost";
  }
  return reason === "stopped" && asked === "stopped" ? "takenOver" : "ended";
}

/**
 * How the remote meets its cast's end, as endWayOf, read once as the end arrives: whether this tab
 * asked it is forgotten as the film is taken over (takeEndAnswerOf), and the way must not change.
 */
export function useEndWay(
  castId: number,
  reason: string | undefined,
  fileId: number,
): "takenOver" | "ended" | "lost" | undefined {
  return useMemo(
    () => (reason === undefined ? undefined : endWayOf(reason, fileId, endAskedOf(castId))),
    [castId, reason, fileId],
  );
}

/**
 * The remote's note for an end this tab did not ask (D552 §4): the receiver took the film back,
 * "‹TV› stopped casting"; it is gone, the server's end or this sender's on losing its Chromecast,
 * "‹TV› unreachable"; any other, "Casting ended". The owner, 2026-11-25: "Let's keep the reasoning
 * text short". Play here plays the film here as an ordinary play (playHere); undefined while the
 * cast lasts or ends another way (end: its way, useEndWay, and the end itself, over).
 */
export function useRemoteEnded(
  end: {
    readonly way: "takenOver" | "ended" | "lost" | undefined;
    readonly over: { readonly reason: string } | undefined;
  },
  receiverName: string,
  playHere: () => void,
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
  return { text, playHere };
}
