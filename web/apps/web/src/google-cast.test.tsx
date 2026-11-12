// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { castSenderScript, type CastChannel } from "./cast-sdk.ts";
import { CastTargets } from "./cast-to.tsx";
import { DebugToolsContext } from "./debug-tools.tsx";
import { castReceiverAppIdOf } from "./discovery-page.ts";
import {
  askGuestCastGrantOf,
  castLogTickedKey,
  guestCastNamespace,
  isCastLogTicked,
  isChromeBrowser,
  linkCastSession,
} from "./google-cast.ts";
import { LiveProvider } from "./live.tsx";
import type { Services } from "./services.tsx";
import { StatusTray } from "./status-tray.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  failOnUnexpectedRequests,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** Chrome's client hints, as Chrome on a computer or on Android gives them. */
const chromeHints = {
  brands: [
    { brand: "Chromium", version: "142" },
    { brand: "Google Chrome", version: "142" },
  ],
};

/** Has the page's browser give hints, or none where undefined (Safari, Firefox, iOS). */
function givenHints(hints: unknown): void {
  Object.defineProperty(navigator, "userAgentData", { configurable: true, value: hints });
}

/** The grant the core mints for one cast: a secret. */
const grant = "grant-secret-7f3a";

/** A Google Cast device's session in Chrome's sender SDK, the receiver's answers given by the test. */
interface FakeCastDevice {
  readonly sent: (readonly [unknown, unknown])[];
  readonly say: (message: unknown) => void;
  readonly endFromChrome: () => void;
  readonly endings: () => number;
  /** How many sessions the page has asked Chrome for. */
  readonly requests: () => number;
}

/**
 * Puts a sender SDK on the page's globals whose picker picks the device named "Living room TV";
 * isOlderFirst, its first session joins a receiver page older than the sender's, left running, which
 * does not listen on the grant's namespace, and its ending stopping that page.
 */
function givenCastSdk(isOlderFirst = false): FakeCastDevice {
  const sent: (readonly [unknown, unknown])[] = [];
  const listeners: ((namespace: unknown, text: unknown) => void)[] = [];
  const stateListeners: ((event: unknown) => void)[] = [];
  let endings = 0;
  let requests = 0;
  const session = {
    getCastDevice: () => ({ friendlyName: "Living room TV" }),
    sendMessage: async (namespace: unknown, message: unknown) => {
      await Promise.resolve();
      if (isOlderFirst && requests === 1) {
        // The SDK's own answer, a bare code, to a namespace the receiver does not listen on.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- the SDK rejects with a string
        throw "invalid_parameter";
      }
      sent.push([namespace, message]);
    },
    addMessageListener: (
      _namespace: unknown,
      listener: (namespace: unknown, text: unknown) => void,
    ) => {
      listeners.push(listener);
    },
    endSession: () => {
      endings += 1;
      if (isOlderFirst && requests === 1) {
        current = null;
        for (const listener of stateListeners) {
          listener({ sessionState: "SESSION_ENDED" });
        }
      }
    },
  };
  let current: unknown = null;
  const context = {
    setOptions: () => undefined,
    requestSession: async () => {
      requests += 1;
      current = session;
      await Promise.resolve();
    },
    getCurrentSession: () => current,
    addEventListener: (_type: unknown, listener: (event: unknown) => void) => {
      stateListeners.push(listener);
    },
    removeEventListener: () => undefined,
  };
  vi.stubGlobal("cast", {
    framework: {
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContext: { getInstance: () => context },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      SessionState: { SESSION_ENDED: "SESSION_ENDED" },
    },
  });
  // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
  vi.stubGlobal("chrome", { cast: { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } } });
  return {
    sent,
    say: (message) => {
      for (const listener of listeners) {
        listener(guestCastNamespace, JSON.stringify(message));
      }
    },
    endFromChrome: () => {
      current = null;
      for (const listener of stateListeners) {
        listener({ sessionState: "SESSION_ENDED" });
      }
    },
    endings: () => endings,
    requests: () => requests,
  };
}

/** The services of a page under a discovery's loader that names its receiver application. */
function underTheLoader(
  answers: Answers,
  heard: Heard[],
  server: FakeTogetherServer,
): Services & { readonly asked: string[] } {
  const served = servedBy(answers, heard, "token");
  const asked: string[] = [];
  return {
    ...served,
    openTogetherSocket: server.openSocket,
    session: { ...served.session, locator: () => "locator-1" },
    throughDiscovery: {
      clientApi: "https://discovery.example/api",
      coreId: "core-1",
      fingerprint: "fp-1",
    },
    castReceiverAppId: "ABCD1234",
    askGuestCastGrant: async (receiverName) => {
      asked.push(receiverName);
      await Promise.resolve();
      return grant;
    },
    asked,
  };
}

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
    givenCastSdk(); // loaded already, so the page's one load is left for the next test to see
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
    const device = givenCastSdk();
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
    const device = givenCastSdk(true);
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

describe("the Cast menu's Show the cast log on the TV (the owner, 2026-11-12)", () => {
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
    const device = givenCastSdk();
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
    expect(shown.box?.parentElement?.textContent).toContain("Show the cast log on the TV");
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
