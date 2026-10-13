// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { CodeSignIn } from "./code-sign-in.tsx";
import type { Ask, Poller } from "./polling.ts";
import { mount, servedBy, type Answer, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
});

/** A pairing the server gives, its code lasting ten minutes from now. */
function pairing(secret: string, code: string): Answer {
  return [200, { secret, code, expiresAt: Date.now() + 600_000 }];
}

/** A poller the test runs by hand: each look the screen starts is kept. */
function handPoller(): Poller & { readonly asks: Ask[] } {
  const asks: Ask[] = [];
  return {
    asks,
    start: (ask) => {
      asks.push(ask);
      return () => undefined;
    },
  };
}

/** Runs the screen's next look at its pairing, and lets its answers come. */
async function nextLook(poller: { readonly asks: readonly Ask[] }): Promise<void> {
  const ask = poller.asks.at(-1);
  if (ask === undefined) {
    throw new Error("nothing is polling");
  }
  await act(async () => {
    await new Promise<void>((resolve) => {
      ask(() => {
        resolve();
      });
    });
  });
}

describe("a device signing in by the code it shows (D288)", () => {
  it("shows its code, then the check code once it is entered, then signs in once confirmed", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /pairings": pairing("s1", "abcd-efgh"),
        "POST /pairings/state": [
          [200, { state: "checking", checkCode: "wxyz-0123" }],
          [200, { state: "activated" }],
        ],
        "POST /pairings/session": [
          201,
          { token: "tv-token", deviceSecret: "tv-secret", user: { id: 2, name: "partner" } },
        ],
      },
      heard,
    );
    const poller = handPoller();
    mounted = await mount(
      withServices(<CodeSignIn onBack={() => undefined} poller={poller} />, services),
    );
    expect(mounted.container.textContent).toContain("abcd-efgh");
    expect(mounted.container.textContent).toContain("under Activate a device");
    await nextLook(poller);
    expect(mounted.container.textContent).toContain("wxyz-0123");
    expect(mounted.container.textContent).not.toContain("abcd-efgh");
    await nextLook(poller);
    expect(services.session.token()).toBe("tv-token");
    expect(services.session.deviceProof()).toBe("tv-secret");
    expect(heard.map((request) => request.methodAndPath)).toEqual([
      "POST /pairings",
      "POST /pairings/state",
      "POST /pairings/state",
      "POST /pairings/session",
    ]);
  });

  it("asks for a new code once its pairing ends, and says so", async () => {
    const services = servedBy({
      "POST /pairings": [pairing("s1", "abcd-efgh"), pairing("s2", "jkmn-pqrs")],
      "POST /pairings/state": [200, { state: "ended" }],
    });
    const poller = handPoller();
    mounted = await mount(
      withServices(<CodeSignIn onBack={() => undefined} poller={poller} />, services),
    );
    expect(mounted.container.textContent).toContain("abcd-efgh");
    await nextLook(poller);
    expect(mounted.container.textContent).toContain("jkmn-pqrs");
    expect(mounted.container.textContent).toContain("The last code ended");
  });
});
