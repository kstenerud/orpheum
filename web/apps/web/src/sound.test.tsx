// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { PlaybackDecision } from "./watching.ts";
import { Play, PlayView } from "./play.tsx";
import { BoostSwitch } from "./sound.tsx";
import {
  choose,
  click,
  mount,
  playbackCauses,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision, television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { YourDevices } from "./your-devices.tsx";

/** A decision to play the original, with how the device makes sound when it has been asked. */
function direct(audioOutput?: PlaybackDecision["audioOutput"]): PlaybackDecision {
  return directDecision(audioOutput === undefined ? {} : { audioOutput });
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The button whose text starts with text: an answer is its name and what it does. */
function answerButton(text: string): HTMLButtonElement | undefined {
  return [...(mounted?.container.querySelectorAll("button") ?? [])].find((each) =>
    each.textContent.startsWith(text),
  );
}

/** What was asked of the server, in order, without the bodies. */
function asked(heard: readonly Heard[]): string[] {
  return heard.map((each) => each.methodAndPath);
}

describe("how a device makes sound (D222)", () => {
  it("is asked before the device's first playback, and the decision is asked again with the answer", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [
          [200, direct()],
          [200, direct("built_in_speakers")],
        ],
        "PUT /me/devices/5": [204, undefined],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    expect(mounted.container.textContent).toContain("How does this device play sound?");
    expect(mounted.container.querySelector("video")).toBeNull();
    // A receiver's copy prepared for its container or codec is stereo, so the boost works there
    // too, and the question must not promise otherwise (D222 §2).
    expect(answerButton("Soundbar or receiver")?.textContent).toContain(
      "a copy prepared for another reason is stereo, and the dialogue boost works on it",
    );
    await click(answerButton("Built-in speakers"));
    // The film is not measured yet, so the page asks how long its measuring will take (D324 §6).
    expect(asked(heard)).toEqual([
      "POST /playback",
      "PUT /me/devices/5",
      "POST /playback",
      "GET /files/2/waits",
    ]);
    expect(playbackCauses(heard)).toEqual(["playPage", "soundAnswered"]);
    expect(heard[1]?.body).toEqual({ audioOutput: "built_in_speakers" });
    expect(mounted.container.querySelector("video")).not.toBeNull();
    expect(mounted.container.textContent).toContain("Sound: Built-in speakers");
  });

  it("is not asked of a session with no device to keep it on", async () => {
    const services = servedBy({ "POST /playback": [200, direct()] });
    services.session.signIn("token");
    mounted = await mount(withServices(<Play fileId={2} />, services));
    expect(mounted.container.querySelector("video")).not.toBeNull();
    expect(mounted.container.textContent).not.toContain("How does this device play sound?");
  });

  it("can be changed under the video, which asks for the decision again", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [
          [200, direct("built_in_speakers")],
          [200, direct("headphones")],
        ],
        "PUT /me/devices/5": [204, undefined],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    await click(answerButton("Change"));
    expect(answerButton("Built-in speakers")?.getAttribute("aria-pressed")).toBe("true");
    await click(answerButton("Headphones"));
    const told = heard.find((each) => each.methodAndPath === "PUT /me/devices/5");
    expect(told?.body).toEqual({ audioOutput: "headphones" });
    expect(playbackCauses(heard)).toEqual(["playPage", "soundChanged"]);
    expect(mounted.container.textContent).toContain("Sound: Headphones");
  });

  it("is chosen for each of one's devices in Your devices", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/devices/7": [204, undefined] }, heard);
    const livingRoom = television();
    let reloads = 0;
    mounted = await mount(
      withServices(
        <YourDevices
          canManage
          devices={{ state: "loaded", value: [livingRoom] }}
          onChanged={() => {
            reloads += 1;
          }}
        />,
        services,
      ),
    );
    const select = mounted.container.querySelector("select");
    expect(select?.value).toBe("");
    expect(select?.textContent).toContain("Not asked yet");
    await choose(select, "receiver");
    expect(heard).toEqual([
      { methodAndPath: "PUT /me/devices/7", body: { audioOutput: "receiver" } },
    ]);
    expect(reloads).toBe(1);
  });
});

describe("the dialogue boost's switch", () => {
  it("starts as the person left it, and switching it changes the mix and is kept for every file", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, heard);
    const switched: boolean[] = [];
    mounted = await mount(
      withServices(
        <BoostSwitch
          isInitiallyOn
          onSwitched={(isOn) => {
            switched.push(isOn);
          }}
        />,
        services,
      ),
    );
    const box = mounted.container.querySelector("input");
    expect(box?.checked).toBe(true);
    await click(box);
    expect(box?.checked).toBe(false);
    expect(switched).toEqual([false]);
    expect(heard).toEqual([
      { methodAndPath: "PUT /me/preferences", body: { isDialogueBoostOn: false } },
    ]);
  });
});

describe("the dialogue boost on a film's ladder", () => {
  /** A stereo device's film on its ladder: its surround track's standard mix and boosted one. */
  function onLadder(isDialogueBoostOn: boolean): PlaybackDecision {
    const converted = { codec: "aac", channelCount: 2, kbps: 128, isOriginal: false };
    const plain = directDecision();
    return directDecision({
      onLadder: {
        url: "/api/v1/bytes/media/chunks/t/master.m3u8",
        startAtBps: 1_896_000,
        qualityKbps: 1200,
        height: 480,
        isBelowTheLowest: false,
        soundCarriedGainDb: 0,
        isReady: false,
        filmPictureKbps: 4000,
        linkSharePercent: 70,
      },
      preferences: { ...plain.preferences, isDialogueBoostOn },
      tracks: {
        ...plain.tracks,
        pictures: [{ codec: "h264", width: 854, height: 480, kbps: 1200, isOriginal: false }],
        sounds: [
          { audioTrackIndex: 0, isBoosted: false, ...converted },
          { audioTrackIndex: 0, isBoosted: true, ...converted },
        ],
        fileSounds: [{ audioTrackIndex: 0, codec: "eac3", channelCount: 6 }],
      },
    });
  }

  it("is switched under the film as under a prepared copy, playing the other mix and kept for every file (the owner, 2026-10-24)", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, heard);
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={onLadder(false)} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    const box = [...mounted.container.querySelectorAll("label")]
      .find((label) => label.textContent === "Dialogue boost")
      ?.querySelector("input");
    expect(box?.checked).toBe(false);
    expect(mounted.container.textContent).toContain(
      "Audio: AAC stereo, 128 kb/s (transcoded from E-AC-3 5.1)",
    );
    await click(box);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isDialogueBoostOn: true },
    });
    expect(mounted.container.textContent).toContain(
      "Audio: AAC stereo, 128 kb/s (transcoded from E-AC-3 5.1, dialogue boost)",
    );
  });

  it("starts as the person left it, and is not offered for a track with no centre to raise", async () => {
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={onLadder(true)} onDecideAgain={() => undefined} />,
        servedBy({}),
      ),
    );
    expect(mounted.container.textContent).toContain("dialogue boost)");
    const stereo = onLadder(false);
    mounted.unmount();
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={{
            ...stereo,
            tracks: { ...stereo.tracks, sounds: stereo.tracks.sounds?.slice(0, 1) ?? [] },
          }}
          onDecideAgain={() => undefined}
        />,
        servedBy({}),
      ),
    );
    expect(mounted.container.textContent).not.toContain("Dialogue boost");
  });
});
