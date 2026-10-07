// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { PlaybackDecision } from "./watching.ts";
import { Play } from "./play.tsx";
import { BoostSwitch } from "./sound.tsx";
import { choose, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
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
    services.session.signIn("token", 5);
    mounted = await mount(withServices(<Play fileId={2} />, services));
    expect(mounted.container.textContent).toContain("How does this device play sound?");
    expect(mounted.container.querySelector("video")).toBeNull();
    // A receiver's copy prepared for its container or codec is stereo, so the boost works there
    // too, and the question must not promise otherwise (D222 §2).
    expect(answerButton("Soundbar or receiver")?.textContent).toContain(
      "a copy prepared for another reason is stereo, and the dialogue boost works on it",
    );
    await click(answerButton("Built-in speakers"));
    expect(asked(heard)).toEqual(["POST /playback", "PUT /me/devices/5", "POST /playback"]);
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
    services.session.signIn("token", 5);
    mounted = await mount(withServices(<Play fileId={2} />, services));
    await click(answerButton("Change"));
    expect(answerButton("Built-in speakers")?.getAttribute("aria-pressed")).toBe("true");
    await click(answerButton("Headphones"));
    expect(heard[1]?.body).toEqual({ audioOutput: "headphones" });
    expect(asked(heard).filter((each) => each === "POST /playback")).toHaveLength(2);
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
          devices={{ state: "loaded", value: [livingRoom] }}
          onChanged={() => {
            reloads += 1;
          }}
          onMakeCode={() => undefined}
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
