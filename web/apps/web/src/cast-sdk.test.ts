// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { isCancelledByThePerson, onCastSessionJoined, openCastSession } from "./cast-sdk.ts";

/**
 * Puts a Cast context on the page's globals whose requestSession tells its listeners each of states,
 * as Chrome does while the person picks, then fails with code, as the SDK does: a bare string.
 */
function givenARequestFailing(code: string, states: readonly string[]): void {
  const listeners: ((event: unknown) => void)[] = [];
  const castContext = {
    setOptions: () => undefined,
    addEventListener: (_type: unknown, listener: (event: unknown) => void) => {
      listeners.push(listener);
    },
    removeEventListener: () => undefined,
    requestSession: async () => {
      for (const sessionState of states) {
        for (const listener of listeners) {
          listener({ sessionState });
        }
      }
      await Promise.resolve();
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- the SDK rejects with a string
      throw code;
    },
  };
  Reflect.set(globalThis, "cast", {
    framework: {
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContext: { getInstance: () => castContext },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
      SessionState: {
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_STARTING: "SESSION_STARTING",
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SESSION_START_FAILED: "SESSION_START_FAILED",
      },
    },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, "cast");
});

describe("a Cast session asked for", () => {
  it("is the person's own closing of the picker where no device was picked, whatever code Chrome answers", async () => {
    // The owner, 2026-11-18: Chrome's picker found no device, they closed it, and the page said a
    // bug, Chrome having answered session_error: "Since I closed the chrome search dialog myself,
    // it should display nothing in orpheum."
    givenARequestFailing("session_error", []);
    const failure: unknown = await openCastSession("ABCD1234").catch((cause: unknown) => cause);
    expect(isCancelledByThePerson(failure)).toBe(true);
  });

  it("is a failure, not the person's choice, where a device was picked and its session could not start", async () => {
    givenARequestFailing("session_error", ["SESSION_STARTING", "SESSION_START_FAILED"]);
    const failure: unknown = await openCastSession("ABCD1234").catch((cause: unknown) => cause);
    expect(isCancelledByThePerson(failure)).toBe(false);
  });
});

describe("a Cast session the page has without the picker", () => {
  it("is the context's current one, and each one resumed or started with no picker open; a session the picker starts is the picker's alone", async () => {
    const listeners: ((event: unknown) => void)[] = [];
    const say = (sessionState: string): void => {
      for (const listener of [...listeners]) {
        listener({ sessionState });
      }
    };
    const running = { name: "running" };
    const resumed = { name: "resumed" };
    const picked = { name: "picked" };
    const startedLater = { name: "started later" };
    let current: unknown = running;
    const options: unknown[] = [];
    const castContext = {
      setOptions: (set: unknown) => {
        options.push(set);
      },
      getCurrentSession: () => current,
      addEventListener: (_type: unknown, listener: (event: unknown) => void) => {
        listeners.push(listener);
      },
      removeEventListener: (_type: unknown, listener: (event: unknown) => void) => {
        listeners.splice(listeners.indexOf(listener), 1);
      },
      requestSession: async () => {
        current = picked;
        say("SESSION_STARTING");
        say("SESSION_STARTED");
        await Promise.resolve();
      },
    };
    Reflect.set(globalThis, "cast", {
      framework: {
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        CastContext: { getInstance: () => castContext },
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SessionState: {
          // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
          SESSION_STARTING: "SESSION_STARTING",
          // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
          SESSION_STARTED: "SESSION_STARTED",
          // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
          SESSION_RESUMED: "SESSION_RESUMED",
        },
      },
    });
    const told: unknown[] = [];
    const stop = onCastSessionJoined("ABCD1234", (session) => {
      told.push(session);
    });
    expect(options).toEqual([{ receiverApplicationId: "ABCD1234", autoJoinPolicy: undefined }]);
    current = resumed;
    say("SESSION_RESUMED");
    await openCastSession("ABCD1234");
    current = startedLater;
    say("SESSION_STARTED");
    stop();
    current = running;
    say("SESSION_RESUMED");
    expect(told).toEqual([running, resumed, startedLater]);
  });
});
