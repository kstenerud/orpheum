// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { channelOf, isCancelledByThePerson, openCastSession } from "./cast-sdk.ts";

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

/** Waits until the session has answered what was set through it, refused or not. */
async function answered(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await Promise.resolve();
  }
}

describe("a Cast device's own volume, through its session", () => {
  it("is read, set and heard as the SDK's CastSession says it, kept between 0 and 1", async () => {
    Reflect.set(globalThis, "cast", {
      framework: {
        // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own names
        SessionEventType: { VOLUME_CHANGED: "volumechanged" },
      },
    });
    const asked: string[] = [];
    const listeners = new Map<unknown, () => void>();
    const levels: number[] = [];
    const session = {
      getCastDevice: () => ({ friendlyName: "Living room TV" }),
      getVolume: () => levels.at(-1),
      isMute: () => true,
      setVolume: (to: number) => {
        asked.push(`volume ${String(to)}`);
        return Promise.resolve();
      },
      setMute: (isMuted: boolean) => {
        asked.push(`mute ${String(isMuted)}`);
        return Promise.reject(new Error("session_error"));
      },
      addEventListener: (type: string, listener: () => void) => {
        listeners.set(type, listener);
      },
      removeEventListener: (type: string) => {
        listeners.delete(type);
      },
    };
    const { volume } = channelOf(session, "urn:x-cast:org.orpheum.cast");
    expect(volume.read()).toBeUndefined();
    levels.push(0.3);
    expect(volume.read()).toEqual({ level: 0.3, isMuted: true });
    volume.setLevel(1.4);
    volume.setMuted(false);
    await answered();
    expect(asked).toEqual(["volume 1", "mute false"]);
    let heard = 0;
    const unhear = volume.onChange(() => {
      heard += 1;
    });
    listeners.get("volumechanged")?.();
    expect(heard).toBe(1);
    // A mute set through it is told as the session answers, the SDK telling no change of its own.
    volume.setMuted(true);
    await answered();
    expect(heard).toBe(2);
    unhear();
    expect(listeners.size).toBe(0);
    volume.setMuted(false);
    await answered();
    expect(heard).toBe(2);
  });
});
