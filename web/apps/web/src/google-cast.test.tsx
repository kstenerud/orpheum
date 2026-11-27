// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readFileSync } from "node:fs";
import path from "node:path";

import { apiPrefix, createApi } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { castSenderScript, type CastChannel } from "./cast-sdk.ts";
import { isCastSessionLost } from "./cast-session-lost.ts";
import { CastTargets } from "./cast-to.tsx";
import { DebugToolsContext } from "./debug-tools.tsx";
import { castReceiverAppIdOf } from "./discovery-page.ts";
import {
  askGuestCastGrantOf,
  askTogetherCastGrantOf,
  guestCastOf,
  openGuestReceiverBy,
  castLogTickedKey,
  guestCastNamespace,
  isCastLogTicked,
  isChromeBrowser,
  linkCastSession,
} from "./google-cast.ts";
import { LiveProvider } from "./live.tsx";
import { StatusTray } from "./status-tray.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  failOnUnexpectedRequests,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import {
  chromeHints,
  givenCastSdk,
  givenHints,
  grant,
  underTheLoader,
} from "./test-google-cast.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** A device of the person's, as GET /me/devices lists it. */
const thisLaptop = {
  id: 1,
  name: "Laptop",
  platform: "web",
  addedAt: 0,
  lastUsedAt: 0,
  isSignedIn: true,
  isByCode: false,
  isThisDevice: true,
};

let mounted: Mounted | undefined;
afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  Reflect.deleteProperty(navigator, "userAgentData");
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("which browsers cast to a Google Cast device (D465 §3)", () => {
  it("is Chrome alone, by its client hints: not Edge, Android's WebView, or a browser giving none", () => {
    expect(isChromeBrowser({ userAgentData: chromeHints })).toBe(true);
    const edge = { brands: [{ brand: "Chromium" }, { brand: "Microsoft Edge" }] };
    expect(isChromeBrowser({ userAgentData: edge })).toBe(false);
    const webView = { brands: [{ brand: "Chromium" }, { brand: "Android WebView" }] };
    expect(isChromeBrowser({ userAgentData: webView })).toBe(false);
    expect(isChromeBrowser({})).toBe(false); // Safari, Firefox, any browser on an iPhone
  });

  it("launches the receiver application the loader's page names, and none on the core's own page (D465 §5)", () => {
    const page = document.implementation.createHTMLDocument("loader");
    expect(castReceiverAppIdOf(page)).toBeUndefined();
    const meta = page.createElement("meta");
    meta.name = "orpheum-cast-receiver-app-id";
    meta.content = "ABCD1234";
    page.head.append(meta);
    expect(castReceiverAppIdOf(page)).toBe("ABCD1234");
  });
});

describe("a grant for one cast to a Google Cast device (D465 §2)", () => {
  it("is asked of the core for the device Chrome's picker named, and is what the core answers", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy({ "POST /me/casts/guest-grants": [201, { grant }] }, heard, "token");
    await expect(askGuestCastGrantOf(api)("Living Room TV")).resolves.toBe(grant);
    expect(heard.map((each) => [each.methodAndPath, each.body])).toEqual([
      ["POST /me/casts/guest-grants", { receiverName: "Living Room TV" }],
    ]);
    failOnUnexpectedRequests();
  });
});

describe("the Cast menu's Chromecast (D465 §3)", () => {
  it("is offered after the devices, behind a separator with a magnifying glass, in Chrome alone", async () => {
    const server = fakeTogetherServer();
    const services = underTheLoader(
      { "GET /me/devices": [200, { devices: [thisLaptop] }] },
      [],
      server,
    );
    givenCastSdk(vi.stubGlobal); // loaded already, so the page's one load is left for the next test to see
    const menu = withServices(
      <LiveProvider>
        <CastTargets film={() => undefined} />
      </LiveProvider>,
      services,
    );
    givenHints(undefined);
    mounted = await mount(menu);
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(() => mounted?.container.textContent.includes("None of your") === true);
    expect(buttonWithText(mounted.container, "Chromecast")).toBeUndefined();
    mounted.unmount();
    givenHints(chromeHints);
    mounted = await mount(menu);
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    const entry = buttonWithText(mounted.container, "Chromecast");
    expect(entry?.previousElementSibling?.tagName.toLowerCase()).toBe("hr");
    expect(entry?.firstElementChild?.getAttribute("class")).toBe("icon cast-kind cast-kind--find");
  });

  it("loads Google's sender script only once the Cast menu is first opened (the owner, 2026-11-12)", async () => {
    const server = fakeTogetherServer();
    const services = underTheLoader({ "GET /me/devices": [200, { devices: [] }] }, [], server);
    givenHints(chromeHints);
    const appended: unknown[] = [];
    vi.spyOn(document.head, "append").mockImplementation((...nodes) => {
      appended.push(...nodes); // kept, not added: a test touches no network
    });
    const senderScripts = (): number =>
      appended.filter((node) => node instanceof HTMLScriptElement && node.src === castSenderScript)
        .length;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <StatusTray isAdmin={false} canHostWatchTogether={false} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "sending" });
    expect(senderScripts()).toBe(0);
    const sign = mounted.container.querySelector<HTMLButtonElement>(
      'button[aria-label="Cast to a device"]',
    );
    await act(async () => {
      sign?.click();
      await Promise.resolve();
    });
    await settleUntil(() => senderScripts() === 1);
    expect(senderScripts()).toBe(1);
  });

  it("hands the device its grant over the Cast channel alone, casts to the device it becomes, and ends the cast when Chrome ends the session", async () => {
    const heard: Heard[] = [];
    const server = fakeTogetherServer();
    const services = underTheLoader(
      {
        "GET /me/devices": [200, { devices: [thisLaptop] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
        "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }],
      },
      heard,
      server,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    const printed: unknown[] = [];
    for (const method of ["log", "info", "warn", "error", "debug"] as const) {
      vi.spyOn(console, method).mockImplementation((...said: unknown[]) => {
        printed.push(...said);
      });
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => ({ fileId: 4, positionMs: 61_000 })} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    await click(buttonWithText(mounted.container, "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    expect(services.asked).toEqual(["Living room TV"]);
    expect(device.sent).toEqual([
      [
        guestCastNamespace,
        {
          type: "guest",
          grant,
          reach: {
            clientApi: "https://discovery.example/api",
            coreId: "core-1",
            fingerprint: "fp-1",
            clientOrigin: globalThis.location.origin,
            locator: "locator-1",
          },
        },
      ],
    ]);
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts"));
    const made = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(made === undefined ? undefined : bodyField(made, "toDeviceId")).toBe(12);
    await settleUntil(
      () => mounted?.container.textContent.includes("Asking Living room TV") === true,
    );
    // The grant went over the namespace and nowhere else: no request, storage or console has it.
    expect(
      JSON.stringify(heard.map((each) => [each.methodAndPath, each.query, each.body])),
    ).not.toContain(grant);
    const stored = [localStorage, sessionStorage].flatMap((storage) =>
      Object.keys(storage).map((key) => `${key}=${storage.getItem(key) ?? ""}`),
    );
    expect(stored.join("\n")).not.toContain(grant);
    expect(printed.map(String).join("\n")).not.toContain(grant);
    await act(async () => {
      device.endFromChrome();
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts/9/end"));
    const ended = heard.find((each) => each.methodAndPath === "POST /me/casts/9/end");
    expect(ended === undefined ? undefined : bodyField(ended, "reason")).toBe("stopped");
  });

  // The owner, staging, 2026-11-27: Chromecast pressed as the film was still loading, it played on
  // here for the seconds Chrome's picker and the device's launch took, and the TV was handed it
  // paused at its start, where it stood at the press, and stayed black.
  it("casts the film from where it stands as the device is ready, playing on here while Chrome's picker was open, not from where it stood at the press", async () => {
    const heard: Heard[] = [];
    const server = fakeTogetherServer();
    const services = underTheLoader(
      {
        "GET /me/devices": [200, { devices: [thisLaptop] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
      },
      heard,
      server,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    // Still loading at the press: its video stands paused at the film's start.
    const here = { positionMs: 85_403, isPaused: true };
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => ({ fileId: 4, ...here })} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    await click(buttonWithText(mounted.container, "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    here.positionMs = 89_477; // it started, and played on as the device launched
    here.isPaused = false;
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts"));
    const made = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(made?.body).toEqual({ toDeviceId: 12, fileId: 4, positionMs: 89_477 });
  });

  // The owner, staging, 2026-11-25: the Chromecast switched off, the phone's Cast session ended and
  // the page ended the cast as the person's Stop, so the film played on the phone by itself.
  it("ends no cast and says its Cast session lost, as that session ends with no Stop in Chrome's control: the device switched off, lost, or the phone's Wi-Fi gone (D553 §5)", async () => {
    const heard: Heard[] = [];
    const server = fakeTogetherServer();
    const services = underTheLoader(
      {
        "GET /me/devices": [200, { devices: [thisLaptop] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
        "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }],
      },
      heard,
      server,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => ({ fileId: 4, positionMs: 61_000 })} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    await click(buttonWithText(mounted.container, "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts"));
    expect(isCastSessionLost(9)).toBe(false);
    await act(async () => {
      device.vanish();
      await Promise.resolve();
    });
    await settleUntil(() => isCastSessionLost(9));
    // A hint for the remote, not an end: a Chromecast playing on is left playing, and the server's
    // silence limit ends a cast whose receiver is truly gone (D364 §6).
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
  });

  it("stops a receiver older than the page, left running on the device, and launches it again for the grant (the owner, 2026-11-12)", async () => {
    const heard: Heard[] = [];
    const server = fakeTogetherServer();
    const services = underTheLoader(
      {
        "GET /me/devices": [200, { devices: [thisLaptop] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
      },
      heard,
      server,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal, true);
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => ({ fileId: 4, positionMs: 61_000 })} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    await click(buttonWithText(mounted.container, "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    // The older page stopped, a second session launched it as it is now, which heard the grant.
    expect(device.endings()).toBe(1);
    expect(device.requests()).toBe(2);
    expect(device.sent[0]?.[0]).toBe(guestCastNamespace);
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts"));
    const made = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(made === undefined ? undefined : bodyField(made, "toDeviceId")).toBe(12);
    expect(mounted.container.textContent).not.toContain("could not take the cast");
  });

  it("ends the Cast session once its cast is over, the Cast menu closed (Stop casting on the remote)", async () => {
    const server = fakeTogetherServer();
    const services = underTheLoader({}, [], server);
    let endings = 0;
    const channel: CastChannel = {
      receiverName: "Living room TV",
      send: async () => {
        await Promise.resolve();
      },
      onMessage: () => undefined,
      onEnded: () => undefined,
      end: () => {
        endings += 1;
      },
      isOf: () => false,
    };
    mounted = await mount(
      withServices(
        <LiveProvider>
          <StatusTray isAdmin={false} canHostWatchTogether={false} />
        </LiveProvider>,
        services,
      ),
    );
    await server.accept();
    linkCastSession(services.api, channel, 5);
    const casting = {
      castId: 5,
      receiverDeviceId: 12,
      receiverName: "Living room TV",
      fileId: 4,
      itemName: "A film",
      durationMs: 0,
      isAccepted: true,
    };
    await server.say({ type: "sending", sending: casting });
    expect(endings).toBe(0);
    await server.say({ type: "sending" });
    await settleUntil(() => endings === 1);
    expect(endings).toBe(1);
  });
});

describe("the Cast menu's Show cast log (the owner, 2026-11-12)", () => {
  /**
   * Opens the Cast menu, the person showing debug tools or not, does what before says, and casts to
   * the Chromecast: what the device was handed, and the menu's box, if any, as it stood.
   */
  async function castWithLogChoice(
    showsDebugTools: boolean,
    before: (box: HTMLInputElement | null) => Promise<void> = () => Promise.resolve(),
  ): Promise<{ readonly handed: unknown; readonly box: HTMLInputElement | null }> {
    const server = fakeTogetherServer();
    const services = underTheLoader(
      { "GET /me/devices": [200, { devices: [thisLaptop] }] },
      [],
      server,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    // The device's failure below, which ends the cast the test does not want, is said on the console.
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    mounted?.unmount();
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
          <LiveProvider>
            <CastTargets film={() => undefined} />
          </LiveProvider>
        </DebugToolsContext>,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Chromecast") !== undefined,
    );
    const box = mounted.container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    await before(box);
    await click(buttonWithText(mounted.container, "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    await act(async () => {
      device.say({ type: "failed", reason: "not wanted by the test" });
      await Promise.resolve();
    });
    return { handed: device.sent[0]?.[1], box };
  }

  afterEach(() => {
    localStorage.clear();
  });

  it("is offered only while the person shows debug tools, and without them has no effect, ticked or not", async () => {
    localStorage.setItem(castLogTickedKey, "1"); // ticked while debug tools were on
    const hidden = await castWithLogChoice(false);
    expect(hidden.box).toBeNull();
    expect(hidden.handed).toMatchObject({ type: "guest", grant });
    expect(hidden.handed).not.toHaveProperty("showsLog");
    const shown = await castWithLogChoice(true);
    expect(shown.box?.checked).toBe(true);
    expect(shown.box?.parentElement?.textContent).toContain("Show cast log");
    expect(shown.handed).toMatchObject({ type: "guest", grant, showsLog: true });
  });

  it("is off until ticked, kept in this browser once ticked, and only then hands the device the log", async () => {
    const unticked = await castWithLogChoice(true);
    expect(unticked.box?.checked).toBe(false);
    expect(unticked.handed).not.toHaveProperty("showsLog");
    const ticked = await castWithLogChoice(true, async (box) => {
      await click(box);
    });
    expect(ticked.handed).toMatchObject({ showsLog: true });
    expect(isCastLogTicked()).toBe(true);
    const untickedAgain = await castWithLogChoice(true, async (box) => {
      expect(box?.checked).toBe(true);
      await click(box);
    });
    expect(untickedAgain.handed).not.toHaveProperty("showsLog");
    expect(localStorage.getItem(castLogTickedKey)).toBeNull();
  });
});

describe("a Watch Together guest by link's Chromecast (D518)", () => {
  /** Each route a guest's membership may call (x-orpheum-capability member, none or media). */
  function membersRoutes(): readonly RegExp[] {
    const document: unknown = JSON.parse(
      readFileSync(path.join(import.meta.dirname, "../../../../api/openapi.json"), "utf8"),
    );
    const routes: RegExp[] = [];
    for (const [template, operations] of Object.entries(
      Reflect.get(document as object, "paths") as object,
    )) {
      for (const [method, operation] of Object.entries(operations as object)) {
        const capability: unknown = Reflect.get(operation as object, "x-orpheum-capability");
        if (capability === "member" || capability === "none" || capability === "media") {
          const pattern = `${method.toUpperCase()} ${template.replaceAll(/\{[^/}]+\}/gu, "[^/]+")}`;
          routes.push(new RegExp(`^${pattern}$`, "u"));
        }
      }
    }
    return routes;
  }

  it("asks its grant by the guest's membership, signed in as no one, hands it over with the membership's locator, and asks for no route a guest is refused", async () => {
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    const routes = membersRoutes();
    const asked: string[] = [];
    const refused: string[] = [];
    const api = createApi({
      getToken: () => undefined,
      fetch: (request: Request) => {
        const route = `${request.method} ${new URL(request.url).pathname.slice(apiPrefix.length)}`;
        asked.push(`${route} ${request.headers.get("Authorization") ?? "signed out"}`);
        if (!routes.some((each) => each.test(route))) {
          refused.push(route);
          return Promise.resolve(new Response("{}", { status: 403 }));
        }
        const answer = JSON.stringify({ grant });
        return Promise.resolve(
          new Response(answer, { status: 201, headers: { "Content-Type": "application/json" } }),
        );
      },
    });
    const ways = guestCastOf(
      {
        castReceiverAppId: "ABCD1234",
        askGuestCastGrant: askTogetherCastGrantOf(api, "the-guests-membership"),
        throughDiscovery: {
          clientApi: "https://discovery.example/api",
          coreId: "core-1",
          fingerprint: "fp-1",
        },
      },
      globalThis.location.origin,
    );
    expect(ways).toBeDefined();
    if (ways === undefined) {
      return;
    }
    const opening = openGuestReceiverBy(ways, {
      locator: "the-memberships-locator",
      showsLog: false,
    });
    await vi.waitFor(() => {
      expect(device.sent).toHaveLength(1);
    });
    device.say({ type: "ready", deviceId: 12 });
    const receiver = await opening;
    expect(receiver?.channel.receiverName).toBe("Living room TV");
    expect(receiver?.deviceId).toBe(12);
    expect(asked).toEqual(["POST /together/cast-grants Bearer the-guests-membership"]);
    const handed = JSON.stringify(device.sent[0]?.[1]);
    expect(handed).toContain(grant);
    expect(handed).toContain("the-memberships-locator");
    expect(refused).toEqual([]);
  });
});
