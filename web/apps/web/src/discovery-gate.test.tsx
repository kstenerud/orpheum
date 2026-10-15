// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { locateHash, sessionOfferKey } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, withLatest, type GateParts } from "./discovery-gate.tsx";
import {
  discoveryClientApiOf,
  heldWith,
  rememberCore,
  startOf,
  type HeldSession,
  type KeptCore,
  type Target,
} from "./discovery-start.ts";
import type { StringStorage } from "./browser-storage.ts";
import { buttonWithText, click, mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const linkHash = `#k7xq2.a-links-secret.${fingerprint}`;

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
    onConnected: (core) => {
      connected.push(core);
    },
  };
}

describe("where a page served by a discovery starts", () => {
  it("names its discovery in the page it serves, and the core's own page names none (D204)", () => {
    const page = document.implementation.createHTMLDocument("");
    expect(discoveryClientApiOf(page)).toBe("");
    const meta = page.createElement("meta");
    meta.setAttribute("name", "orpheum-discovery-client-api");
    meta.setAttribute("content", "https://api-staging.orpheum-app.com/client/v1");
    page.head.append(meta);
    expect(discoveryClientApiOf(page)).toBe("https://api-staging.orpheum-app.com/client/v1");
  });

  it("starts from a link, else the core it reached before, else nothing", () => {
    const storage = memoryStorage();
    expect(startOf(linkHash, storage)).toEqual({
      kind: "link",
      link: { coreId: "k7xq2", secret: "a-links-secret", fingerprint },
    });
    expect(startOf("", storage)).toEqual({ kind: "none" });
    rememberCore(storage, { coreId: "k7xq2", fingerprint });
    expect(startOf("", storage)).toEqual({
      kind: "remembered",
      core: { coreId: "k7xq2", fingerprint },
    });
  });
});

describe("the gate a discovery's page opens on", () => {
  it("spends a link only on its click, never on the page's loading (the protocol's §3.1)", async () => {
    const heard: string[] = [];
    const start = startOf(linkHash, memoryStorage());
    mounted = await mount(<DiscoveryGate start={start} parts={partsOf(discovery({}, heard))} />);
    expect(mounted.container.textContent).toContain("Your server's link");
    expect(heard).toEqual([]);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("This link no longer finds its server"));
    expect(heard).toEqual([`/locate {"hash":"${await locateHash("a-links-secret")}"}`]);
    expect(mounted.container.textContent).toContain("This link no longer finds its server");
  });

  it("says when nothing reaches the server, and tells the discovery so", async () => {
    const heard: string[] = [];
    const located = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [{ urls: ["stun:stun.example:3478"] }],
    };
    // No RTCPeerConnection here: the connection fails as one through no path would.
    const fetched = discovery({ "/locate": [200, located], "/dead-end": [204, {}] }, heard);
    mounted = await mount(
      <DiscoveryGate start={startOf(linkHash, memoryStorage())} parts={partsOf(fetched)} />,
    );
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => heard.at(-1)?.startsWith("/dead-end") === true);
    expect(page.textContent).toContain("Can't reach the server from this network");
    expect(heard.at(-1)).toBe(`/dead-end {"bootstrapToken":"a-bootstrap-token"}`);
  });

  it("reaches the core at its own published name before WebRTC (W2c)", async () => {
    const heard: string[] = [];
    const located = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      paths: [
        { kind: "ipv4", url: "https://v4.k7xq2.zone.example" },
        { kind: "webrtc", offer: "/cores/k7xq2/offer" },
      ],
    };
    const toDiscovery = discovery({ "/locate": [200, located] }, heard);
    // The discovery's fetch also asks the core's name for its health: it answers.
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    const connected: KeptCore[] = [];
    const reached: string[] = [];
    const parts: GateParts = {
      ...partsOf(fetched, connected),
      onConnected: (core, how) => {
        connected.push(core);
        reached.push(how.kind === "direct" ? how.origin : how.kind);
      },
    };
    mounted = await mount(
      <DiscoveryGate start={startOf(linkHash, memoryStorage())} parts={parts} />,
    );
    await click(buttonWithText(mounted.container, "Connect"));
    await settleUntil(() => reached.length > 0);
    expect(reached).toEqual(["https://v4.k7xq2.zone.example"]);
    expect(connected).toEqual([{ coreId: "k7xq2", fingerprint }]);
  });

  it("reaches the server by what this browser holds when its link is spent", async () => {
    const heard: string[] = [];
    const core = { coreId: "k7xq2", fingerprint };
    const held: HeldSession = { core, token: "a-membership-credential", locator: "a-locator" };
    const located = {
      coreId: "k7xq2",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
    };
    const toDiscovery = discovery({ "/paths": [200, located] }, heard);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    const connected: KeptCore[] = [];
    const parts: GateParts = { ...partsOf(fetched, connected), sessionOf: () => held };
    mounted = await mount(
      <DiscoveryGate start={startOf(linkHash, memoryStorage())} parts={parts} />,
    );
    await click(buttonWithText(mounted.container, "Connect"));
    await settleUntil(() => connected.length > 0);
    expect(heard).toEqual([
      `/locate {"hash":"${await locateHash("a-links-secret")}"}`,
      `/paths {"locator":"a-locator"}`,
    ]);
    expect(connected).toEqual([core]);
  });

  it("with no link and no server reached before, says how to get in", async () => {
    mounted = await mount(
      <DiscoveryGate start={{ kind: "none" }} parts={partsOf(discovery({}, []))} />,
    );
    expect(mounted.container.textContent).toContain("Open the link you were sent");
  });
});

describe("what a connection's later offers are proven with", () => {
  const core = { coreId: "k7xq2", fingerprint };
  const target: Target = {
    coreId: "k7xq2",
    fingerprint,
    held: { bootstrapToken: "a-bootstrap-token" },
    key: new Uint8Array(32),
    iceServers: [],
    directUrls: [],
  };

  it("is what its person gained by the link once they joined: their membership", async () => {
    const holding: { held?: HeldSession } = {};
    const parts: GateParts = { ...partsOf(discovery({}, [])), sessionOf: () => holding.held };
    const latest = withLatest(parts, core, target).latest;
    await expect(latest?.()).resolves.toBeUndefined();
    holding.held = { core, token: "a-membership-credential", locator: "a-locator" };
    await expect(latest?.()).resolves.toEqual({
      held: { locator: "a-locator" },
      key: await sessionOfferKey("a-membership-credential"),
    });
  });

  it("is never a session from an earlier visit in place of the link being spent", async () => {
    const earlier: HeldSession = { core, token: "an-old-token", locator: "an-old-locator" };
    const parts: GateParts = { ...partsOf(discovery({}, [])), sessionOf: () => earlier };
    await expect(withLatest(parts, core, target).latest?.()).resolves.toBeUndefined();
  });
});

describe("what this browser holds with a core", () => {
  const core = { coreId: "k7xq2", fingerprint };
  const session = (token?: string, locator?: string) => ({
    token: () => token,
    locator: () => locator,
  });
  const membership = { credential: "a-membership-credential", locator: "a-membership-locator" };

  it("is its session when signed in, else its membership as a guest, else nothing", () => {
    expect(heldWith(core, session("a-token", "a-locator"), membership)).toEqual({
      core,
      token: "a-token",
      locator: "a-locator",
    });
    expect(heldWith(core, session(), membership)).toEqual({
      core,
      token: "a-membership-credential",
      locator: "a-membership-locator",
    });
    expect(heldWith(core, session(), { credential: "kept-before-locators" })).toBeUndefined();
    expect(heldWith(core, session(), undefined)).toBeUndefined();
  });
});
