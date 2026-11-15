// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  connectionOf,
  DataCaps,
  dataCapsKey,
  gbPerHourOf,
  networkKindOf,
  rungUnderCap,
} from "./data-cap.ts";
import { FakeConnection, heldStorage } from "./test-data-cap.ts";

/** The core's default ladder on a node encoding on a GPU, each rung with its 96 kb/s sound. */
const defaultRungsKbps = [496, 796, 1296, 2096, 3096, 6096];

describe("the kind of network a device is on (D488)", () => {
  it("is what navigator.connection's type says, never guessed where it says nothing", () => {
    expect(networkKindOf(new FakeConnection("cellular"))).toBe("mobile");
    expect(networkKindOf(new FakeConnection("wifi"))).toBe("wifi");
    expect(networkKindOf(new FakeConnection("ethernet"))).toBe("wifi");
    expect(networkKindOf(new FakeConnection("bluetooth"))).toBe("unknown");
    expect(networkKindOf(new FakeConnection(undefined))).toBe("unknown");
    expect(networkKindOf(undefined)).toBe("unknown");
  });

  it("is read from a navigator only where it has a connection to read", () => {
    const connection = new FakeConnection("wifi");
    expect(connectionOf({ connection })).toBe(connection);
    expect(connectionOf({})).toBeUndefined();
    expect(connectionOf({ connection: { type: "wifi" } })).toBeUndefined();
  });
});

describe("a device's data caps (D488)", () => {
  it("start at No limit for every kind of network (the owner, 2026-11-14)", () => {
    const { storage } = heldStorage();
    for (const type of ["wifi", "cellular", undefined]) {
      expect(new DataCaps(storage, new FakeConnection(type)).capKbps()).toBeUndefined();
    }
    expect(new DataCaps(storage, undefined).capKbps()).toBeUndefined();
  });

  it("keep one cap for Wi-Fi and one for mobile data, and play by the one for the network the device moves to", () => {
    const { storage, held } = heldStorage();
    const connection = new FakeConnection("cellular");
    const caps = new DataCaps(storage, connection);
    let told = 0;
    const stop = caps.subscribe(() => {
      told += 1;
    });
    caps.choose(496);
    expect(caps.capKbps()).toBe(496);
    connection.moveTo("wifi");
    expect(caps.kind()).toBe("wifi");
    expect(caps.capKbps()).toBeUndefined();
    caps.choose(3096);
    connection.moveTo("cellular");
    expect(caps.capKbps()).toBe(496);
    expect(told).toBe(4);
    expect(JSON.parse(held.get(dataCapsKey) ?? "")).toEqual({ mobile: 496, wifi: 3096 });
    stop();
    connection.moveTo("wifi");
    expect(told).toBe(4);
  });

  it("keep a cap of their own where the browser says nothing of the network, and lift one by No limit", () => {
    const { storage } = heldStorage();
    const caps = new DataCaps(storage, undefined);
    caps.choose(1296);
    expect(new DataCaps(storage, undefined).capKbps()).toBe(1296);
    expect(new DataCaps(storage, new FakeConnection("wifi")).capKbps()).toBeUndefined();
    caps.choose(undefined);
    expect(caps.capKbps()).toBeUndefined();
  });

  it("read anything kept that is not a rate as none", () => {
    const { storage, held } = heldStorage();
    const caps = new DataCaps(storage, undefined);
    for (const kept of ["{", "7", "null", '{"unknown":-5}', '{"unknown":"1296"}']) {
      held.set(dataCapsKey, kept);
      expect(caps.capKbps()).toBeUndefined();
    }
  });
});

describe("a cap's figures (D488)", () => {
  it("are each rung's video and sound in GB per hour, to one decimal", () => {
    expect(defaultRungsKbps.map(gbPerHourOf)).toEqual([0.2, 0.4, 0.6, 0.9, 1.4, 2.7]);
  });

  it("play, for a film, the highest rung within the cap, or its lowest where none is", () => {
    expect(rungUnderCap(defaultRungsKbps, undefined)).toBeUndefined();
    expect(rungUnderCap(defaultRungsKbps, 1296)).toBe(1296);
    expect(rungUnderCap(defaultRungsKbps, 2000)).toBe(1296);
    expect(rungUnderCap([2096, 3096], 1296)).toBe(2096);
  });
});
