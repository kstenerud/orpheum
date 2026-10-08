// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";
import { keepMembership, readMembership } from "./together-storage.ts";

/**
 * Has back, on this device, the Watch Together sessions the signed-in person is in (D267 §2): one
 * they host, for their screens and player, and one they joined signed in, for the viewer's page. A
 * membership this tab or browser keeps for the same session is left as it is; each device has a
 * credential of its own, so asking cuts off none of their others (§3).
 */
export async function haveBackMySessions(api: Api): Promise<void> {
  const { sessions } = dataOrThrow(await api.GET("/together/mine"));
  for (const each of sessions ?? []) {
    const role = each.isHost ? "host" : "viewer";
    const storage = each.isHost ? tabStorage() : browserStorage();
    if (readMembership(storage, role)?.sessionId !== each.sessionId) {
      keepMembership(storage, role, {
        sessionId: each.sessionId,
        credential: each.membershipCredential,
      });
    }
  }
}
