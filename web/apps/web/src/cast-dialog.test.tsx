// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastDialog } from "./cast-dialog.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import { CastingLeft } from "./old-addresses.tsx";
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
import { fakeTogetherServer } from "./test-together.ts";

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 0,
  isAccepted: true,
};

describe("Cast's dialog (D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The dialog over film 4's Play page, its live connection open; what it was told of closing. */
  async function opened(): Promise<{
    readonly live: ReturnType<typeof fakeTogetherServer>;
    readonly onClose: ReturnType<typeof vi.fn>;
  }> {
    const live = fakeTogetherServer();
    const services = servedBy({ "GET /me/devices": [200, { devices: [] }] }, [], "token");
    services.navigation.go({ screen: "play", fileId: 4 });
    const onClose = vi.fn();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastDialog onClose={onClose} />
        </LiveProvider>,
        { ...services, openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    return { live, onClose };
  }

  it("closes once a cast started from it is taken, the page under it its remote", async () => {
    const { live, onClose } = await opened();
    await live.say({ type: "sending" });
    await live.say({ type: "sending", sending: { ...sending, isAccepted: false } });
    expect(onClose).not.toHaveBeenCalled();
    await live.say({ type: "sending", sending });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("stays open over a cast on as it opened, saying what plays where", async () => {
    const live = fakeTogetherServer();
    const onClose = vi.fn();
    /** The dialog, opened once the live connection says a cast is on. */
    function OpenedWhileCasting(): JSX.Element | null {
      return useLive().sending === undefined ? null : <CastDialog onClose={onClose} />;
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <OpenedWhileCasting />
        </LiveProvider>,
        { ...servedBy({}, [], "token"), openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    await live.say({ type: "sending", sending: { ...sending, fileId: 5 } });
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector("dialog")?.textContent).toContain(
      "Playing Metropolis on Fire TV",
    );
  });

  /** The dialog opened over a cast on, the end request answered as given; what it was told of closing. */
  async function openedWhileCasting(
    end: Answers[string],
    heard: Heard[],
  ): Promise<ReturnType<typeof vi.fn>> {
    const live = fakeTogetherServer();
    const onClose = vi.fn();
    /** The dialog, opened once the live connection says a cast is on. */
    function OpenedWhileCasting(): JSX.Element | null {
      return useLive().sending === undefined ? null : <CastDialog onClose={onClose} />;
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <OpenedWhileCasting />
        </LiveProvider>,
        {
          ...servedBy({ "POST /me/casts/9/end": end }, heard, "token"),
          openTogetherSocket: live.openSocket,
        },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    return onClose;
  }

  // Principles §28: its Stop hid a failure, the cast going on while the person took it for stopped.
  it("stops casting, tried once more where the first try fails, and closes", async () => {
    const heard: Heard[] = [];
    const onClose = await openedWhileCasting(
      [refusal(503, "unavailable"), [200, { positionMs: 0, isPlaying: false }]],
      heard,
    );
    await click(buttonWithText(document.body, "Stop casting"));
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toHaveLength(2);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("says why Stop casting failed where both tries do, and stays open to try again", async () => {
    const onClose = await openedWhileCasting(
      [refusal(503, "unavailable"), refusal(503, "unavailable")],
      [],
    );
    await click(buttonWithText(document.body, "Stop casting"));
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector("dialog [role=alert]")).not.toBeNull();
  });
});

describe("an old address of the casting page (D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** /casting, as a history entry holds it, once the live connection says what this device sends. */
  async function leftAt(
    said: Readonly<Record<string, unknown>>,
  ): Promise<ReturnType<typeof servedBy>> {
    const live = fakeTogetherServer();
    const services = servedBy({}, [], "token");
    services.navigation.go({ screen: "casting" });
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastingLeft />
        </LiveProvider>,
        { ...services, openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    await live.say(said);
    return services;
  }

  it("goes to the Play page of the film the cast plays, its remote, in its place", async () => {
    const services = await leftAt({ type: "sending", sending });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4 });
    expect(services.navigation.behind()).toEqual([{ screen: "films" }]);
  });

  it("goes home where this device casts no film", async () => {
    const services = await leftAt({ type: "sending" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });
});
