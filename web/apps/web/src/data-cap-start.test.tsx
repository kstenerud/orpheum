// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { DataCaps, dataCapsKey, deviceDataCaps } from "./data-cap.ts";
import { Play } from "./play.tsx";
import { mount, playbackCauses, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { FakeConnection, heldStorage } from "./test-data-cap.ts";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  deviceDataCaps().choose(undefined);
  window.localStorage.removeItem(dataCapsKey);
});

describe("a cap starting to hold (D489)", () => {
  it("is told once each time the cap falls under the file's rate, by a change of network or a choice (D491)", () => {
    const { storage } = heldStorage();
    const connection = new FakeConnection("wifi");
    const caps = new DataCaps(storage, connection);
    caps.choose(undefined);
    connection.moveTo("cellular");
    caps.choose(796);
    let starts = 0;
    const stop = caps.whenCapFallsUnder(2400, () => {
      starts += 1;
    });
    caps.choose(496);
    expect(starts).toBe(0); // under it before, under it still
    connection.moveTo("wifi");
    expect(starts).toBe(0); // Wi-Fi has No limit
    connection.moveTo("cellular");
    expect(starts).toBe(1); // mobile data's cap holds again
    caps.choose(undefined);
    caps.choose(796);
    expect(starts).toBe(2);
    caps.choose(3096);
    expect(starts).toBe(2); // a cap at or above the file's rate lets its own file play
    caps.choose(2400);
    expect(starts).toBe(2);
    caps.choose(2096);
    expect(starts).toBe(3); // lowered under it
    stop();
    caps.choose(undefined);
    caps.choose(496);
    expect(starts).toBe(3);
  });

  it("is told of any cap where the file's rate is not known", () => {
    const caps = new DataCaps(heldStorage().storage, undefined);
    let starts = 0;
    const stop = caps.whenCapFallsUnder(0, () => {
      starts += 1;
    });
    caps.choose(6096);
    expect(starts).toBe(1);
    stop();
  });

  it("moves a film playing its own file to its ladder, asking again from where it stands", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, directDecision()],
        "GET /files/41/waits": [200, {}],
        "GET /files/41/up-next": [200, {}],
        "GET /files/41/availability": [200, {}],
        "POST /playback/t/report": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={41} />, services));
    expect(playbackCauses(heard)).toEqual(["playPage"]);
    await act(async () => {
      deviceDataCaps().choose(796);
      await Promise.resolve();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    // Asked once: the film the core plays as it is again, its ladder not there, is not asked for again.
    expect(playbackCauses(heard)).toEqual(["playPage", "dataCapStarted"]);
    // The core applies the cap it keeps for this kind of network (D493).
    const asked = heard.filter((each) => each.methodAndPath === "POST /playback").at(-1);
    expect(asked?.body).toMatchObject({ networkKind: "unknown" });
  });

  it("leaves a film playing its own file where the cap starting is at or above its rate (D491)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, directDecision()],
        "GET /files/41/waits": [200, {}],
        "GET /files/41/up-next": [200, {}],
        "GET /files/41/availability": [200, {}],
        "POST /playback/t/report": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={41} />, services));
    await act(async () => {
      deviceDataCaps().choose(2400); // the file's own 2400 kb/s
      await Promise.resolve();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(playbackCauses(heard)).toEqual(["playPage"]);
    await act(async () => {
      deviceDataCaps().choose(2096);
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(playbackCauses(heard)).toEqual(["playPage", "dataCapStarted"]);
  });
});
