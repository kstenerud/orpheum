// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { PageNotControlled, type FetchRequest, type KeptCore } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import {
  GateMessage,
  pathLostRetryAfterMs,
  pathLostRetryFirstMs,
  pathLostRetryMostMs,
} from "./discovery-gate-message.tsx";
import type { Start } from "./discovery-start.ts";
import { HandOffRefused, isLoaderPathLost, redeemOnce } from "./loader-at-client-origin.tsx";
import type { OrpheumLoader } from "./loader-interface.ts";
import { checkedClient, ClientRefused } from "./loader-run-client.ts";
import { createSession } from "./session.ts";
import { buttonWithText, click, mount, settleUntil, type Mounted } from "./test-dom.tsx";
import { clientPage, coreServing } from "./test-signed-core.ts";

/*
 * The loader's page waits out a lost path while it is open (§28): the owner's train, 2026-11-13,
 * where one channel lost mid-manifest ended the page for good, and the network was back in minutes.
 */

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

/** The path the manifest is asked at. */
const manifestPath = "/api/v1/client-manifest";

/** What rtc-request says of a channel closed before its answer ended. */
function channelClosed(): Error {
  return new Error(`the channel for ${manifestPath} closed before its response ended`);
}

/**
 * The discovery, which finds the core by a link at its own published name, and the core's name,
 * which answers its health: a reach that gets through each time, with the dead ends it is told heard.
 */
function discoveryFinding(heard: string[]): typeof fetch {
  const located = {
    coreId: "k7xq2",
    bootstrapToken: "a-bootstrap-token",
    iceServers: [],
    paths: [{ kind: "ipv4", url: "https://v4.k7xq2.zone.example" }],
  };
  return (url) => {
    const address = url instanceof Request ? url.url : url.toString();
    if (address.startsWith("https://v4.k7xq2.zone.example/")) {
      return Promise.resolve(new Response("{}", { status: 200 }));
    }
    const path = address.replace("https://discovery.example/client/v1", "");
    heard.push(path);
    return Promise.resolve(
      path === "/locate"
        ? Response.json(located)
        : new Response(null, { status: path === "/dead-end" ? 204 : 404 }),
    );
  };
}

/** A fingerprint of a key other than any core's here. */
const anotherFingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";

/** A link to the core whose key has fingerprint. */
function linkTo(fingerprint: string): Start {
  return { kind: "link", link: { coreId: "k7xq2", secret: "a-links-secret", fingerprint } };
}

/** The gate's parts at a client origin, as the loader gives them, each connection told to connected. */
function loaderParts(
  connected: (core: KeptCore) => Promise<void>,
  heard: string[] = [],
): GateParts {
  return {
    client: { clientApi: "https://discovery.example/client/v1", fetch: discoveryFinding(heard) },
    sessionOf: () => undefined,
    hasRealms: false,
    realmStorage: undefined,
    forgetCore: () => undefined,
    isPathLost: isLoaderPathLost,
    onConnected: connected,
  };
}

/** A send whose manifest's answer begins and is then lost, as its channel closes on a train. */
function losingTheManifest(send: FetchRequest): FetchRequest {
  return (request) => {
    if (new URL(request.url).pathname !== manifestPath) {
      return send(request);
    }
    const body = new ReadableStream<Uint8Array>({
      start: (controller) => {
        controller.enqueue(new TextEncoder().encode('{"manifest":"'));
        controller.error(channelClosed());
      },
    });
    return Promise.resolve(new Response(body, { headers: { "Content-Type": "application/json" } }));
  };
}

/** Advances the fake clock by stepMs at a time, at most steps times, until condition holds. */
async function advanceUntil(
  condition: () => boolean,
  stepMs: number,
  steps: number,
): Promise<void> {
  for (let step = 0; step < steps && !condition(); step += 1) {
    await vi.advanceTimersByTimeAsync(stepMs);
  }
}

/** The page's text that says it waits for the network. */
const waitingWords = "Waiting for the network";

describe("the loader's page while its path is lost (§28)", () => {
  it("loads the core's client on a new connection once a lost channel ended its manifest, with no reload", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const served = await coreServing(clientPage);
    // Each connection's own send: the first loses the manifest's answer, the next carries it.
    const askedOn: string[][] = [];
    const mountedOn: number[] = [];
    const parts = loaderParts(async (core) => {
      const connection = askedOn.push([]);
      const carried = connection === 1 ? losingTheManifest(served.send) : served.send;
      const run = await checkedClient({
        send: (request) => {
          askedOn[connection - 1]?.push(new URL(request.url).pathname);
          return carried(request);
        },
        fingerprint: core.fingerprint,
        refusedVersions: () => Promise.resolve([]),
        inertPage: document,
        mount: () => {
          mountedOn.push(connection);
        },
        tellWorker: () => Promise.resolve(),
      });
      run({ major: 1 } as unknown as OrpheumLoader);
    });
    mounted = await mount(<DiscoveryGate start={linkTo(served.fingerprint)} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes(waitingWords));
    expect(page.textContent).not.toContain("Can't reach the server from this network");
    expect(mountedOn).toEqual([]);
    await advanceUntil(() => askedOn.length > 1, 500, (2 * pathLostRetryMostMs) / 500);
    // The manifest's signature is checked by the platform's crypto, in its own time.
    await settleUntil(() => mountedOn.length > 0);
    expect(mountedOn).toEqual([2]);
    // The manifest is asked for again, on the new connection.
    expect(askedOn).toEqual([[manifestPath], [manifestPath, "/"]]);
  });

  it("keeps a refusal as a refusal: a manifest another key signed is never tried again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const served = await coreServing(clientPage);
    let tries = 0;
    const parts = loaderParts(async (core) => {
      tries += 1;
      await checkedClient({
        send: served.send,
        fingerprint: core.fingerprint,
        refusedVersions: () => Promise.resolve([]),
        inertPage: document,
        mount: () => undefined,
        tellWorker: () => Promise.resolve(),
      });
    });
    mounted = await mount(<DiscoveryGate start={linkTo(anotherFingerprint)} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("Can't reach the server from this network"));
    await vi.advanceTimersByTimeAsync(2 * pathLostRetryMostMs);
    expect(tries).toBe(1);
    expect(page.textContent).not.toContain(waitingWords);
  });

  it("waits out no refusal: the client's, a hand-off's, or a page no worker controls", () => {
    expect(isLoaderPathLost(new ClientRefused("unsigned", "unsigned"))).toBe(false);
    expect(isLoaderPathLost(new ClientRefused("pageChanged", "changed"))).toBe(false);
    expect(isLoaderPathLost(new ClientRefused("refusedVersion", "refused"))).toBe(false);
    expect(isLoaderPathLost(new HandOffRefused("spent"))).toBe(false);
    expect(isLoaderPathLost(new PageNotControlled())).toBe(false);
    expect(isLoaderPathLost(channelClosed())).toBe(true);
    expect(isLoaderPathLost(new AggregateError([new Error("no path connected")]))).toBe(true);
  });

  it("takes a hand-off's session once a page, asking again after a lost answer under the same key", async () => {
    const keys: (string | null)[] = [];
    const send: FetchRequest = (request) => {
      keys.push(request.headers.get("Idempotency-Key"));
      return keys.length === 1
        ? Promise.reject(channelClosed())
        : Promise.resolve(
            Response.json({
              token: "a-token",
              deviceSecret: "a-secret",
              user: { id: 1, name: "karl", role: "admin", isAdmin: true },
            }),
          );
    };
    const items = new Map<string, string>();
    const session = createSession({
      getItem: (key) => items.get(key) ?? null,
      setItem: (key, value) => {
        items.set(key, value);
      },
      removeItem: (key) => {
        items.delete(key);
      },
    });
    const takeHandOff = redeemOnce("a-code");
    await expect(takeHandOff(send, session)).rejects.toThrow(channelClosed().message);
    await takeHandOff(send, session);
    await takeHandOff(send, session);
    expect(keys).toHaveLength(2);
    expect(keys[0]).not.toBeNull();
    expect(keys[1]).toBe(keys[0]);
    expect(session.token()).toBe("a-token");
    // A code the core refuses, as once it was spent, is a refusal, never waited out.
    const refused = redeemOnce("a-spent-code");
    const spent = (): Promise<Response> =>
      Promise.resolve(Response.json({ code: "spent" }, { status: 409 }));
    await expect(refused(spent, session)).rejects.toBeInstanceOf(HandOffRefused);
  });

  it("says it waits for the network, and tries again at once on Try again and back online", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const triedAt: number[] = [];
    const heard: string[] = [];
    const parts = loaderParts(() => {
      triedAt.push(Date.now());
      return Promise.reject(channelClosed());
    }, heard);
    mounted = await mount(<DiscoveryGate start={linkTo(anotherFingerprint)} parts={parts} />);
    await click(buttonWithText(mounted.container, "Connect"));
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes(waitingWords));
    expect(page.textContent).toContain(
      "The connection to your server was lost. This page keeps trying, and carries on by itself once the network is back.",
    );
    // Told to the discovery as a dead end, with what happened to copy.
    expect(heard).toContain("/dead-end");
    expect(buttonWithText(page, "Copy what happened")).not.toBeNull();
    const pressedAt = Date.now();
    await click(buttonWithText(page, "Try again"));
    await advanceUntil(() => triedAt.length > 1, 100, 10);
    expect(triedAt).toHaveLength(2);
    expect((triedAt[1] ?? Infinity) - pressedAt).toBeLessThan(pathLostRetryFirstMs);
    // Back online once that try has ended: a try under way is never asked to start again, and its
    // end runs through the platform's crypto in its own time, so the browser says it now and then.
    const deadEnds = (): number => heard.filter((path) => path === "/dead-end").length;
    await advanceUntil(() => deadEnds() > 1, 100, (2 * pathLostRetryFirstMs) / 100);
    // Said each tenth of a second for 8 s at most: the try's own timer waits 10 s (twice the first).
    let saidAt = Date.now();
    for (let said = 0; said < 80 && triedAt.length < 3; said += 1) {
      saidAt = Date.now();
      window.dispatchEvent(new Event("online"));
      await new Promise((resolve) => {
        setTimeout(resolve, 100);
      });
    }
    expect(triedAt).toHaveLength(3);
    expect((triedAt[2] ?? Infinity) - saidAt).toBeLessThan(pathLostRetryFirstMs);
    expect(page.textContent).toContain(waitingWords);
  });

  it("tries by itself on a backoff that doubles from its first wait and never passes its ceiling", async () => {
    expect([1, 2, 3, 4, 5, 6, 12].map(pathLostRetryAfterMs)).toEqual([
      5000, 10_000, 20_000, 40_000, 60_000, 60_000, 60_000,
    ]);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    // The clock also moves as the test runs, a few milliseconds, inside the margin.
    const marginMs = 200;
    for (const tries of [1, 3, 12]) {
      let retried = 0;
      const waiting = {
        tries,
        retry: () => {
          retried += 1;
        },
      };
      const shown = await mount(
        <GateMessage
          phase="pathLost"
          cause="unknown"
          onRetry={() => undefined}
          waiting={waiting}
          after={null}
        />,
      );
      const waitMs = Math.min(pathLostRetryFirstMs * 2 ** (tries - 1), pathLostRetryMostMs);
      await vi.advanceTimersByTimeAsync(waitMs - marginMs);
      expect(retried).toBe(0);
      await vi.advanceTimersByTimeAsync(2 * marginMs);
      expect(retried).toBe(1);
      shown.unmount();
    }
  });
});
