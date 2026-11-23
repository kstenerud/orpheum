// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { localNetworkNoteDismissedKey } from "./local-network-note.tsx";
import { plainHttpNoteDismissedKey } from "./plain-http-note.tsx";
import { ShellBanners } from "./shell-banners.tsx";
import { buttonNamed, click, mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { securityUpdateSnoozeKey, type Update } from "./update.tsx";
import { withServices } from "./test-services.tsx";

/** A server whose version lacks a security fix. */
const pending: Update = {
  version: "v0.3.1",
  latest: "v0.3.3",
  digest: `sha256:${"a".repeat(64)}`,
  security: { summary: "A sign-in could be guessed.", fixedIn: "v0.3.2" },
  command: "docker run --rm ghcr.io/kstenerud/orpheum:v0.3.3 install-script | sh",
  checkedAt: 1_793_000_000_000,
};

describe("the shell's banners (D538 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(globalThis, "isSecureContext");
    for (const key of [plainHttpNoteDismissedKey, localNetworkNoteDismissedKey]) {
      localStorage.removeItem(key);
    }
    localStorage.removeItem(securityUpdateSnoozeKey);
  });

  /** The banners' words shown, top to bottom. */
  function shown(): readonly string[] {
    return [...(mounted?.container.querySelectorAll(".banner") ?? [])].map(
      (each) => each.textContent,
    );
  }

  it("shows two at once in the spec's order, security first, the rest waiting until one goes", async () => {
    Reflect.set(globalThis, "isSecureContext", false);
    mounted = await mount(
      withServices(<ShellBanners isAdmin={true} />, {
        ...servedBy({ "GET /system/update": [200, pending] }),
        onLocalNetworkAsking: (listener) => {
          listener(true);
          return () => undefined;
        },
      }),
    );
    const current = mounted;
    await settleUntil(() => current.container.textContent.includes("A security update"));
    const [security, plainHttp] = shown();
    expect(shown()).toHaveLength(2);
    expect(security).toContain("A security update is available (v0.3.2)");
    expect(plainHttp).toContain("plain http");
    // The local network's waits: dismissing the plain http one brings it.
    const banners = [...current.container.querySelectorAll<HTMLElement>(".banner")];
    await click(buttonNamed(banners[1] ?? current.container, "Dismiss"));
    expect(shown()).toHaveLength(2);
    expect(shown()[1]).toContain("Your browser is asking to reach devices on your network.");
  });
});
