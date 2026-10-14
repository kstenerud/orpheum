// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { locateHash } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import { discoveryClientApiOf, rememberCore, startOf, type KeptCore } from "./discovery-start.ts";
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

  it("with no link and no server reached before, says how to get in", async () => {
    mounted = await mount(
      <DiscoveryGate start={{ kind: "none" }} parts={partsOf(discovery({}, []))} />,
    );
    expect(mounted.container.textContent).toContain("Open the link you were sent");
  });
});
