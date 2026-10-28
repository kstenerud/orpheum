// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { KeyRefused } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import type { HeldSession, KeptCore } from "./discovery-start.ts";
import { mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const core = { coreId: "k7xq2", fingerprint, realm: "karlnas" };
const held: HeldSession = { core, token: "a-session", locator: "a-revoked-locator" };
const located = {
  coreId: "k7xq2",
  iceServers: [],
  paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
};

/**
 * A discovery that finds the core by the locator, heard, and a core whose published name answers;
 * any other path, /dead-end included, is a 204.
 */
function discovery(heard: string[]): typeof fetch {
  return (url) => {
    const address = url instanceof Request ? url.url : url.toString();
    if (address.startsWith("https://v4.k7xq2.zone.example/")) {
      return Promise.resolve(new Response("{}", { status: 200 }));
    }
    const path = address.replace("https://discovery.example/client/v1", "");
    heard.push(path);
    return Promise.resolve(
      path === "/paths"
        ? new Response(JSON.stringify(located), { status: 200 })
        : new Response(null, { status: 204 }),
    );
  };
}

/**
 * The parts of a page whose remembered core refuses its key, as connectd passes the core's refusal
 * on, that hear for which core onKeyRefused was told.
 */
function refusingParts(heard: string[], refusedFor: KeptCore[]): GateParts {
  return {
    client: { clientApi: "https://discovery.example/client/v1", fetch: discovery(heard) },
    sessionOf: () => held,
    hasRealms: false,
    realmStorage: undefined,
    forgetCore: () => undefined,
    onConnected: () =>
      Promise.reject(
        new AggregateError([
          new KeyRefused("the core refused the offer: the offer's proof does not hold"),
        ]),
      ),
    onKeyRefused: (refused) => {
      refusedFor.push(refused);
    },
  };
}

describe("a remembered server that refused this device's key (the owner's Fire TV, 2026-10-28)", () => {
  it("forgets what the device held and says it needs to sign in again, not that its network failed", async () => {
    const heard: string[] = [];
    const refusedFor: KeptCore[] = [];
    mounted = await mount(
      <DiscoveryGate
        start={{ kind: "remembered", core }}
        parts={refusingParts(heard, refusedFor)}
      />,
    );
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("needs to sign in again"));
    expect(page.textContent).not.toContain("Can't reach the server from this network");
    expect(refusedFor).toEqual([core]);
    // No dead end: the core was reached, and the discovery's week of dead ends is not told one.
    expect(heard).toContain("/paths");
    expect(heard).not.toContain("/dead-end");
    // The next report names the refusal.
    expect(page.querySelector(".what-happened pre")?.textContent).toContain(
      "the offer's proof does not hold",
    );
  });

  it("at a discovery with realms, forgets the server too and offers the realm sign-in, saying why", async () => {
    let isForgotten = false;
    const refusedFor: KeptCore[] = [];
    const parts: GateParts = {
      ...refusingParts([], refusedFor),
      hasRealms: true,
      forgetCore: () => {
        isForgotten = true;
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "remembered", core }} parts={parts} />);
    const page = mounted.container;
    await settleUntil(() => page.querySelector('input[name="realm"]') !== null);
    expect(page.textContent).toContain("This device needs to sign in again");
    expect(page.textContent).not.toContain("can't be reached from this network");
    expect(isForgotten).toBe(true);
    expect(refusedFor).toEqual([core]);
  });

  it("opens the realm sign-in saying why when a client origin sent the page here for it", async () => {
    mounted = await mount(
      <DiscoveryGate
        start={{ kind: "none", keyRefusedBy: "k7xq2" }}
        parts={{ ...refusingParts([], []), sessionOf: () => undefined, hasRealms: true }}
      />,
    );
    const page = mounted.container;
    expect(page.querySelector('input[name="realm"]')).not.toBeNull();
    expect(page.textContent).toContain("This device needs to sign in again");
    const happened = page.querySelector(".what-happened pre")?.textContent ?? "";
    expect(happened).toContain("Server: k7xq2");
    expect(happened).toContain("the offer's proof does not hold");
  });
});
