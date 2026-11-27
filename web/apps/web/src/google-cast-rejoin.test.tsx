// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { isCastSessionLost } from "./cast-session-lost.ts";
import { endCastSessionOf } from "./google-cast.ts";
import { LiveProvider } from "./live.tsx";
import { StatusTray } from "./status-tray.tsx";
import {
  bodyField,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** Chrome, by its client hints, as Chrome on Android gives them. */
function givenChrome(): void {
  Object.defineProperty(navigator, "userAgentData", {
    configurable: true,
    value: { brands: [{ brand: "Google Chrome", version: "142" }] },
  });
}

let mounted: Mounted | undefined;
afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  Reflect.deleteProperty(navigator, "userAgentData");
  vi.unstubAllGlobals();
});

/** A CastSession of the SDK's on the Chromecast: whether a page linked it, and its ends asked. */
interface FakeRunningSession {
  readonly session: object;
  /** Whether a page has read its device, as linking it does (cast-sdk.ts's channelOf). */
  readonly isLinked: () => boolean;
  /** How many times a page ended it. */
  readonly ends: () => number;
}

/** A running session on "Living room TV". */
function runningSession(): FakeRunningSession {
  let isLinked = false;
  let ends = 0;
  const session = {
    getCastDevice: () => {
      isLinked = true;
      return { friendlyName: "Living room TV" };
    },
    sendMessage: async () => {
      await Promise.resolve();
    },
    addMessageListener: () => undefined,
    endSession: () => {
      ends += 1;
    },
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  };
  return { session, isLinked: () => isLinked, ends: () => ends };
}

/**
 * The sender SDK as a page of the loader's origin finds it loaded again, or come back from the
 * background: Chrome holds a session of the receiver application this origin left running, current
 * from the start, which the test resumes as a new object, ends or loses.
 */
function givenRunningCastSdk(first: FakeRunningSession): {
  /** The SDK hands the page session, as Chrome does a session it gave up and joined again. */
  readonly resume: (session: FakeRunningSession) => void;
  /** The current session's device lost: the SDK says it ended alone (D552 §5). */
  readonly vanish: () => void;
  /** The person's Stop in Chrome's own Cast control. */
  readonly endFromChrome: () => void;
  /** The application ids the context was set to. */
  readonly appIds: unknown[];
  readonly requests: () => number;
} {
  let current: unknown = first.session;
  let requests = 0;
  const appIds: unknown[] = [];
  const stateListeners: ((event: unknown) => void)[] = [];
  const say = (sessionState: string): void => {
    for (const listener of [...stateListeners]) {
      listener({ sessionState });
    }
  };
  const context = {
    setOptions: (options: unknown) => {
      appIds.push(Reflect.get(options as object, "receiverApplicationId"));
    },
    requestSession: async () => {
      requests += 1;
      await Promise.resolve();
    },
    getCurrentSession: () => current,
    addEventListener: (_type: unknown, listener: (event: unknown) => void) => {
      stateListeners.push(listener);
    },
    removeEventListener: (_type: unknown, listener: (event: unknown) => void) => {
      const at = stateListeners.indexOf(listener);
      if (at !== -1) {
        stateListeners.splice(at, 1);
      }
    },
  };
  vi.stubGlobal("cast", {
    framework: {
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContext: { getInstance: () => context },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      SessionState: {
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_STARTED: "SESSION_STARTED",
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_RESUMED: "SESSION_RESUMED",
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_ENDING: "SESSION_ENDING",
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_ENDED: "SESSION_ENDED",
      },
    },
  });
  // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
  vi.stubGlobal("chrome", { cast: { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } } });
  return {
    resume: (next) => {
      current = next.session;
      say("SESSION_RESUMED");
    },
    vanish: () => {
      current = null;
      say("SESSION_ENDED");
    },
    endFromChrome: () => {
      say("SESSION_ENDING");
      current = null;
      say("SESSION_ENDED");
    },
    appIds,
    requests: () => requests,
  };
}

/** The cast this device sends to the Chromecast, as the live connection says it. */
const sentToTheChromecast = {
  castId: 9,
  receiverDeviceId: 12,
  receiverName: "Living room TV",
  fileId: 4,
  itemName: "A film",
  durationMs: 0,
  isAccepted: true,
  isGuestReceiver: true,
};

describe("the Cast session of a cast this device sends, rejoined (the owner, 2026-11-27)", () => {
  afterEach(() => {
    act(() => {
      endCastSessionOf(9); // the link is the page's: the next test's page starts with none
    });
  });

  /** A page of the loader's, loaded again, the live connection saying it sends the Chromecast's cast. */
  async function loadedAgain(answers: Answers, heard: Heard[] = []): Promise<void> {
    const server = fakeTogetherServer();
    givenChrome();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <StatusTray isAdmin={false} canHostWatchTogether={false} />
        </LiveProvider>,
        {
          ...servedBy(answers, heard, "token"),
          openTogetherSocket: server.openSocket,
          // The loader's page names its receiver application.
          castReceiverAppId: "ABCD1234",
        },
      ),
    );
    await server.accept();
    await server.say({ type: "sending", sending: sentToTheChromecast });
  }

  /** Ends the linked session as a cast its device did not take does: the linked one is ended. */
  function endTheLinked(): void {
    act(() => {
      endCastSessionOf(9);
    });
  }

  it("links the Chromecast's session again on a page loaded again, opening no picker", async () => {
    const first = runningSession();
    const sdk = givenRunningCastSdk(first);
    await loadedAgain({});
    await settleUntil(first.isLinked);
    expect(sdk.appIds).toEqual(["ABCD1234"]);
    expect(sdk.requests()).toBe(0);
    endTheLinked();
    expect(first.ends()).toBe(1);
  });

  it("follows the session Chrome hands back as a new object, ending no cast and saying no session lost", async () => {
    const heard: Heard[] = [];
    const first = runningSession();
    const sdk = givenRunningCastSdk(first);
    await loadedAgain({}, heard);
    await settleUntil(first.isLinked);
    const second = runningSession();
    await act(async () => {
      sdk.resume(second);
      await Promise.resolve();
    });
    await settleUntil(second.isLinked);
    expect(isCastSessionLost(9)).toBe(false);
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    endTheLinked();
    expect(second.ends()).toBe(1);
    expect(first.ends()).toBe(0);
  });

  it("rejoins a session lost and resumed, the loss a hint only until then (D553 §5)", async () => {
    const heard: Heard[] = [];
    const first = runningSession();
    const sdk = givenRunningCastSdk(first);
    await loadedAgain({}, heard);
    await settleUntil(first.isLinked);
    await act(async () => {
      sdk.vanish();
      await Promise.resolve();
    });
    await settleUntil(() => isCastSessionLost(9));
    const second = runningSession();
    await act(async () => {
      sdk.resume(second);
      await Promise.resolve();
    });
    await settleUntil(second.isLinked);
    expect(isCastSessionLost(9)).toBe(false);
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    endTheLinked();
    expect(second.ends()).toBe(1);
  });

  it("still ends the cast as stopped when the rejoined session is stopped from Chrome's own Cast control (D553 §1)", async () => {
    const heard: Heard[] = [];
    const first = runningSession();
    const sdk = givenRunningCastSdk(first);
    await loadedAgain(
      { "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }] },
      heard,
    );
    await settleUntil(first.isLinked);
    const second = runningSession();
    await act(async () => {
      sdk.resume(second);
      await Promise.resolve();
    });
    await settleUntil(second.isLinked);
    await act(async () => {
      sdk.endFromChrome();
      await Promise.resolve();
    });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts/9/end"));
    const ended = heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end");
    expect(ended.map((each) => bodyField(each, "reason"))).toEqual(["stopped"]);
  });
});
