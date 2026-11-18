// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { isCancelledByThePerson, openCastSession } from "./cast-sdk.ts";

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
