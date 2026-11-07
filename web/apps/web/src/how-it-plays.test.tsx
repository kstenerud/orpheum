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
  expect(text()).toBe("Plays it converted");
  await act(async () => {
    mounted?.container.querySelector("video")?.dispatchEvent(
      new CustomEvent(playerSwitchedQualityEvent, {
        detail: { qualityKbps: 4200, linkEstimateKbps: 18_000 },
      }),
    );
    await Promise.resolve();
  });
  expect(text()).toBe("Plays it converted · Quality 4.2 Mb/s · Link about 18.0 Mb/s");
});

it("says nothing where nothing is said of how the film plays", async () => {
  mounted = await mount(
    <ul>
      <HowItPlays playback={undefined} />
    </ul>,
  );
  expect(mounted.container.querySelector(".how-it-plays")).toBeNull();
});
