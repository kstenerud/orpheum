// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { isNotedSending, noteSending } from "./cast.ts";
import { CastToDevice, PlayOrSendOn } from "./cast-to.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
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

/** What the page's live connection says the receiver is handed, as a child of the provider. */
function Receiving(): JSX.Element {
  const { receiving } = useLive();
  return <p>{receiving === undefined ? "" : `receiving cast ${String(receiving.castId)}`}</p>;
}

/** The file the page's live connection says the receiver plays, as a child of the provider. */
function ReceivingFile(): JSX.Element {
  const { receiving } = useLive();
  return <p>{receiving === undefined ? "" : `receiving file ${String(receiving.fileId)}`}</p>;
}

describe("Cast to another device (D337 §1, §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("lists the person's other devices open now, and casts to the one chosen from where the film stands", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [
          200,
          {
            devices: [device(1, "Laptop", true), device(2, "Fire TV"), device(3, "Phone, closed")],
          },
        ],
        "POST /me/casts": [201, { castId: 9, isSender: true }],
      },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => 61_000} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    expect(server.heard[0]).toEqual({ type: "live", sessionToken: "token" });
    await server.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    expect(mounted.container.textContent).toContain("Fire TV");
    expect(mounted.container.textContent).not.toContain("Phone, closed");
    expect(buttonWithText(mounted.container, "Laptop")).toBeUndefined();
    await click(buttonWithText(mounted.container, "Fire TV"));
    const cast = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(cast?.body).toEqual({ toDeviceId: 2, fileId: 4, positionMs: 61_000 });
    expect(bodyField(cast!, "toDeviceId")).toBe(2);
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    // A film played here next goes on to the TV (§9 Q3).
    expect(isNotedSending(tabStorage())).toBe(true);
    noteSending(tabStorage(), false);
  });

  it("says none is open when no other device is", async () => {
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [device(1, "Laptop", true), device(2, "Fire TV")] }] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    expect(mounted.container.textContent).toContain(
      "None of your other devices has Orpheum open now",
    );
  });
});

describe("the page's live connection (D337 §2, §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("receives a cast handed to it, opening the film's Play page", async () => {
    const services = servedBy({}, [], "token");
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Receiving />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({
      type: "receive",
      receive: {
        castId: 5,
        membershipCredential: "cast",
        fileId: 7,
        positionMs: 61_000,
        fromDeviceId: 2,
        fromDeviceName: "Pixel",
      },
    });
    expect(mounted.container.textContent).toContain("receiving cast 5");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
  });

  it("plays the next episode when the same cast is handed again with it (plans/up-next.md)", async () => {
    const services = servedBy({}, [], "token");
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <ReceivingFile />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    const handed = {
      castId: 5,
      membershipCredential: "cast",
      fileId: 7,
      fromDeviceId: 2,
      fromDeviceName: "Pixel",
    };
    await server.say({ type: "receive", receive: handed });
    await server.say({
      type: "receive",
      receive: { ...handed, fileId: 8, membershipCredential: "again" },
    });
    expect(mounted.container.textContent).toContain("receiving file 8");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 8 });
  });
});

describe("a film played on a device that casts (§9 Q3)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    noteSending(tabStorage(), false);
  });

  it("goes on to the receiver, the device staying its remote", async () => {
    noteSending(tabStorage(), true);
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [{ castId: 3, isSender: true, receiverDeviceId: 2 }] }],
        "POST /me/casts": [201, { castId: 4, isSender: true }],
      },
      heard,
      "token",
    );
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        services,
      ),
    );
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toEqual({
      toDeviceId: 2,
      fileId: 8,
    });
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    expect(mounted.container.textContent).not.toContain("playing here");
  });

  it("plays here once the server says the device casts no more, and asks nothing where it never cast", async () => {
    noteSending(tabStorage(), true);
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        servedBy({ "GET /me/casts": [200, { casts: [] }] }, heard, "token"),
      ),
    );
    expect(mounted.container.textContent).toContain("playing here");
    expect(isNotedSending(tabStorage())).toBe(false);
    mounted.unmount();
    heard.length = 0;
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        servedBy({}, heard, "token"),
      ),
    );
    expect(mounted.container.textContent).toContain("playing here");
    expect(heard).toEqual([]);
  });
});
