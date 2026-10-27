// SPDX-License-Identifier: MPL-2.0

// Apart from watch-together.tsx, whose page loads only when it is first shown: the tray's sign holds
// the details at hand on every screen, and loads no player for it (the audit of 2026-10-25, W1).

import type { Api } from "@orpheum/sdk";
import { useEffect } from "react";

import { tabStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import type { SessionDetails } from "./together-people.tsx";

/**
 * The last details of each session this tab hosts, by its id: the session's page shows them the
 * moment it opens, and the server's answer replaces them as it comes. Opened, it waited for the
 * server first, and the owner found it slow: "Ideally that should be instantaneous" (2026-10-12).
 */
const knownDetails = new Map<number, SessionDetails>();

/** Asks for a session's details, and keeps them as its last known. */
export async function detailsOf(api: Api, sessionId: number): Promise<SessionDetails> {
  const details = dataOrThrow(
    await api.GET("/together/{id}", { params: { path: { id: sessionId } } }),
  );
  knownDetails.set(sessionId, details);
  return details;
}

/** The last details known of the session sessionId, or undefined. */
export function knownDetailsOf(sessionId: number): SessionDetails | undefined {
  return knownDetails.get(sessionId);
}

/** Has the details of the session this tab hosts at hand before its page opens (knownDetails). */
export function usePrefetchedHostedDetails(): void {
  const { api } = useServices();
  const membership = useMembership(tabStorage, "host");
  const sessionId = membership?.sessionId;
  useEffect(() => {
    if (sessionId !== undefined && !knownDetails.has(sessionId)) {
      detailsOf(api, sessionId).catch(() => undefined); // the controls ask again as they open
    }
  }, [api, sessionId]);
}
