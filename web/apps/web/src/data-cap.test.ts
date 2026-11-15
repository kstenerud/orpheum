// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  connectionOf,
  DataCaps,
  dataCapsKey,
  gbPerHourOf,
  isWithinCap,
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

describe("a device's data caps, the core's with a copy in its browser (D493)", () => {
  it("take the core's into the copy at each read of who is signed in, telling who listens", () => {
    const { storage, held } = heldStorage();
    const caps = new DataCaps(storage, new FakeConnection("cellular"));
    caps.choose(796);
    let told = 0;
    caps.subscribe(() => {
      told += 1;
    });
    const sent: unknown[] = [];
    caps.settleWithCore({ mobileKbps: 496, wifiKbps: 3096 }, (kept) => {
      sent.push(kept);
      return Promise.resolve();
    });
    expect(caps.capKbps()).toBe(496);
    expect(JSON.parse(held.get(dataCapsKey) ?? "{}")).toEqual({ wifi: 3096, mobile: 496 });
    expect(told).toBe(1);
    expect(sent).toEqual([]);
    caps.settleWithCore({ mobileKbps: 496, wifiKbps: 3096 }, (kept) => {
      sent.push(kept);
      return Promise.resolve();
    });
    expect(told).toBe(1); // the same read again changes nothing
  });

  it("keep the latest choice on the core too, one write at a time, once the core is known", async () => {
    const caps = new DataCaps(heldStorage().storage, new FakeConnection("wifi"));
    caps.choose(6096); // before the core is known: kept here alone
    const sent: unknown[] = [];
    caps.settleWithCore({ wifiKbps: 2096 }, async (kept) => {
      await Promise.resolve();
      sent.push(kept);
    });
    caps.choose(1296);
    await caps.written();
    expect(sent).toEqual([{ wifiKbps: 1296 }]);
    caps.choose(796);
    caps.choose(undefined);
    await caps.written();
    // The latest asked for, as the sound offset's latestWrite keeps (D493, rule 11).
    expect(sent).toEqual([{ wifiKbps: 1296 }, {}]);
  });

  it("carry the browser's over once where the core keeps none, and keep a failed write's choice here", async () => {
    const caps = new DataCaps(heldStorage().storage, undefined);
    caps.choose(796);
    const sent: unknown[] = [];
    caps.settleWithCore({}, (kept) => {
      sent.push(kept);
      return Promise.reject(new Error("the core is away"));
    });
    await caps.written(); // the failure is reported, never thrown
    expect(sent).toEqual([{ unknownKbps: 796 }]);
    expect(caps.capKbps()).toBe(796);
    caps.settleWithCore({ unknownKbps: 796 }, (kept) => {
      sent.push(kept);
      return Promise.resolve();
    });
    await caps.written();
    expect(sent).toHaveLength(1); // the core's are the truth now
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

describe("a film's own file under a cap (D491)", () => {
  it("plays with no cap, or one at or above its rate, never one below or where its rate is not known", () => {
    expect(isWithinCap(2400, undefined)).toBe(true);
    expect(isWithinCap(0, undefined)).toBe(true);
    expect(isWithinCap(2400, 2400)).toBe(true);
    expect(isWithinCap(2400, 3096)).toBe(true);
    expect(isWithinCap(2400, 2399)).toBe(false);
    expect(isWithinCap(0, 6096)).toBe(false);
  });
});
