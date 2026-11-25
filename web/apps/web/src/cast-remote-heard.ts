// SPDX-License-Identifier: MPL-2.0

/**
 * Whether the cast remote hears its receiver (D550 §2, D553 §5), apart from the remote's page
 * (cast-remote.tsx): what its Reaching state and its not-answering note key on.
 */

import { leaderOf, type CastSending, type TogetherSnapshot } from "@orpheum/sdk";
import { useState } from "react";

import { useCastSessionLost } from "./cast-session-lost.ts";
import { useLive } from "./live.tsx";

/**
 * Whether the receiver is heard (D550 §2): its device among the person's connected devices (the live
 * connection's, told once it has said any), and, once a device was heard leading the cast's session,
 * one still leading it. The remote's own connection stands with a television switched off, so it
 * alone does not say. A receiver still loading, never heard leading, is not gone. A guest receiver
 * (a Chromecast's, D465 §1) is in no devices message, so it is heard by its session, and not once
 * this page's Cast session with it was lost with no Stop (D553 §5: useCastSessionLost), a hint the
 * device is gone that ends no cast: the server's silence limit ends it as its receiver goes.
 */
export function useReceiverHeard(
  sent: CastSending,
  snapshot: TogetherSnapshot | undefined,
): boolean {
  const { connectedDeviceIds } = useLive();
  const isSessionLost = useCastSessionLost(sent.castId);
  const isLeading = snapshot !== undefined && leaderOf(snapshot) !== undefined;
  const [hasLed, setHasLed] = useState(false);
  if (isLeading && !hasLed) {
    setHasLed(true); // kept as it renders, so the render that loses the leader already knows
  }
  const isDeviceGone = sent.isGuestReceiver
    ? isSessionLost
    : connectedDeviceIds.length > 0 && !connectedDeviceIds.includes(sent.receiverDeviceId);
  const isLeaderGone = hasLed && !isLeading;
  return !isDeviceGone && !isLeaderGone;
}
