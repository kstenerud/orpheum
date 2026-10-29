// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
import type { JSX } from "react";
import { afterEach, describe, expect, it, vi, type Mock } from "vitest";

import { clientScriptHeader, createUpdateWatch } from "./app-update.ts";
import type { StringStorage } from "./browser-storage.ts";
import { sendFailuresToCore } from "./failure-reports.ts";
import {
  assetNamedBy,
  newerClientOver,
  reloadsForNewerClient,
  reportReloadForNewerClient,
  watchForNewerClient,
  type NewerClient,
} from "./newer-client.ts";
import { LaterScreen, lazyScreen } from "./screen-load.tsx";
import { bodyField, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { testServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  watchForNewerClient(undefined);
  sendFailuresTo(() => undefined);
  vi.restoreAllMocks();
});

function memoryStorage(): StringStorage {
  const kept = new Map<string, string>();
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => {
      kept.set(key, value);
    },
    removeItem: (key) => {
      kept.delete(key);
    },
  };
}

/** A page no loader runs, and one a loader runs, each controlled by its worker. */
const page = { isUnderLoader: false, isControlled: true };
const loaderPage = { isUnderLoader: true, isControlled: true };

/** A screen, whose code never comes here. */
type AScreen = () => JSX.Element;

/** What Chrome said of the title screen's chunk a deploy took away (the owner's Mac, 2026-10-29). */
const goneChunk = new TypeError(
  "Failed to fetch dynamically imported module: https://c.example/assets/title-D5G0YPLU.js",
);

/** A page running index-OLD.js, its tab's storage as given, with what it learns of the served client. */
function clientOf(
  storage: StringStorage,
  learns: { readonly isServed?: boolean; readonly isLeftOut?: boolean } = {},
): NewerClient & { readonly reload: Mock<() => void> } {
  return {
    ownScript: "index-OLD.js",
    isServed: async () => await Promise.resolve(learns.isServed ?? false),
    isLeftOutByWorker: async () => await Promise.resolve(learns.isLeftOut ?? false),
    storage: () => storage,
    reload: vi.fn<() => void>(),
  };
}

/** Shows the title screen, whose code never loads. */
async function mountGoneTitle(): Promise<Mounted> {
  const load = vi.fn<() => Promise<{ default: AScreen }>>().mockRejectedValue(goneChunk);
  const Lazy = lazyScreen("title", load, 0);
  return await mount(
    <LaterScreen resetKey="title">
      <Lazy />
    </LaterScreen>,
  );
}

// A tab open across a deploy opened a film's page, whose code had another name now, and said "This
// page could not load" (the owner's Mac, staging, 2026-10-29).
describe("a screen's code no longer served, as a newer client is", () => {
  it("reloads the page once, showing no failure, and tells the core's log on the page after", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const storage = memoryStorage();
    const client = clientOf(storage, { isServed: true });
    watchForNewerClient(client);
    mounted = await mountGoneTitle();
    await settleUntil(() => client.reload.mock.calls.length > 0);
    expect(client.reload).toHaveBeenCalledTimes(1);
    expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
    expect(mounted.container.textContent).toContain("Loading…");
    expect(sent).toEqual([]);
    // The page loaded again tells the core's log why, once.
    reportReloadForNewerClient(storage);
    reportReloadForNewerClient(storage);
    expect(sent).toHaveLength(1);
    expect(sent[0]).toContain("the page reloaded for a newer client, once");
    expect(sent[0]).toContain("the code of the title screen did not load");
    expect(sent[0]).toContain("/assets/title-D5G0YPLU.js");
    expect(sent[0]).toContain("the server serves another client");
  });

  it("reaches the core on the page after the reload, through client-main.tsx's own wiring order: the failure sink before the reload's note is read (VQ137)", async () => {
    const storage = memoryStorage();
    // The page before the reload stores why, and reloads (newer-client.ts's own doing).
    const before = clientOf(storage, { isServed: true });
    expect(
      await reloadsForNewerClient({ screen: "title", reason: goneChunk.message }, page, before),
    ).toBe(true);
    // The page after the reload: already signed in, as a browser kept signed in across one is.
    // client-main.tsx wires sendFailuresToCore before it calls reportReloadForNewerClient; this
    // is that same order, over the real sink and a stand-in of the core.
    const heard: Heard[] = [];
    const services = servedBy({ "POST /client-problems": [204, undefined] }, heard, "a-token");
    sendFailuresToCore(services.api, services.session, () => "/titles/5285");
    reportReloadForNewerClient(storage);
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    expect(heard).toHaveLength(1);
    const [first] = heard;
    expect(first?.methodAndPath).toBe("POST /client-problems");
    expect(first === undefined ? undefined : bodyField(first, "page")).toBe("/titles/5285");
    expect(first === undefined ? undefined : bodyField(first, "message")).toEqual(
      expect.stringContaining("the page reloaded for a newer client, once"),
    );
  });

  it("shows the failure, reloading no more, where a reload from the same build did not help", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const storage = memoryStorage();
    const first = clientOf(storage, { isServed: true });
    expect(
      await reloadsForNewerClient({ screen: "title", reason: goneChunk.message }, page, first),
    ).toBe(true);
    const again = clientOf(storage, { isServed: true });
    watchForNewerClient(again);
    mounted = await mountGoneTitle();
    await settleUntil(() => mounted?.container.querySelector('[role="alert"]') !== null);
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toBe(
      "This page could not load.",
    );
    expect(again.reload).not.toHaveBeenCalled();
  });

  it("reloads where the worker's client files no longer name the screen's code (D308)", async () => {
    const storage = memoryStorage();
    const isLeftOut = vi.fn(async () => await Promise.resolve(true));
    const client = { ...clientOf(storage), isLeftOutByWorker: isLeftOut };
    expect(
      await reloadsForNewerClient(
        { screen: "title", reason: goneChunk.message },
        loaderPage,
        client,
      ),
    ).toBe(true);
    expect(isLeftOut).toHaveBeenCalledWith("assets/title-D5G0YPLU.js");
    expect(client.reload).toHaveBeenCalledTimes(1);
  });

  it("shows the failure where the client served is the page's own", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const client = clientOf(memoryStorage());
    watchForNewerClient(client);
    mounted = await mountGoneTitle();
    await settleUntil(() => mounted?.container.querySelector('[role="alert"]') !== null);
    expect(client.reload).not.toHaveBeenCalled();
  });

  it("never reloads under a loader whose page no worker controls, as with DevTools' bypass", async () => {
    const client = clientOf(memoryStorage(), { isServed: true });
    expect(
      await reloadsForNewerClient(
        { screen: "title", reason: goneChunk.message },
        { isUnderLoader: true, isControlled: false },
        client,
      ),
    ).toBe(false);
    expect(client.reload).not.toHaveBeenCalled();
  });

  it("never reloads a development page, which has no build to key its one reload by", async () => {
    const client = { ...clientOf(memoryStorage(), { isServed: true }), ownScript: undefined };
    expect(
      await reloadsForNewerClient({ screen: "title", reason: goneChunk.message }, page, client),
    ).toBe(false);
    expect(client.reload).not.toHaveBeenCalled();
  });

  it("learns a newer client is served by asking the server, whose answers name it", async () => {
    const watch = createUpdateWatch(memoryStorage);
    const services = testServices(undefined, "/titles/1", async () => {
      await Promise.resolve();
      return new Response(JSON.stringify({ status: "ready", reasons: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json", [clientScriptHeader]: "index-NEW.js" },
      });
    });
    services.api.use(watch.middleware("index-OLD.js"));
    const client = newerClientOver(services.api, "index-OLD.js", watch);
    expect(await client.isServed()).toBe(true);
  });

  it("finds the client's file a failed import names", () => {
    expect(assetNamedBy(goneChunk.message)).toBe("assets/title-D5G0YPLU.js");
    expect(
      assetNamedBy(
        "error loading dynamically imported module: https://c.example/assets/play-BOY.js",
      ),
    ).toBe("assets/play-BOY.js");
    expect(assetNamedBy("Importing a module script failed.")).toBeUndefined();
  });
});
