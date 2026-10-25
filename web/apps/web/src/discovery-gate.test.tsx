// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import {
  fromBase64,
  locateHash,
  TurnedAway,
  realmAsk,
  realmKey,
  realmPerson,
  realmTag,
  sessionOfferKey,
} from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, withLatest, type GateParts } from "./discovery-gate.tsx";
import {
  discoveryClientApiOf,
  discoveryHasRealms,
  handOffFragment,
  heldWith,
  readHandOffFragment,
  whenSignedOut,
  type HeldSession,
  type KeptCore,
  type Start,
  type Target,
} from "./discovery-start.ts";
import { startAtClient, type AtClientOrigin } from "./loader-at-client-origin.tsx";
import type { StringStorage } from "./browser-storage.ts";
import { buttonWithText, click, enter, mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const linkHash = `#k7xq2.a-links-secret.${fingerprint}`;

/** The start a page opened on linkHash makes. */
const linkStart: Start = {
  kind: "link",
  link: { coreId: "k7xq2", secret: "a-links-secret", fingerprint },
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
    // The loader at the core's client origin, which keeps the core it reached (D308).
    const at = (hash: string): AtClientOrigin => ({
      config: {
        clientApi: "https://discovery.example/client/v1",
        hasRealms: false,
        clientOrigins: "https://{core}.client.example",
        loaderOrigin: "https://app.example",
      },
      coreId: "k7xq2",
      storage,
      location: { hash, pathname: "/", search: "", assign: () => undefined },
      history: { replaceState: () => undefined },
      loaderOrigin: "https://app.example",
      unmount: () => undefined,
      tellWorker: () => Promise.resolve(),
    });
    expect(startAtClient(at(linkHash))).toEqual(linkStart);
    expect(startAtClient(at(""))).toEqual({ kind: "none" });
    storage.setItem("orpheum.loader.core", JSON.stringify({ fingerprint, realm: "" }));
    expect(startAtClient(at(""))).toEqual({
      kind: "remembered",
      core: { coreId: "k7xq2", fingerprint },
    });
  });
});

describe("the gate a discovery's page opens on", () => {
  it("spends a link only on its click, never on the page's loading (the protocol's §3.1)", async () => {
    const heard: string[] = [];
    const start = linkStart;
    mounted = await mount(<DiscoveryGate start={start} parts={partsOf(discovery({}, heard))} />);
    expect(mounted.container.textContent).toContain("Your server's link");
    expect(heard).toEqual([]);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("This link no longer finds its server"));
    expect(heard).toEqual([`/locate {"hash":"${await locateHash("a-links-secret")}"}`]);
    expect(mounted.container.textContent).toContain("This link no longer finds its server");
  });

  it("sends a remembered server found no more back to the loader's list, but says so of a link", async () => {
    let gone = 0;
    const parts: GateParts = {
      ...partsOf(discovery({}, [])),
      onGone: () => {
        gone += 1;
      },
    };
    const remembered = { kind: "remembered", core: { coreId: "k7xq2", fingerprint } } as const;
    mounted = await mount(<DiscoveryGate start={remembered} parts={parts} />);
    await settleUntil(() => gone > 0);
    expect(gone).toBe(1);
    mounted.unmount();
    mounted = await mount(<DiscoveryGate start={linkStart} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("This link no longer finds its server"));
    expect(gone).toBe(1);
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
    mounted = await mount(<DiscoveryGate start={linkStart} parts={partsOf(fetched)} />);
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
        reached.push(how.first.kind === "direct" ? how.first.origin : how.first.kind);
      },
    };
    mounted = await mount(<DiscoveryGate start={linkStart} parts={parts} />);
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
    mounted = await mount(<DiscoveryGate start={linkStart} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    await settleUntil(() => connected.length > 0);
    expect(heard).toEqual([
      `/locate {"hash":"${await locateHash("a-links-secret")}"}`,
      `/paths {"locator":"a-locator"}`,
    ]);
    expect(connected).toEqual([core]);
  });

  it("reaches the core a realm sign-in handed on by its bootstrap token, at once (D308 §4)", async () => {
    const heard: string[] = [];
    const core = { coreId: "k7xq2", fingerprint };
    const handOff = {
      code: "a-code",
      bootstrapToken: "a-bootstrap-token",
      fingerprint,
      realm: "nas",
    };
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
    mounted = await mount(
      <DiscoveryGate
        start={{ kind: "handoff", core, handOff }}
        parts={partsOf(fetched, connected)}
      />,
    );
    await settleUntil(() => connected.length > 0);
    expect(heard).toEqual([`/paths {"bootstrapToken":"a-bootstrap-token"}`]);
    expect(connected).toEqual([core]);
  });

  it("with no link and no server reached before, says how to get in", async () => {
    mounted = await mount(
      <DiscoveryGate start={{ kind: "none" }} parts={partsOf(discovery({}, []))} />,
    );
    expect(mounted.container.textContent).toContain("Open the link you were sent");
  });
});

describe("a hand-off's fragment (D308 §4)", () => {
  it("carries the code, the bootstrap token, the fingerprint and the realm, and reads back only whole", () => {
    const handOff = { code: "a+code/1", bootstrapToken: "a token", fingerprint, realm: "nas.kids" };
    expect(readHandOffFragment(handOffFragment(handOff))).toEqual(handOff);
    for (const text of [
      "",
      linkHash,
      "#handoff=a-code",
      "#handoff=a-code&bootstrap=a-token",
      `#bootstrap=a-token&fingerprint=${fingerprint}`,
    ]) {
      expect(readHandOffFragment(text)).toBeUndefined();
    }
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
    lanUrls: [],
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

describe("a person with no link at a discovery that has realms (D297, D298)", () => {
  // The Ed25519 key of the seed 0x00…0x1f, and its fingerprint, as discovery.test.ts has them.
  const publicKey = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg";
  const fingerprintOfKey = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";

  /** A discovery taking karl's sign-in at karlnas, the core at its own name, which answers. */
  async function realmDiscovery(
    heard: string[],
    paths: readonly { readonly kind: string; readonly url: string }[] = [
      { kind: "ipv4", url: "https://v4.k7xq2.zone.example" },
    ],
  ): Promise<typeof fetch> {
    const key = await realmKey("karlnas", "karl", "a password");
    const person = await realmPerson("karlnas", "karl");
    const karls = JSON.stringify({
      realm: "karlnas",
      person,
      ask: await realmAsk(key, "karlnas", person),
    });
    const tag = await realmTag(key, "k7xq2", fromBase64(publicKey));
    return (url, init) => {
      const address = url instanceof Request ? url.url : url.toString();
      if (address.startsWith("https://v4.k7xq2.zone.example/")) {
        return Promise.resolve(new Response("{}", { status: 200 }));
      }
      const body = typeof init?.body === "string" ? init.body : "";
      heard.push(`${address.replace("https://discovery.example/client/v1", "")} ${body}`);
      const isKarl = body === karls;
      return Promise.resolve(
        isKarl
          ? new Response(
              JSON.stringify({
                coreId: "k7xq2",
                bootstrapToken: "a-bootstrap-token",
                iceServers: [],
                paths,
                publicKey,
                tag,
                claimedAt: 1_790_000_000_000,
              }),
              { status: 200 },
            )
          : new Response("{}", { status: 404 }),
      );
    };
  }

  it("is asked for realm, name and password together, where the page says its discovery has realms", async () => {
    const page = document.implementation.createHTMLDocument("");
    expect(discoveryHasRealms(page)).toBe(false);
    const meta = page.createElement("meta");
    meta.setAttribute("name", "orpheum-discovery-realms");
    meta.setAttribute("content", "true");
    page.head.append(meta);
    expect(discoveryHasRealms(page)).toBe(true);
    mounted = await mount(
      <DiscoveryGate
        start={{ kind: "none" }}
        parts={{ ...partsOf(discovery({}, [])), hasRealms: true }}
      />,
    );
    expect(mounted.container.querySelector('input[name="realm"]')).not.toBeNull();
    expect(mounted.container.textContent).not.toContain("Which server?");
  });

  it("reaches the core its realm sign-in found, hands on the name and password, and keeps the realm", async () => {
    const heard: string[] = [];
    const storage = memoryStorage();
    const connected: { core: KeptCore; signIn: unknown }[] = [];
    const parts: GateParts = {
      ...partsOf(await realmDiscovery(heard)),
      hasRealms: true,
      realmStorage: storage,
      onConnected: (core, _reached, signIn) => {
        connected.push({ core, signIn });
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), " KarlNAS ");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(page.querySelector('input[type="checkbox"]'));
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => connected.length > 0);
    expect(heard[0]?.startsWith("/realm ")).toBe(true);
    expect(connected).toEqual([
      {
        core: { coreId: "k7xq2", fingerprint: fingerprintOfKey, realm: "karlnas" },
        signIn: {
          name: "karl",
          password: "a password",
          held: { bootstrapToken: "a-bootstrap-token" },
        },
      },
    ]);
    expect(storage.getItem("orpheum.discovery.realm")).toBe("karlnas");
  });

  it("tries no LAN name where the page only hands the browser on, so the local network is asked for once", async () => {
    const lan = "https://192-168-1-2.k7xq2.zone.example:8443";
    const heard: string[] = [];
    const reached: unknown[] = [];
    const parts: GateParts = {
      ...partsOf(
        await realmDiscovery(heard, [
          { kind: "lan", url: lan },
          { kind: "ipv4", url: "https://v4.k7xq2.zone.example" },
        ]),
      ),
      hasRealms: true,
      shouldSkipLanNames: true,
      onConnected: (_core, reaching) => {
        reached.push(reaching.first);
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    await enter(page.querySelector('input[name="realm"]'), "karlnas");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a password");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => reached.length > 0);
    expect(reached).toEqual([{ kind: "direct", origin: "https://v4.k7xq2.zone.example" }]);
    expect(heard.filter((request) => request.startsWith(lan))).toEqual([]);
  });

  it("stops with a warning when the realm leads to another server than the one remembered (D312 §2)", async () => {
    const connected: KeptCore[] = [];
    const remembered = { coreId: "old7x", fingerprint: "an-older-fingerprint", realm: "karlnas" };
    const asked: KeptCore[] = [];
    const parts: GateParts = {
      ...partsOf(await realmDiscovery([])),
      hasRealms: true,
      changedFrom: (found) => {
        asked.push(found);
        return found.fingerprint === remembered.fingerprint ? undefined : remembered;
      },
      onConnected: (core) => {
        connected.push(core);
      },
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    const signIn = async (): Promise<void> => {
      await enter(page.querySelector('input[name="realm"]'), "karlnas");
      await enter(page.querySelector('input[name="name"]'), "karl");
      await enter(page.querySelector('input[type="password"]'), "a password");
      await click(buttonWithText(page, "Sign in"));
      await settleUntil(() => page.textContent.includes("Your household's server has changed"));
    };
    await signIn();
    expect(asked).toEqual([{ coreId: "k7xq2", fingerprint: fingerprintOfKey, realm: "karlnas" }]);
    expect(page.textContent).toContain("The new one was connected on");
    expect(connected).toEqual([]);
    await click(buttonWithText(page, "Stop"));
    expect(page.querySelector('input[name="realm"]')).not.toBeNull();
    expect(connected).toEqual([]);
    await signIn();
    await click(buttonWithText(page, "Go on to the new server"));
    await settleUntil(() => connected.length > 0);
    expect(connected).toEqual([
      { coreId: "k7xq2", fingerprint: fingerprintOfKey, realm: "karlnas" },
    ]);
  });

  it("says the realm, name or password is wrong as one, and keeps no realm when not asked to", async () => {
    const storage = memoryStorage();
    storage.setItem("orpheum.discovery.realm", "karlnas");
    const parts: GateParts = {
      ...partsOf(await realmDiscovery([])),
      hasRealms: true,
      realmStorage: storage,
    };
    mounted = await mount(<DiscoveryGate start={{ kind: "none" }} parts={parts} />);
    const page = mounted.container;
    expect(page.querySelector<HTMLInputElement>('input[name="realm"]')?.value).toBe("karlnas");
    await click(page.querySelector('input[type="checkbox"]'));
    await enter(page.querySelector('input[name="realm"]'), "the-smiths");
    await enter(page.querySelector('input[name="name"]'), "karl");
    await enter(page.querySelector('input[type="password"]'), "a guess");
    await click(buttonWithText(page, "Sign in"));
    await settleUntil(() => page.textContent.includes("The realm, name or password is wrong"));
    expect(storage.getItem("orpheum.discovery.realm")).toBeNull();
  });
});

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
    const happened = page.querySelector(".what-happened pre")?.textContent ?? "";
    expect(happened).toContain("Server: k7xq2");
    expect(happened).toContain("Addresses tried: https://v4.k7xq2.zone.example");
    expect(happened).toContain("What failed:");
    expect(page.textContent).toContain("A VPN or a travel eSIM");
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
