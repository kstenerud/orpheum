// SPDX-License-Identifier: MPL-2.0

/**
 * Whether the cast remote hears its receiver (D550 §2, D553 §5, D554 §4), read by the Play page
 * over the castee (play-castee.tsx): what its Reaching state and its giving the receiver up key on.
 */

import { leaderOf, type CastSending, type TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, useState } from "react";

import { tellCastSessionLinked, useCastSessionLost } from "./cast-session-lost.ts";
import { useLive } from "./live.tsx";

/**
 * Whether the receiver is heard (D550 §2): its device among the person's connected devices (the live
 * connection's, told once it has said any), and, once a device was heard leading the cast's session,
 * one still leading it. The remote's own connection stands with a television switched off, so it
 * alone does not say. A receiver still loading, never heard leading, is not gone. A guest receiver
 * (a Chromecast's, D465 §1) is in no devices message, so it is heard by its session: this page's
 * Cast session with it lost with no Stop (D553 §5: useCastSessionLost) is a hint the server
 * overrides (D554 §4), counted only while the cast's session shows no leader (its one leader is the
 * receiver). A phone in airplane mode lost its Cast session with no Stop while the Chromecast played
 * on, leading (the owner, 2026-11-25); so the hint, the leader heard after it, is forgotten, and a
 * later loss starts from clean.
 */
export function useReceiverHeard(
  sent: CastSending,
  snapshot: TogetherSnapshot | undefined,
): boolean {
  const { connectedDeviceIds } = useLive();
  const isLeading = snapshot !== undefined && leaderOf(snapshot) !== undefined;
  const isHintLost = useCastSessionLostUnled(sent, isLeading);
  const [hasLed, setHasLed] = useState(false);
  if (isLeading && !hasLed) {
    setHasLed(true); // kept as it renders, so the render that loses the leader already knows
  }
  const isDeviceGone = sent.isGuestReceiver
    ? isHintLost
    : connectedDeviceIds.length > 0 && !connectedDeviceIds.includes(sent.receiverDeviceId);
  const isLeaderGone = hasLed && !isLeading;
  return !isDeviceGone && !isLeaderGone;
}

/**
 * Whether a guest receiver's Cast session was lost (useCastSessionLost) while the cast's session
 * shows no leader (isLeading): the hint the server overrides (D554 §4), forgotten as the receiver
 * is heard leading after it.
 */
function useCastSessionLostUnled(sent: CastSending, isLeading: boolean): boolean {
  const isSessionLost = useCastSessionLost(sent.castId);
  const isOverridden = sent.isGuestReceiver && isSessionLost && isLeading;
  useEffect(() => {
    if (isOverridden) {
      tellCastSessionLinked(sent.castId);
    }
  }, [isOverridden, sent.castId]);
  return isSessionLost && !isLeading;
}
