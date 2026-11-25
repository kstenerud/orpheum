// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import {
  apiPrefix,
  createApi,
  type OpenTogetherSocket,
  type TogetherEndReason,
} from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { receiverFormats } from "./device.ts";
import { GuestPart } from "./guest-part.ts";

/** The session's welcome of the Chromecast, device 12 of the guest's membership, a follower. */
const welcome = {
  v: 3,
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 12,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video", isLeader: true }] },
    { id: 2, name: "Gran", devices: [{ id: 12, role: "video" }] },
  ],
};

/** The group at film fileId, as its session tells each follower. */
function groupAt(fileId: number, seq: number): string {
  const group = { state: "playing", positionMs: 5000, rate: 1, fileId, event: "leaderPlays" };
  return JSON.stringify({ v: 3, type: "group", seq, group });
}

/** A Chromecast in a guest's part: what it asked, noted and was told, and its session's socket. */
function guestsPart(): {
  readonly part: GuestPart;
  readonly asked: string[];
  readonly notes: string[];
  readonly welcomed: number[];
  readonly ended: TogetherEndReason[];
  readonly socket: () => Parameters<OpenTogetherSocket>[0] | undefined;
  readonly isClosed: () => boolean;
} {
  const asked: string[] = [];
  const notes: string[] = [];
  const welcomed: number[] = [];
  const ended: TogetherEndReason[] = [];
  const sockets: Parameters<OpenTogetherSocket>[0][] = [];
  let isClosed = false;
  const api = createApi({
    origin: "https://core.example",
    getToken: () => undefined,
    fetch: async (request) => {
      const apiPath = new URL(request.url).pathname.slice(apiPrefix.length);
      const signedIn = request.headers.get("Authorization") ?? "signed out";
      asked.push(`${request.method} ${apiPath} ${signedIn} ${await request.text()}`);
      // No film starts: what is tested is the part, not the player.
      return new Response("{}", { status: 503 });
    },
  });
  const part = new GuestPart({
    api,
    credential: "the-guests-membership",
    reached: {
      fetch: () => Promise.reject(new Error("no chunk is read in a test")),
      openSocket: (listeners) => {
        sockets.push(listeners);
        return {
          send: () => undefined,
          close: () => {
            isClosed = true;
          },
        };
      },
      close: () => undefined,
    },
    video: document.createElement("video"),
    note: (line) => notes.push(line),
    onWelcomed: (deviceId) => welcomed.push(deviceId),
    onEnded: (reason) => ended.push(reason),
  });
  return {
    part,
    asked,
    notes,
    welcomed,
    ended,
    socket: () => sockets[0],
    isClosed: () => isClosed,
  };
}

describe("a Chromecast taking a Watch Together guest's part (D518 §1)", () => {
  it("is welcomed once as the guest's device, and asks for each film the group puts on by the membership", async () => {
    const guest = guestsPart();
    guest.part.start();
    guest.socket()?.onOpen();
    expect(guest.welcomed).toEqual([]);
    guest.socket()?.onMessage(JSON.stringify(welcome));
    expect(guest.welcomed).toEqual([12]);
    expect(guest.asked).toEqual([]); // the group plays nothing yet
    guest.socket()?.onMessage(groupAt(7, 2));
    const formats = JSON.stringify({ playableFormats: receiverFormats() });
    await expect
      .poll(() => guest.asked)
      .toEqual([`POST /together/playback Bearer the-guests-membership ${formats}`]);
    await expect.poll(() => guest.notes.join("\n")).toContain("could not start the group's file 7");
    guest.socket()?.onMessage(groupAt(8, 3));
    await expect.poll(() => guest.asked).toHaveLength(2);
    expect(guest.welcomed).toEqual([12]);
    expect(guest.ended).toEqual([]);
  });

  it("ends, its session let go of, as the guest is removed or the session ends (D518 §2)", () => {
    for (const reason of ["ejected", "hostEnded"] as const) {
      const guest = guestsPart();
      guest.part.start();
      guest.socket()?.onOpen();
      guest.socket()?.onMessage(JSON.stringify(welcome));
      guest.socket()?.onMessage(JSON.stringify({ v: 3, type: "ended", reason }));
      expect(guest.ended).toEqual([reason]);
      expect(guest.isClosed()).toBe(true);
    }
  });

  it("lets go of the session at its stop, telling no end and asking nothing more", async () => {
    const guest = guestsPart();
    const stop = guest.part.start();
    guest.socket()?.onOpen();
    guest.socket()?.onMessage(JSON.stringify(welcome));
    stop();
    expect(guest.isClosed()).toBe(true);
    guest.socket()?.onMessage(groupAt(7, 2));
    await Promise.resolve();
    expect(guest.asked).toEqual([]);
    expect(guest.ended).toEqual([]);
  });
});
