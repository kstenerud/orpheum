// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pairingCodeOf } from "./lab-cast-target.tsx";
import { receiverDiscoveryOf } from "./cast-reach.ts";
import { castTargetRequest, openOurs, signInOf } from "./lab-cast.tsx";

const session = { token: () => "a-token", locator: () => "a-locator" };
const throughDiscovery = {
  clientApi: "https://api.example/client/v1",
  coreId: "k7xq2",
  fingerprint: "a-fingerprint",
  clientOrigin: "https://k7xq2.client.example",
};

describe("what a play sent to the lab's receiver signs in by", () => {
  it("is nothing unless the person ticked it and the page holds a session", () => {
    expect(signInOf(false, session, throughDiscovery)).toEqual({});
    expect(signInOf(true, { token: () => undefined, locator: () => undefined }, undefined)).toEqual(
      {},
    );
  });

  it("is the session alone on the core's own origin", () => {
    expect(signInOf(true, session, undefined)).toEqual({ authorization: "Bearer a-token" });
  });

  it("carries the way to the core from away under a discovery's loader (the 11-05 batch's item 10)", () => {
    expect(signInOf(true, session, throughDiscovery)).toEqual({
      authorization: "Bearer a-token",
      reach: { ...throughDiscovery, token: "a-token", locator: "a-locator" },
    });
  });
});

describe("what asks the lab's receiver to become a cast target", () => {
  it("carries the way to the core from away under a discovery's loader, which it pairs through", () => {
    expect(castTargetRequest(session, throughDiscovery)).toEqual({
      type: "device",
      reach: { ...throughDiscovery, token: "a-token", locator: "a-locator" },
    });
  });

  it("carries nothing on the core's own origin, where the receiver pairs with the core that served it", () => {
    expect(castTargetRequest(session, undefined)).toEqual({ type: "device" });
  });
});

describe("the lab's opening of our receiver", () => {
  it("asks for its report, which it makes only when asked, its log shown with it (the owner, 2026-11-12)", async () => {
    const sent: unknown[] = [];
    const opened = {
      send: async (message: unknown) => {
        sent.push(message);
        await Promise.resolve();
      },
    };
    const appIds: string[] = [];
    const open = async (appId: string): Promise<typeof opened> => {
      appIds.push(appId);
      await Promise.resolve();
      return opened;
    };
    await expect(openOurs(open, "ABCD1234")).resolves.toBe(opened);
    expect(appIds).toEqual(["ABCD1234"]);
    expect(sent).toEqual([{ type: "report" }]);
  });
});

describe("the lab receiver's pairing code", () => {
  it("is read from its pairing message alone", () => {
    expect(pairingCodeOf({ type: "pairing", code: "abcd-efgh" })).toBe("abcd-efgh");
    expect(pairingCodeOf({ type: "pairing", code: "" })).toBeUndefined();
    expect(pairingCodeOf({ type: "checkCode", checkCode: "42-17" })).toBeUndefined();
    expect(pairingCodeOf("pairing")).toBeUndefined();
  });
});

describe("the page's discovery as the lab's receiver is told it", () => {
  it("names this page's origin, the core's client origin, which the receiver goes direct from alone (the 11-23 batch's item 3(d))", () => {
    const found = {
      clientApi: throughDiscovery.clientApi,
      coreId: throughDiscovery.coreId,
      fingerprint: throughDiscovery.fingerprint,
    };
    expect(receiverDiscoveryOf(found, "https://k7xq2.client.example")).toEqual(throughDiscovery);
    expect(receiverDiscoveryOf(undefined, "https://k7xq2.client.example")).toBeUndefined();
  });
});
