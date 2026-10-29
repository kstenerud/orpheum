// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TogetherEndReason } from "@orpheum/sdk";
import { beforeEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import { buttonWithText, click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import type { Services } from "./services.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

/**
 * A guest's page, signed in by token or by link alone, back in their session, which the server
 * then ends for reason.
 */
async function endedFor(
  reason: TogetherEndReason,
  token: string | undefined,
): Promise<{ readonly mounted: Mounted; readonly services: Services }> {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /together/mine": [200, { sessions: [] }] }, [], token),
    navigation: createNavigation(fakePlace("/together")),
    openTogetherSocket: server.openSocket,
  };
  const mounted = await mount(withServices(<TogetherViewer />, services));
  // A guest back in the session taps through its first page (TapToContinue).
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await server.accept();
  await server.say(welcome);
  await server.say({ type: "ended", reason });
  return { mounted, services };
}

// A page that said only "You can close this page" stopped a signed-in guest's app as their session
// ended (the owner, 2026-10-29): they go home, told why there.
describe("a Watch Together guest's page as their session ends", () => {
  it("takes a signed-in guest home, with a notice of why", async () => {
    const { mounted, services } = await endedFor("hostEnded", "signed-in-token");
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(currentNotice()?.text).toBe("The host ended the Watch Together session.");
    expect(mounted.container.textContent).not.toContain("You can close this page");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("keeps a signed-in guest sent away for the host's connection on the page, with Join again (D286 §5)", async () => {
    const { mounted, services } = await endedFor("uploadFull", "signed-in-token");
    expect(services.navigation.route()).toEqual({ screen: "together" });
    expect(currentNotice()).toBeUndefined();
    expect(buttonWithText(mounted.container, "Join again")).toBeDefined();
    expect(window.localStorage.getItem("orpheum.together.viewer")).not.toBeNull();
    mounted.unmount();
  });

  it("tells a guest by link alone on the page itself, having nowhere else to go", async () => {
    const { mounted, services } = await endedFor("ejected", undefined);
    expect(services.navigation.route()).toEqual({ screen: "together" });
    expect(currentNotice()).toBeUndefined();
    expect(mounted.container.textContent).toContain(
      "The host removed you from the session. You can close this page.",
    );
    mounted.unmount();
  });
});
