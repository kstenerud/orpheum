// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { playerSwitchedQualityEvent, type PlaybackReport } from "@orpheum/sdk";
import { act, useRef, type JSX } from "react";
import { afterEach, expect, it } from "vitest";

import { HowItPlays, usePlaybackHere } from "./how-it-plays.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;
afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A video given delivery, and How it plays from its own playback (usePlaybackHere). */
function PlaysHere(props: { readonly delivery: PlaybackReport["delivery"] }): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  const playback = usePlaybackHere(videoRef, props.delivery);
  return (
    <>
      <video ref={videoRef}>
        <track kind="captions" />
      </video>
      <ul>
        <HowItPlays playback={playback} />
      </ul>
    </>
  );
}

it("says how the film plays here from the local player's own playback: its delivery, then the quality and link it began (D444)", async () => {
  mounted = await mount(<PlaysHere delivery="converted" />);
  const text = (): string | null | undefined =>
    mounted?.container.querySelector(".how-it-plays")?.textContent;
  expect(text()).toBe("Playing: Converted");
  // With no link said, one fact alone, no " · " and one icon.
  expect(mounted.container.querySelectorAll(".how-it-plays__fact")).toHaveLength(1);
  expect(mounted.container.querySelectorAll("svg.how-it-plays__icon")).toHaveLength(1);
  await act(async () => {
    mounted?.container.querySelector("video")?.dispatchEvent(
      new CustomEvent(playerSwitchedQualityEvent, {
        detail: { qualityKbps: 4200, linkEstimateKbps: 18_000 },
      }),
    );
    await Promise.resolve();
  });
  expect(text()).toBe("Playing: Converted 4.2 Mb/s · Connection: 18.0 Mb/s");
});

// The menu spec of D538, "▶ Converted 12.1 Mb/s · 📶 69.2 Mb/s", and the owner, 2026-11-23: "the
// text about how it's playing was supposed to use shortened text and icons".
it("marks each fact by its icon, its word said to a screen reader alone, on one line of ⁝", async () => {
  mounted = await mount(
    <ul>
      <HowItPlays playback={{ delivery: "direct", qualityKbps: 4200, linkKbps: 18_000 }} />
    </ul>,
  );
  const note = mounted.container.querySelector("li");
  expect(note?.className).toBe("list-box__note list-box__note--one-line");
  const facts = [...mounted.container.querySelectorAll(".how-it-plays__fact")];
  expect(facts).toHaveLength(2);
  for (const fact of facts) {
    expect(fact.firstElementChild?.matches("svg.how-it-plays__icon")).toBe(true);
  }
  expect(
    [...mounted.container.querySelectorAll(".how-it-plays__said")].map((each) => each.textContent),
  ).toEqual(["Playing: ", "Connection: "]);
  expect(mounted.container.querySelector(".how-it-plays")?.textContent).toBe(
    "Playing: Original 4.2 Mb/s · Connection: 18.0 Mb/s",
  );
});

it("says nothing where nothing is said of how the film plays", async () => {
  mounted = await mount(
    <ul>
      <HowItPlays playback={undefined} />
    </ul>,
  );
  expect(mounted.container.querySelector(".how-it-plays")).toBeNull();
});
