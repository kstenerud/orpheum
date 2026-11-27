// SPDX-License-Identifier: MPL-2.0

/**
 * A Google Cast device for tests (google-cast.ts): Chrome's client hints, its sender SDK with a
 * picker that picks "Living room TV", and the services of a page under a discovery's loader.
 */

import type { vi } from "vitest";

import { guestCastNamespace } from "./google-cast.ts";
import type { Services } from "./services.tsx";
import { servedBy, type Answers, type Heard } from "./test-dom.tsx";
import type { FakeTogetherServer } from "./test-together.ts";

/** Chrome's client hints, as Chrome on a computer or on Android gives them. */
export const chromeHints: { readonly brands: readonly { brand: string; version: string }[] } = {
  brands: [
    { brand: "Chromium", version: "142" },
    { brand: "Google Chrome", version: "142" },
  ],
};

/** Has the page's browser give hints, or none where undefined (Safari, Firefox, iOS). */
export function givenHints(hints: unknown): void {
  Object.defineProperty(navigator, "userAgentData", { configurable: true, value: hints });
}

/** The grant the core mints for one cast: a secret. */
export const grant = "grant-secret-7f3a";

/** A Google Cast device's session in Chrome's sender SDK, the receiver's answers given by the test. */
export interface FakeCastDevice {
  readonly sent: (readonly [unknown, unknown])[];
  readonly say: (message: unknown) => void;
  /** The person's Stop in Chrome's own Cast control: the SDK says the session ending, then ended. */
  readonly endFromChrome: () => void;
  /** The device switched off or lost: the SDK says the session ended alone (D552 §5). */
  readonly vanish: () => void;
  readonly endings: () => number;
  /** How many sessions the page has asked Chrome for. */
  readonly requests: () => number;
  /** Chrome's picker closed with no device picked from now on: the SDK rejects with "cancel". */
  readonly picksNone: () => void;
}

/**
 * Puts a sender SDK on the page's globals whose picker picks the device named "Living room TV";
 * isOlderFirst, its first session joins a receiver page older than the sender's, left running, which
 * does not listen on the grant's namespace, and its ending stopping that page.
 */
export function givenCastSdk(
  stubGlobal: typeof vi.stubGlobal,
  isOlderFirst = false,
): FakeCastDevice {
  const sdk = new FakeSdk();
  stubCastGlobals(stubGlobal, fakeContext(sdk, fakeSession(sdk, isOlderFirst)));
  return {
    sent: sdk.sent,
    say: (message) => {
      for (const listener of sdk.listeners) {
        listener(guestCastNamespace, JSON.stringify(message));
      }
    },
    endFromChrome: () => {
      sdk.tell("SESSION_ENDING");
      sdk.drop();
    },
    vanish: () => {
      sdk.drop();
    },
    endings: () => sdk.endings,
    requests: () => sdk.requests,
    picksNone: () => {
      sdk.isPickedNone = true;
    },
  };
}

/** What the fake sender SDK holds: what was sent, who listens, and its current session. */
class FakeSdk {
  readonly sent: (readonly [unknown, unknown])[] = [];
  readonly listeners: ((namespace: unknown, text: unknown) => void)[] = [];
  readonly stateListeners: ((event: unknown) => void)[] = [];
  endings = 0;
  requests = 0;
  isPickedNone = false;
  current: unknown = null;

  /** Tells the SDK's listeners its session's state changed to sessionState. */
  tell(sessionState: string): void {
    for (const listener of this.stateListeners) {
      listener({ sessionState });
    }
  }

  /** The current session gone, its state told as ended. */
  drop(): void {
    this.current = null;
    this.tell("SESSION_ENDED");
  }

  /** Counts a session's end; isOlderFirst on the first, the older receiver page stops with it. */
  ends(isOlderFirst: boolean): void {
    this.endings += 1;
    if (isOlderFirst && this.requests === 1) {
      this.drop();
    }
  }

  /** Counts a session asked for, session now the current one. */
  opens(session: unknown): void {
    this.requests += 1;
    this.current = session;
  }
}

/** A Cast session on "Living room TV"; isOlderFirst, the first joins an older receiver page. */
function fakeSession(sdk: FakeSdk, isOlderFirst: boolean): unknown {
  return {
    getCastDevice: () => ({ friendlyName: "Living room TV" }),
    sendMessage: async (namespace: unknown, message: unknown) => {
      await Promise.resolve();
      if (isOlderFirst && sdk.requests === 1) {
        // The SDK's own answer, a bare code, to a namespace the receiver does not listen on.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- the SDK rejects with a string
        throw "invalid_parameter";
      }
      sdk.sent.push([namespace, message]);
    },
    addMessageListener: (
      _namespace: unknown,
      listener: (namespace: unknown, text: unknown) => void,
    ) => {
      sdk.listeners.push(listener);
    },
    endSession: () => {
      sdk.ends(isOlderFirst);
    },
  };
}

/** The SDK's CastContext, whose picker opens session, or rejects once picksNone. */
function fakeContext(sdk: FakeSdk, session: unknown): unknown {
  return {
    setOptions: () => undefined,
    requestSession: async () => {
      if (sdk.isPickedNone) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- the SDK rejects with a string
        throw "cancel";
      }
      sdk.opens(session);
      await Promise.resolve();
    },
    getCurrentSession: () => sdk.current,
    addEventListener: (_type: unknown, listener: (event: unknown) => void) => {
      sdk.stateListeners.push(listener);
    },
    removeEventListener: () => undefined,
  };
}

/** Puts the Cast SDK's globals on the page, its CastContext's instance context. */
function stubCastGlobals(stubGlobal: typeof vi.stubGlobal, context: unknown): void {
  stubGlobal("cast", {
    framework: {
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContext: { getInstance: () => context },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      SessionState: { SESSION_ENDING: "SESSION_ENDING", SESSION_ENDED: "SESSION_ENDED" },
    },
  });
  // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
  stubGlobal("chrome", { cast: { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } } });
}

/** The services of a page under a discovery's loader that names its receiver application. */
export function underTheLoader(
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
