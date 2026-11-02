// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import {
  fromBase64,
  locateHash,
  NoServiceWorker,
  realmAsk,
  realmKey,
  realmPerson,
  realmTag,
  sessionOfferKey,
  stepsSoFar,
} from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, withLatest, type GateParts } from "./discovery-gate.tsx";
import {
  discoveryClientApiOf,
  discoveryHasRealms,
  handOffFragment,
  heldWith,
  readHandOffFragment,
  type HeldSession,
  type KeptCore,
  type Start,
  type Target,
} from "./discovery-start.ts";
import { startAtClient } from "./loader-at-client-origin.tsx";
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
    const at = (hash: string): Parameters<typeof startAtClient>[0] => ({
      coreId: "k7xq2",
      storage,
      location: { hash, pathname: "/", search: "", assign: () => undefined },
      history: { replaceState: () => undefined },
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
    expect(heard.at(-1)).toBe(
      `/dead-end {"bootstrapToken":"a-bootstrap-token","ipv6":"unknown","tried":[{"kind":"webrtc","ended":"failed"}],"cause":"unknown"}`,
    );
  });

  /**
   * A discovery whose core has the paths named and sees the page from clientPrefix, the core's names
   * failing at once as on a network with no route for their family; every call heard.
   */
  function unreachedCore(
    paths: readonly { readonly kind: string; readonly url: string }[],
    clientPrefix: string,
    heard: string[],
  ): typeof fetch {
    const located = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      clientPrefix,
      paths,
    };
    const toDiscovery = discovery({ "/locate": [200, located], "/dead-end": [204, {}] }, heard);
    return (url, init) =>
      (url instanceof Request ? url.url : url.toString()).includes(".k7xq2.zone.example")
        ? Promise.reject(new TypeError("Failed to fetch"))
        : toDiscovery(url, init);
  }

  it("says this network has no IPv6 when the server has no IPv4 way in, and tells the discovery why (D398)", async () => {
    const heard: string[] = [];
    const v6Only = [{ kind: "ipv6", url: "https://v6.k7xq2.zone.example" }];
    const fetched = unreachedCore(v6Only, "203.0.113.0/24", heard);
    mounted = await mount(<DiscoveryGate start={linkStart} parts={partsOf(fetched)} />);
    await click(buttonWithText(mounted.container, "Connect"));
    await settleUntil(() => heard.at(-1)?.startsWith("/dead-end") === true);
    expect(mounted.container.textContent).toContain(
      "This network seems to have no IPv6, and your server has no IPv4 way in yet",
    );
    expect(heard.at(-1)).toBe(
      `/dead-end {"bootstrapToken":"a-bootstrap-token","ipv6":"no","tried":[{"kind":"ipv6","ended":"unreachable"},{"kind":"webrtc","ended":"failed"}],"cause":"noIpv6"}`,
    );
  });

  it("says today's words when the network has IPv6 or the server an IPv4 way in (D398)", async () => {
    const v6Only = [{ kind: "ipv6", url: "https://v6.k7xq2.zone.example" }];
    const both = [...v6Only, { kind: "ipv4", url: "https://v4.k7xq2.zone.example" }];
    const cases: readonly [typeof v6Only, string][] = [
      // The discovery saw the page over IPv6: this network has it, whatever the names did.
      [v6Only, "2001:db8:1:2::/64"],
      // The server has an IPv4 way in, and it failed too: the cause is something else.
      [both, "203.0.113.0/24"],
    ];
    for (const [paths, clientPrefix] of cases) {
      const heard: string[] = [];
      const shown = await mount(
        <DiscoveryGate
          start={linkStart}
          parts={partsOf(unreachedCore(paths, clientPrefix, heard))}
        />,
      );
      await click(buttonWithText(shown.container, "Connect"));
      await settleUntil(() => heard.at(-1)?.startsWith("/dead-end") === true);
      expect(shown.container.textContent).toContain(
        "Try your phone's mobile data, or another Wi-Fi",
      );
      expect(shown.container.textContent).not.toContain("no IPv6");
      expect(heard.at(-1)).toContain(`"cause":"unknown"`);
      shown.unmount();
    }
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
    expect(stepsSoFar().at(-1)).toMatch(/reached the core over direct$/);
  });

  // Firefox with no service worker (staging, 2026-11-02): the core held the connection, the page
  // closed it and said its network could not reach the server.
  it("says a browser that runs no service worker cannot play, and tells the discovery of no dead end", async () => {
    const heard: string[] = [];
    const located = {
      coreId: "k7xq2",
      bootstrapToken: "a-bootstrap-token",
      iceServers: [],
      paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
    };
    const toDiscovery = discovery({ "/locate": [200, located], "/dead-end": [204, {}] }, heard);
    const fetched: typeof fetch = (url, init) =>
      (url instanceof Request ? url.url : url.toString()).startsWith(
        "https://v4.k7xq2.zone.example/",
      )
        ? Promise.resolve(new Response("{}", { status: 200 }))
        : toDiscovery(url, init);
    let isTried = false;
    const parts: GateParts = {
      ...partsOf(fetched),
      onConnected: () => {
        isTried = true;
        throw new NoServiceWorker();
      },
    };
    mounted = await mount(<DiscoveryGate start={linkStart} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    const shown = mounted.container;
    await settleUntil(() => shown.textContent.includes("can't play"));
    const page = shown.textContent;
    expect(isTried).toBe(true);
    expect(page).toContain("This browser can't play from the server");
    expect(page).toContain("Copy what happened");
    expect(page).not.toContain("Can't reach the server from this network");
    expect(heard.filter((each) => each.startsWith("/dead-end"))).toEqual([]);
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
    expect(stepsSoFar().at(-1)).toMatch(/reached the core over direct$/);
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
