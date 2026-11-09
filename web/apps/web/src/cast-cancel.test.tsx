// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { CastTargets } from "./cast-to.tsx";
import { LiveProvider } from "./live.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

/** The cast the Laptop sends to the Fire TV, unanswered, as the live connection says it (D364 §6). */
const sendingToTheTv = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 0,
  isAccepted: false,
};

/** A device of the person's, as GET /me/devices lists it. */
function device(id: number, name: string, isThisDevice = false): Record<string, unknown> {
  return {
    id,
    name,
    platform: "web",
    addedAt: 0,
    lastUsedAt: Date.UTC(2026, 9, 25, 20, 0),
    isSignedIn: true,
    isByCode: false,
    isThisDevice,
  };
}

/** Film 4 playing here at 61 s. */
const playingHere = (): { fileId: number; positionMs: number } => ({
  fileId: 4,
  positionMs: 61_000,
});

// Principles §28; the owner, 2026-11-03: Cast's dialog closed while it asked, the phone stayed asking
// with no way out, the receiver never told.
describe("a casting request still unanswered, Cast's dialog opened again", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** Cast's dialog opened again over a cast this device sends unanswered, the end answered as given. */
  async function openedOverAnUnansweredCast(
    heard: Heard[],
    end: Answers[string] = [200, { positionMs: 0, isPlaying: false }],
  ): Promise<ReturnType<typeof liveAndSession>> {
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [device(1, "Laptop", true), device(2, "Fire TV")] }],
        "POST /me/casts/9/end": end,
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={playingHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "devices", deviceIds: [1, 2] });
    await sockets.live.say({ type: "sending", sending: sendingToTheTv });
    return sockets;
  }

  // The owner, 2026-11-03: the dialog closed while it asked, the phone stayed asking with no way out.
  it("shows a cast it sends unanswered as asking when opened again, and cancels it, offering the devices once the server says it is over", async () => {
    const heard: Heard[] = [];
    const sockets = await openedOverAnUnansweredCast(heard);
    expect(mounted?.container.textContent).toContain("Asking Fire TV…");
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeUndefined();
    await click(buttonWithText(mounted!.container, "Cancel casting request"));
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toHaveLength(1);
    // Cancelled, it asks no more though the server has not yet said the cast is over.
    expect(mounted?.container.textContent).not.toContain("Asking Fire TV…");
    expect(mounted?.container.textContent).toContain(
      "You cancelled the casting request to Fire TV.",
    );
    await sockets.live.say({ type: "sending" });
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeDefined(); // to cast again
  });

  it("cancels a casting request once more where the first try fails, and says why where both do", async () => {
    const heard: Heard[] = [];
    await openedOverAnUnansweredCast(heard, [
      refusal(503, "unavailable"),
      [200, { positionMs: 0, isPlaying: false }],
    ]);
    await click(buttonWithText(mounted!.container, "Cancel casting request"));
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toHaveLength(2);
    expect(mounted?.container.textContent).toContain(
      "You cancelled the casting request to Fire TV.",
    );
    mounted?.unmount();
    await openedOverAnUnansweredCast(
      [],
      [refusal(503, "unavailable"), refusal(503, "unavailable")],
    );
    await click(buttonWithText(mounted!.container, "Cancel casting request"));
    expect(mounted?.container.querySelector("[role=alert]")).not.toBeNull();
    expect(mounted?.container.textContent).toContain("Asking Fire TV…"); // still to be cancelled
  });

  it("takes a casting request the server no longer has as cancelled", async () => {
    await openedOverAnUnansweredCast([], refusal(404, "notFound"));
    await click(buttonWithText(mounted!.container, "Cancel casting request"));
    expect(mounted?.container.textContent).toContain(
      "You cancelled the casting request to Fire TV.",
    );
    expect(mounted?.container.querySelector("[role=alert]")).toBeNull();
  });
});
