// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import {
  fromBase64,
  CoreOffline,
  TurnedAway,
  realmKey,
  realmTag,
  type HeldSession,
  type KeptCore,
} from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DiscoveryGate, offlineRetryAfterMs, type GateParts } from "./discovery-gate.tsx";
import { whenSignedOut } from "./discovery-start.ts";
import { buttonWithText, click, enter, mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";

/** A discovery answering each path as asked, hearing every call. */
function discovery(answers: Record<string, [number, unknown]>, heard: string[]): typeof fetch {
  return (url, init) => {
    const address = url instanceof Request ? url.url : url.toString();
    const path = address.replace("https://discovery.example/client/v1", "");
    heard.push(`${path} ${typeof init?.body === "string" ? init.body : ""}`);
    const [status, body] = answers[path] ?? [404, {}];
    return Promise.resolve(new Response(status === 204 ? null : JSON.stringify(body), { status }));
  };
}

function partsOf(fetched: typeof fetch, connected: KeptCore[] = []): GateParts {
  return {
    client: { clientApi: "https://discovery.example/client/v1", fetch: fetched },
    sessionOf: () => undefined,
    hasRealms: false,
    realmStorage: undefined,
    onConnected: (core) => {
      connected.push(core);
    },
    forgetCore: () => undefined,
  };
}

describe("a page at a discovery with realms leaves nothing kept in the way (the owner, 2026-10-15)", () => {
  it("opens the realm sign-in, saying so, when the server it reached before is not reached", async () => {
    const core = { coreId: "k7xq2", fingerprint };
    let isForgotten = false;
    const held: HeldSession = { core, token: "an-old-session", locator: "an-old-locator" };
    const parts: GateParts = {
      ...partsOf(discovery({ "/paths": [404, {}] }, [])),
      hasRealms: true,
      sessionOf: () => held,
      forgetCore: () => {
        isForgotten = true;
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "remembered", core }} parts={parts} />);
    const page = mounted.container;
    await settleUntil(() => page.querySelector('input[name="realm"]') !== null);
    expect(page.textContent).toContain("Sign in again");
    expect(page.textContent).not.toContain("Can't reach the server from this network");
    expect(isForgotten).toBe(true);
  });

  it("keeps the realm sign-in when the server was found but its own sign-in failed", async () => {
    const key = await realmKey("karlnas", "karl", "a password");
    const publicKey = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg";
    const found = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
      publicKey,
      tag: await realmTag(key, "k7xq2", fromBase64(publicKey)),
    };
    const toDiscovery = discovery({ "/realm": [200, found] }, []);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    const parts: GateParts = {
      ...partsOf(fetched),
      hasRealms: true,
      onConnected: () => Promise.reject(new Error("the core's own sign-in failed")),
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), "karlnas");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => page.textContent.includes("can't be reached from this network"));
    expect(page.querySelector('input[name="realm"]')).not.toBeNull();
  });

  // Said as a network that could not reach the server, the owner's Fire TV looked broken when the
  // server had turned it away for the pages already open on its network (staging, 2026-10-25).
  it("says the server turned this device away for its network's share, not that its network failed", async () => {
    const key = await realmKey("karlnas", "karl", "a password");
    const publicKey = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg";
    const found = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
      publicKey,
      tag: await realmTag(key, "k7xq2", fromBase64(publicKey)),
    };
    const heard: string[] = [];
    const toDiscovery = discovery({ "/realm": [200, found] }, heard);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    const parts: GateParts = {
      ...partsOf(fetched),
      hasRealms: true,
      onConnected: () =>
        Promise.reject(new Error("the hand-off failed", { cause: new TurnedAway("network-full") })),
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), "karlnas");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => page.textContent.includes("too many Orpheum pages are open"));
    expect(page.textContent).toContain(
      "This server is turning this device away: too many Orpheum pages are open on your network. Close some and try again.",
    );
    expect(page.textContent).not.toContain("can't be reached from this network");
    expect(page.querySelector('input[name="realm"]')).not.toBeNull();
    expect(heard.some((each) => each.startsWith("/dead-end"))).toBe(false);
  });

  it("keeps a remembered server that turned this device away, and tries it again when asked", async () => {
    const core = { coreId: "k7xq2", fingerprint };
    let isForgotten = false;
    const held: HeldSession = { core, token: "a-session", locator: "a-locator" };
    const located = {
      coreId: "k7xq2",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
    };
    const toDiscovery = discovery({ "/paths": [200, located] }, []);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    let tries = 0;
    const parts: GateParts = {
      ...partsOf(fetched),
      hasRealms: true,
      sessionOf: () => held,
      forgetCore: () => {
        isForgotten = true;
      },
      onConnected: () => {
        tries += 1;
        return tries === 1 ? Promise.reject(new TurnedAway("network-full")) : Promise.resolve();
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "remembered", core }} parts={parts} />);
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("too many Orpheum pages are open"));
    expect(page.textContent).not.toContain("Sign in again");
    expect(isForgotten).toBe(false);
    await click(buttonWithText(page, "Try again"));
    await settleUntil(() => tries === 2);
    expect(tries).toBe(2);
  });

  it("says a remembered server its discovery cannot reach is not connected, keeps it, and tries again by itself", async () => {
    const core = { coreId: "k7xq2", fingerprint };
    let isForgotten = false;
    const held: HeldSession = { core, token: "a-session", locator: "a-locator" };
    const located = {
      coreId: "k7xq2",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
    };
    const heard: string[] = [];
    const toDiscovery = discovery({ "/paths": [200, located] }, heard);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    let tries = 0;
    const parts: GateParts = {
      ...partsOf(fetched),
      hasRealms: true,
      sessionOf: () => held,
      forgetCore: () => {
        isForgotten = true;
      },
      onConnected: () =>
        (tries += 1) === 1
          ? Promise.reject(new AggregateError([new CoreOffline()]))
          : Promise.resolve(),
    };
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      mounted = await mount(<DiscoveryGate start={{ kind: "remembered", core }} parts={parts} />);
      const page = mounted.container;
      await settleUntil(() => page.textContent.includes("isn't connected right now"));
      expect(page.textContent).toContain(
        "It may be restarting, switched off, or without internet.",
      );
      expect(page.textContent).not.toContain("can't be reached from this network");
      expect(isForgotten).toBe(false);
      expect(heard.some((each) => each.startsWith("/dead-end"))).toBe(false);
      await vi.advanceTimersByTimeAsync(offlineRetryAfterMs);
      await settleUntil(() => tries === 2);
      expect(tries).toBe(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("offers what happened to copy when a realm's server was found but nothing reached it (K16)", async () => {
    const key = await realmKey("karlnas", "karl", "a password");
    const publicKey = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg";
    const found = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
      publicKey,
      tag: await realmTag(key, "k7xq2", fromBase64(publicKey)),
    };
    const toDiscovery = discovery({ "/realm": [200, found], "/dead-end": [204, undefined] }, []);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : toDiscovery(url, init);
    mounted = await mount(
      <DiscoveryGate start={{ kind: "none" }} parts={{ ...partsOf(fetched), hasRealms: true }} />,
    );
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), "karlnas");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => page.textContent.includes("Copy what happened"));
    // On the page, below its form, not after a page a screen tall (the owner, 2026-11-07).
    const onPage = page.querySelector("main.credentials-form");
    expect(onPage?.contains(buttonWithText(page, "Copy what happened") ?? null)).toBe(true);
    const happened = page.querySelector(".what-happened pre")?.textContent ?? "";
    expect(happened).toContain("Server: k7xq2");
    expect(happened).toContain("Addresses tried: https://v4.k7xq2.zone.example");
    expect(happened).toContain("What failed:");
    expect(page.textContent).toContain("A VPN or a travel eSIM");
  });

  it("says a realm's server is out of reach for this network's want of IPv6, when it is (D398)", async () => {
    const key = await realmKey("karlnas", "karl", "a password");
    const publicKey = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg";
    const found = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      clientPrefix: "203.0.113.0/24",
      paths: [{ kind: "ipv6", url: "https://v6.k7xq2.zone.example" }],
      publicKey,
      tag: await realmTag(key, "k7xq2", fromBase64(publicKey)),
    };
    const heard: string[] = [];
    const toDiscovery = discovery({ "/realm": [200, found], "/dead-end": [204, undefined] }, heard);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v6.k7xq2.zone.example/",
      )
        ? Promise.reject(new TypeError("Failed to fetch"))
        : toDiscovery(url, init);
    mounted = await mount(
      <DiscoveryGate start={{ kind: "none" }} parts={{ ...partsOf(fetched), hasRealms: true }} />,
    );
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), "karlnas");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => page.textContent.includes("Copy what happened"));
    expect(page.textContent).toContain(
      "Your server was found, but this network seems to have no IPv6, and your server has no IPv4 way in yet",
    );
    expect(heard.find((each) => each.startsWith("/dead-end"))).toContain(`"cause":"noIpv6"`);
  });

  it("starts again once signed out, and never for a page that was never signed in", () => {
    let token: string | undefined = "a-session";
    const listeners = new Set<() => void>();
    const session = {
      token: () => token,
      subscribe: (listener: () => void) => {
        listeners.add(listener);
        return () => {
          listeners.delete(listener);
        };
      },
    };
    const set = (next: string | undefined): void => {
      token = next;
      for (const listener of listeners) {
        listener();
      }
    };
    let startedAgain = 0;
    whenSignedOut(session, () => {
      startedAgain += 1;
    });
    set("a-newer-session");
    expect(startedAgain).toBe(0);
    set(undefined);
    set("signed-in-again");
    set(undefined);
    expect(startedAgain).toBe(1);
    token = undefined;
    let guestStartedAgain = 0;
    whenSignedOut(session, () => {
      guestStartedAgain += 1;
    });
    set(undefined);
    expect(guestStartedAgain).toBe(0);
  });
});
