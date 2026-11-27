// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastTargets, reconnectingSaidForMs } from "./cast-to.tsx";
import { LiveProvider } from "./live.tsx";
import {
  buttonWithText,
  mount,
  servedBy,
  settleAnswers,
  unreachable,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

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

describe("the device list as the live connection says what is open", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("offers no device, saying it reconnects, while the live connection is down (stale lists)", async () => {
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [device(1, "Phone", true), device(3, "Fire TV")] }] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 3], inFrontDeviceIds: [1, 3] });
    expect(buttonWithText(mounted.container, "Fire TV")?.matches(":disabled")).toBe(false);
    expect(mounted.container.textContent).not.toContain("Reconnecting to the server…");
    await server.drop();
    expect(buttonWithText(mounted.container, "Fire TV")?.matches(":disabled")).toBe(true);
    expect(mounted.container.textContent).toContain("Reconnecting to the server…");
  });

  it("lists a device signed in while the list is open, as it connects (the owner, staging, 2026-11-07)", async () => {
    const routes: Record<string, Parameters<typeof servedBy>[0][string]> = {
      "GET /me/devices": [200, { devices: [device(1, "Mac", true)] }],
    };
    const services = servedBy(routes, [], "token");
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    expect(mounted.container.textContent).toContain("None of your other devices");
    routes["GET /me/devices"] = [200, { devices: [device(1, "Mac", true), device(2, "Phone")] }];
    await server.say({ type: "devices", deviceIds: [1, 2], inFrontDeviceIds: [2] });
    expect(mounted.container.textContent).toContain("Phone");
  });
});

describe("Cast's menu while the page makes new connections (D564 §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  let showMenu: (isShown: boolean) => void = () => undefined;
  /** Cast's menu, closed and opened again within the one live connection, as a person does. */
  function Reopened(): JSX.Element | null {
    const [isShown, setIsShown] = useState(true);
    showMenu = setIsShown;
    return isShown ? <CastTargets film={() => undefined} /> : null;
  }

  async function shown(isShown: boolean): Promise<void> {
    await act(async () => {
      showMenu(isShown);
      await settleAnswers();
    });
  }

  /**
   * The menu, its list read and the TV offered, then the live connection lost and the menu opened
   * again, its read of the devices failing: the live connection names a device (5) the list does
   * not, so the menu reads the list again (D480 §3).
   */
  async function reopenedWhileReconnecting(): Promise<{
    readonly routes: Record<string, Parameters<typeof servedBy>[0][string]>;
    readonly server: ReturnType<typeof fakeTogetherServer>;
    readonly pathBack: () => Promise<void>;
  }> {
    const routes: Record<string, Parameters<typeof servedBy>[0][string]> = {
      "GET /me/devices": [200, { devices: [device(1, "Phone", true), device(3, "Fire TV")] }],
    };
    const server = fakeTogetherServer();
    const listeners: (() => void)[] = [];
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Reopened />
        </LiveProvider>,
        {
          ...servedBy(routes, [], "token"),
          openTogetherSocket: server.openSocket,
          onPathBack: (listener) => {
            listeners.push(listener);
            return () => undefined;
          },
        },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 3, 5], inFrontDeviceIds: [1, 3, 5] });
    expect(buttonWithText(mounted.container, "Fire TV")?.matches(":disabled")).toBe(false);
    await shown(false);
    await server.drop();
    routes["GET /me/devices"] = unreachable;
    await shown(true);
    const pathBack = async (): Promise<void> => {
      await act(async () => {
        for (const listener of listeners) {
          listener();
        }
        await settleAnswers();
      });
    };
    return { routes, server, pathBack };
  }

  it("says it reconnects, keeping the devices it listed, offered once it has connected again", async () => {
    const { routes, server, pathBack } = await reopenedWhileReconnecting();
    const text = (): string => mounted?.container.textContent ?? "";
    expect(text()).toContain("Reconnecting to the server…");
    expect(text()).not.toContain("The server could not be reached");
    expect(
      buttonWithText(mounted?.container ?? document.body, "Fire TV")?.matches(":disabled"),
    ).toBe(true);
    routes["GET /me/devices"] = [
      200,
      { devices: [device(1, "Phone", true), device(3, "Fire TV"), device(5, "Laptop")] },
    ];
    await pathBack();
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 3, 5], inFrontDeviceIds: [1, 3, 5] });
    expect(text()).not.toContain("Reconnecting to the server…");
    expect(
      buttonWithText(mounted?.container ?? document.body, "Laptop")?.matches(":disabled"),
    ).toBe(false);
  });

  it("says the server could not be reached once it has not connected for thirty seconds", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    await reopenedWhileReconnecting();
    const text = (): string => mounted?.container.textContent ?? "";
    await act(async () => {
      vi.advanceTimersByTime(reconnectingSaidForMs - 1_000);
      await Promise.resolve();
    });
    expect(text()).not.toContain("The server could not be reached");
    await act(async () => {
      vi.advanceTimersByTime(1_000);
      await Promise.resolve();
    });
    expect(text()).toContain("The server could not be reached");
    expect(buttonWithText(mounted?.container ?? document.body, "Fire TV")).toBeUndefined();
  });
});
