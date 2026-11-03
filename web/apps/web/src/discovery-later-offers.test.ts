// SPDX-License-Identifier: MPL-2.0

import {
  sessionOfferKey,
  type DiscoveryClient,
  signallerOf,
  type HeldSession,
  type Target,
} from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { withLatest, type GateParts } from "./discovery-gate.tsx";
import { holdingAt } from "./loader-at-client-origin.tsx";
import { createSession, type Session } from "./session.ts";
import { keepMembership, membershipText } from "./together-storage.ts";

// The owner's Firefox, signed in as dan, opened the host's Watch Together link through staging's
// discovery: its offers were proven by the link's key, and after the join spent the link every offer
// was refused, the page held the same session as before, so nothing changed for latest; once the
// Wi-Fi went and came back, the page never connected again (2026-10-29).

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const core = { coreId: "k7xq2", fingerprint };
const target: Target = {
  coreId: "k7xq2",
  fingerprint,
  held: { bootstrapToken: "a-links-bootstrap-token" },
  key: new Uint8Array(32),
  iceServers: [],
  directUrls: [],
  lanUrls: [],
};

function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

/** Gate parts holding what a client origin's storage holds, read afresh each time as the loader does. */
function partsOver(storage: StringStorage): GateParts {
  const sessionNow = (): Session => createSession(storage, "k7xq2.");
  const client: DiscoveryClient = { clientApi: "https://discovery.example/client/v1" };
  return {
    client,
    hasRealms: false,
    realmStorage: undefined,
    sessionOf: (held): HeldSession | undefined => {
      const session = sessionNow();
      const token = session.token();
      const locator = session.locator();
      return token === undefined || locator === undefined
        ? undefined
        : { core: held, token, locator };
    },
    holdingOf: () => holdingAt(sessionNow(), () => membershipText(storage, "viewer")),
    forgetCore: () => undefined,
    onConnected: () => Promise.resolve(),
  };
}

describe("a link's later offers, its person signed in before they joined by it", () => {
  it("are proven by the session once the join spent the link, which only the membership tells", async () => {
    const storage = memoryStorage();
    createSession(storage, "k7xq2.").signIn("dans-token", { locator: "dans-locator" });
    const later = withLatest(partsOver(storage), core, target);
    // Before the join the link is the target's own: an earlier visit's session never takes its place.
    await expect(later.latest?.()).resolves.toBeUndefined();
    keepMembership(storage, "viewer", {
      sessionId: 1,
      credential: "dans-membership",
      locator: "the-memberships-locator",
    });
    const proving = { held: { locator: "dans-locator" }, key: await sessionOfferKey("dans-token") };
    await expect(later.latest?.()).resolves.toEqual(proving);
  });

  it("fall back on what the device holds now when the core refuses one", async () => {
    const storage = memoryStorage();
    const later = withLatest(partsOver(storage), core, target);
    await expect(later.fallback?.()).resolves.toBeUndefined();
    createSession(storage, "k7xq2.").signIn("dans-token", { locator: "dans-locator" });
    await expect(later.fallback?.()).resolves.toEqual({
      held: { locator: "dans-locator" },
      key: await sessionOfferKey("dans-token"),
    });
  });
});

describe("the signaller of a target's offers", () => {
  it("sends an offer the core refused as unproven once more under what the device holds now", async () => {
    const storage = memoryStorage();
    createSession(storage, "k7xq2.").signIn("dans-token", { locator: "dans-locator" });
    const sent: unknown[] = [];
    const client: DiscoveryClient = {
      clientApi: "https://discovery.example/client/v1",
      fetch: (_url, init) => {
        sent.push(JSON.parse(typeof init?.body === "string" ? init.body : "null"));
        // Refused every time: what is asked here is what the second offer was sent with.
        const refusal = "the core refused the offer: the offer's proof does not hold";
        return Promise.resolve(
          new Response(JSON.stringify({ answers: [{ offerId: "1", refusal }] })),
        );
      },
    };
    const signaller = signallerOf(client, withLatest(partsOver(storage), core, target));
    const offer = { type: "offer" as const, sdp: "v=0\r\na=fingerprint:sha-256 AB:CD\r\n" };
    await expect(signaller(offer)).rejects.toThrow("proof does not hold");
    expect(sent.map((each) => Object.keys(each as object)[0])).toEqual([
      "bootstrapToken",
      "locator",
    ]);
  });
});
