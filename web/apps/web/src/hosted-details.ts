// SPDX-License-Identifier: MPL-2.0

// Apart from watch-together.tsx, whose page loads only when it is first shown: the tray's sign holds
// the details at hand on every screen, and loads no player for it (the audit of 2026-10-25, W1); the
// tray's two signs as D364 §12 has them are seen on devices, VQ90.

import type { Api } from "@orpheum/sdk";
import { useEffect } from "react";

import { tabStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import type { SessionDetails } from "./together-people.tsx";
import { heldAnswer, rememberAnswer } from "./use-resource.ts";

/**
 * The key the details of the session sessionId are read by (useResource): the tab holds their last
 * answer, so the session's page shows them the moment it opens and the server's answer replaces them
 * as it comes (D478). Opened, it waited for the server first, and the owner found it slow: "Ideally
 * that should be instantaneous" (2026-10-12).
 */
export function hostedDetailsKey(sessionId: number): string {
  return `together/${String(sessionId)}`;
}

/** Asks for a session's details. */
export async function detailsOf(api: Api, sessionId: number): Promise<SessionDetails> {
  return dataOrThrow(await api.GET("/together/{id}", { params: { path: { id: sessionId } } }));
}

/** Has the details of the session this tab hosts at hand before its page opens (hostedDetailsKey). */
export function usePrefetchedHostedDetails(): void {
  const { api } = useServices();
  const membership = useMembership(tabStorage, "host");
  const sessionId = membership?.sessionId;
  useEffect(() => {
    if (sessionId === undefined || heldAnswer(hostedDetailsKey(sessionId)) !== undefined) {
      return;
    }
    detailsOf(api, sessionId).then(
      (details) => {
        rememberAnswer(hostedDetailsKey(sessionId), details);
      },
      () => undefined, // the controls ask again as they open
    );
  }, [api, sessionId]);
}
