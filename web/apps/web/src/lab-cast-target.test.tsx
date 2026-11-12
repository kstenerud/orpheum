// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { CastSession } from "./lab-cast-sdk.ts";
import { CastTargetPart, refusalOf } from "./lab-cast-target.tsx";
import { buttonWithText, click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A cast session whose receiver the test speaks for: what the page sent, and the page's listener. */
function fakeSession(): {
  readonly session: CastSession;
  readonly sent: Readonly<Record<string, unknown>>[];
  readonly receiverSays: (message: unknown) => void;
} {
  const sent: Readonly<Record<string, unknown>>[] = [];
  let listener: ((message: unknown) => void) | undefined;
  const session: CastSession = {
    receiverName: "Attic TV",
    send: (message) => {
      sent.push(message);
      return Promise.resolve();
    },
    onMessage: (each) => {
      listener = each;
    },
    onEnded: () => undefined,
    end: () => undefined,
    load: () => Promise.resolve(undefined),
    onPlayer: () => undefined,
  };
  return {
    session,
    sent,
    receiverSays: (message) => {
      act(() => {
        listener?.(message);
      });
    },
  };
}

describe("the lab's cast target step (the 11-24 batch's item 3)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function mountStep(
    session: CastSession,
    log: (line: string) => void,
  ): Promise<{ readonly press: () => Promise<void>; readonly text: () => string }> {
    const services = servedBy({}, [], "token");
    mounted = await mount(
      withServices(
        <CastTargetPart cast={session} request={{ type: "device" }} log={log} />,
        services,
      ),
    );
    const container = mounted.container;
    return {
      press: async () => {
        await click(buttonWithText(container, "Make it a cast target"));
      },
      text: () => container.textContent,
    };
  }

  it("says the TV is a cast target only once the receiver says the core has taken it", async () => {
    const { session, sent, receiverSays } = fakeSession();
    const page = await mountStep(session, () => undefined);
    await page.press();
    expect(sent).toEqual([{ type: "device" }]);
    expect(page.text()).toContain("Waiting for the core to take the Chromecast's sign-in.");
    expect(page.text()).not.toContain("is a cast target");
    receiverSays({ type: "device", isReady: true });
    expect(page.text()).toContain("Attic TV is a cast target");
    expect(page.text()).not.toContain("Waiting for the core");
  });

  it("shows the core's refusal where the receiver tells it, and logs it, in place of a cast target", async () => {
    const { session, receiverSays } = fakeSession();
    const log: string[] = [];
    const page = await mountStep(session, (line) => log.push(line));
    await page.press();
    receiverSays({ type: "device", isReady: false, refusal: "sign in first" });
    expect(page.text()).toContain("Attic TV is not a cast target: sign in first.");
    expect(page.text()).not.toContain("is a cast target");
    expect(page.text()).not.toContain("Waiting for the core");
    expect(log).toEqual(["the core did not take Attic TV as a cast target: sign in first"]);
    // Pressed again, the refusal gives way to the wait for the new answer.
    await page.press();
    expect(page.text()).not.toContain("is not a cast target");
    expect(page.text()).toContain("Waiting for the core");
  });

  it("takes a refusal after readiness for the end of the target, as when the device was removed", async () => {
    const { session, receiverSays } = fakeSession();
    const page = await mountStep(session, () => undefined);
    await page.press();
    receiverSays({ type: "device", isReady: true });
    receiverSays({
      type: "device",
      isReady: false,
      refusal: "the core refused the receiver's sign-in",
    });
    expect(page.text()).toContain("Attic TV is not a cast target");
    expect(page.text()).not.toContain("Attic TV is a cast target");
  });
});

describe("a receiver's refusal as a message", () => {
  it("is the reason of a device message that is not ready, and nothing else", () => {
    expect(refusalOf({ type: "device", isReady: false, refusal: "no" })).toBe("no");
    expect(refusalOf({ type: "device", isReady: false })).toBeUndefined();
    expect(refusalOf({ type: "device", isReady: true, refusal: "no" })).toBeUndefined();
    expect(refusalOf({ type: "other", isReady: false, refusal: "no" })).toBeUndefined();
    expect(refusalOf(undefined)).toBeUndefined();
  });
});
