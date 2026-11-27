// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  connectThrough,
  targetOfGuestGrant,
  type HeldSession,
  type Target,
} from "./core-target.ts";
import { guestGrantOfferKey, sessionOfferKey, type DiscoveryClient } from "./discovery.ts";
import type { RtcConnectOptions, Signaller } from "./rtc-connect.ts";

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
    const traded: HeldSession[] = [];
    const held = { core, grant: "a guest grant", senderLocator: "the-senders-locator" };
    const target = await targetOfGuestGrant(discoveryOf("k7xq2", []), held, () => traded[0]);
    traded.push({ core, token: "the-guests-token", locator: "the-guests-locator" });
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

describe("a connection through the discovery (D482)", () => {
  it("hands its transport what says the page needs its connection while hidden", async () => {
    const isNeededHidden = (): boolean => false;
    const target: Target = {
      coreId: "k7xq2",
      fingerprint: core.fingerprint,
      held: { bootstrapToken: "a-bootstrap-token" },
      key: new Uint8Array(32),
      iceServers: [{ urls: ["stun:stun.example:3478"] }],
      directUrls: [],
      lanUrls: [],
    };
    const handed: unknown[][] = [];
    const connect = (...given: unknown[]): Promise<never> => {
      handed.push(given.slice(1));
      return Promise.reject(new Error("no connection in this test"));
    };
    await expect(
      connectThrough(discoveryOf("k7xq2", []), target, { isNeededHidden, connect }),
    ).rejects.toThrow("no connection in this test");
    expect(handed).toEqual([
      [
        { iceServers: target.iceServers },
        { isNeededHidden, iceServersAgain: expect.any(Function) as unknown },
      ],
    ]);
  });
});

describe("a connection through the discovery fetches its relay credentials again (D564 §3)", () => {
  const relayed = { urls: ["turn:relay.example:3478"], username: "1:client", credential: "c" };

  /** The iceServersAgain connectThrough hands its transport for target, on a discovery for coreId. */
  async function againOf(
    target: Target,
    coreId: string,
    sent: unknown[],
  ): Promise<() => Promise<RTCIceServer[]>> {
    const discovery: DiscoveryClient = {
      clientApi: "https://discovery.example/client/v1",
      fetch: (url, init) => {
        const asked = typeof url === "string" ? url : "a request that is not a URL's text";
        sent.push([asked, JSON.parse(typeof init?.body === "string" ? init.body : "null")]);
        return Promise.resolve(new Response(JSON.stringify({ coreId, iceServers: [relayed] })));
      },
    };
    let again: (() => Promise<RTCIceServer[]>) | undefined;
    const connect = (
      _signaller: Signaller,
      _config?: RTCConfiguration,
      options?: RtcConnectOptions,
    ): Promise<never> => {
      again = options?.iceServersAgain;
      return Promise.reject(new Error("no connection in this test"));
    };
    await expect(connectThrough(discovery, target, { connect })).rejects.toThrow("no connection");
    if (again === undefined) {
      throw new Error("connectThrough handed no way to fetch the ICE servers again");
    }
    return again;
  }

  const target: Target = {
    coreId: "k7xq2",
    fingerprint: core.fingerprint,
    held: { bootstrapToken: "a-bootstrap-token" },
    key: new Uint8Array(32),
    iceServers: [],
    directUrls: [],
    lanUrls: [],
  };

  it("asks the discovery's paths by what the device holds by then, and takes its ICE servers", async () => {
    const sent: unknown[] = [];
    const later: Target = {
      ...target,
      latest: () =>
        Promise.resolve({ held: { locator: "the-later-locator" }, key: new Uint8Array(32) }),
    };
    const again = await againOf(later, "k7xq2", sent);
    await expect(again()).resolves.toEqual([relayed]);
    expect(sent).toEqual([
      ["https://discovery.example/client/v1/paths", { locator: "the-later-locator" }],
    ]);
  });

  it("asks by what the target began with where the device holds nothing more", async () => {
    const sent: unknown[] = [];
    const again = await againOf(target, "k7xq2", sent);
    await again();
    expect(sent).toEqual([
      ["https://discovery.example/client/v1/paths", { bootstrapToken: "a-bootstrap-token" }],
    ]);
  });

  it("refuses a discovery answering for another core", async () => {
    const again = await againOf(target, "other", []);
    await expect(again()).rejects.toThrow("another server");
  });
});
