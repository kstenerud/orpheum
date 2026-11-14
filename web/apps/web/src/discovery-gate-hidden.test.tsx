// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import type { Start } from "./discovery-start.ts";
import { buttonWithText, click, mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const linkStart: Start = {
  kind: "link",
  link: { coreId: "k7xq2", secret: "a-links-secret", fingerprint },
};

describe("a discovery's gate reaching its core over WebRTC (D482 §3)", () => {
  it("hands the transport what says the page needs its connection while hidden", async () => {
    const located = { coreId: "k7xq2", bootstrapToken: "a-bootstrap-token", iceServers: [] };
    const fetched: typeof fetch = (url) => {
      const address = url instanceof Request ? url.url : url.toString();
      return Promise.resolve(
        address.endsWith("/locate")
          ? new Response(JSON.stringify(located), { status: 200 })
          : new Response(null, { status: 204 }),
      );
    };
    const isNeededHidden = (): boolean => true;
    // The connection itself is the SDK's; here, only what the gate hands it.
    const handed: unknown[] = [];
    const connect = (...given: unknown[]): Promise<never> => {
      handed.push(given[2]);
      return Promise.reject(new Error("no connection in this test"));
    };
    const parts: GateParts = {
      client: { clientApi: "https://discovery.example/client/v1", fetch: fetched },
      sessionOf: () => undefined,
      hasRealms: false,
      realmStorage: undefined,
      onConnected: () => undefined,
      forgetCore: () => undefined,
      reachWays: { isNeededHidden, connect },
    };
    mounted = await mount(<DiscoveryGate start={linkStart} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    await settleUntil(() => handed.length > 0);
    expect(handed[0]).toEqual({ isNeededHidden });
  });
});
