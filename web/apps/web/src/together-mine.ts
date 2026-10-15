// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";
import { forgetMembership, keepMembership, readMembership } from "./together-storage.ts";

/**
 * Forgets the session this tab hosts when it is still the one hosted as the server was asked,
 * askedId, and the server's sessions do not include it as this person's to host.
 */
function forgetHostedIfGone(
  askedId: number | undefined,
  sessions: readonly { readonly sessionId: number; readonly isHost: boolean }[],
): void {
  const hosted = readMembership(tabStorage(), "host");
  if (hosted === undefined || hosted.sessionId !== askedId) {
    return;
  }
  if (!sessions.some((each) => each.isHost && each.sessionId === hosted.sessionId)) {
    forgetMembership(tabStorage(), "host");
  }
}

/**
 * Has back, on this device, the Watch Together sessions the signed-in person is in (D267 §2): one
 * they host, for their screens and player, and one they joined signed in, for the viewer's page. A
 * membership this tab or browser keeps for the same session is left as it is; each device has a
 * credential of its own, so asking cuts off none of their others (§3). A session this tab hosts that
 * the server no longer has, ended or lost with a restart, is forgotten: its sign stayed in the status
 * tray, a session gone (the owner's Android, 2026-10-11).
 */
export async function haveBackMySessions(api: Api): Promise<void> {
  // The session hosted as the server was asked: one started while it answered is not forgotten.
  const asked = readMembership(tabStorage(), "host");
  const { sessions } = dataOrThrow(await api.GET("/together/mine"));
  forgetHostedIfGone(asked?.sessionId, sessions ?? []);
  for (const each of sessions ?? []) {
    const role = each.isHost ? "host" : "viewer";
    const storage = each.isHost ? tabStorage() : browserStorage();
    if (readMembership(storage, role)?.sessionId !== each.sessionId) {
      keepMembership(storage, role, {
        sessionId: each.sessionId,
        credential: each.membershipCredential,
        locator: each.locator,
      });
    }
  }
}
