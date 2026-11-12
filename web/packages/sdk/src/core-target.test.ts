// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { targetOfGuestGrant, type HeldSession } from "./core-target.ts";
import { guestGrantOfferKey, sessionOfferKey, type DiscoveryClient } from "./discovery.ts";

const core = { coreId: "k7xq2", fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw" };

/** A discovery that finds coreId by any locator, and keeps what each call was sent. */
function discoveryOf(coreId: string, sent: unknown[]): DiscoveryClient {
  return {
    clientApi: "https://discovery.example/client/v1",
    fetch: (_url, init) => {
      sent.push(JSON.parse(typeof init?.body === "string" ? init.body : "null"));
      return Promise.resolve(new Response(JSON.stringify({ coreId })));
    },
  };
}

describe("a Google Cast device's target, holding a grant for one cast (D465 §2)", () => {
  it("finds the sender's core by the sender's locator, its first offers proven by the grant", async () => {
    const sent: unknown[] = [];
    const held = { core, grant: "a guest grant", senderLocator: "the-senders-locator" };
    const target = await targetOfGuestGrant(discoveryOf("k7xq2", sent), held, () => undefined);
    expect(sent).toEqual([expect.objectContaining({ locator: "the-senders-locator" })]);
    expect(target.held).toEqual({ locator: "the-senders-locator" });
    expect(target.key).toEqual(await guestGrantOfferKey("a guest grant"));
    // Until the trade, every offer is the grant's: the device holds nothing else.
    await expect(target.latest?.()).resolves.toBeUndefined();
    await expect(target.fallback?.()).resolves.toBeUndefined();
  });

  it("proves its offers by the session the trade made, once it has one", async () => {
    let traded: HeldSession | undefined;
    const held = { core, grant: "a guest grant", senderLocator: "the-senders-locator" };
    const target = await targetOfGuestGrant(discoveryOf("k7xq2", []), held, () => traded);
    traded = { core, token: "the-guests-token", locator: "the-guests-locator" };
    const bySession = {
      held: { locator: "the-guests-locator" },
      key: await sessionOfferKey("the-guests-token"),
    };
    await expect(target.latest?.()).resolves.toEqual(bySession);
    await expect(target.fallback?.()).resolves.toEqual(bySession);
  });

  it("refuses a discovery answering for another core than the sender's", async () => {
    const held = { core, grant: "a guest grant", senderLocator: "the-senders-locator" };
    await expect(
      targetOfGuestGrant(discoveryOf("other", []), held, () => undefined),
    ).rejects.toThrow("another server");
  });
});
