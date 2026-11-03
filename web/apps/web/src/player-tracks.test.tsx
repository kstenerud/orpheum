// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerTracksProvider, usePlayerTracks } from "./player-tracks.tsx";
import { newSoundPanel, SoundPanelContext } from "./sound-panel.ts";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

/** A sound not measured, which levelling leaves as it is. */
const unlevelled = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };

/** A film with two audio tracks, starting with the first. */
const tracks = [
  { streamId: 1, audioTrackIndex: 0, language: "en", kind: "plain" as const },
  { streamId: 2, audioTrackIndex: 1, language: "fr", kind: "plain" as const },
];

/** What a piece over the film reads of the tracks: the audio track chosen, as text. */
function ChosenAudio(): JSX.Element {
  return <output>{String(usePlayerTracks().audio.chosenStreamId)}</output>;
}

describe("the film's track choices, held around the stage (player-tracks.tsx)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
  });

  // A drop-down draws its menu only while open, and a cast's receiver draws none, so the remote's
  // sound controls must not depend on a menu or switch being drawn (plans/player-overlay.md §6).
  it("shows and works a cast's remote's sound with no menu or switch drawn", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "PUT /me/preferences": [204, undefined],
        "PUT /files/4/preferences": [204, undefined],
        // The sound is not measured, so the provider asks how long measuring it will take.
        "GET /files/4/waits": [200, {}],
      },
      heard,
    );
    const panel = newSoundPanel();
    const switched = vi.fn();
    const chosen = vi.fn();
    mounted = await mount(
      withServices(
        <SoundPanelContext value={panel}>
          <PlayerTracksProvider
            fileId={4}
            decision={directDecision({ audioTracks: tracks, startingAudioStreamId: 1 })}
            videoRef={createRef()}
            level={unlevelled}
            onAudioChosen={chosen}
            boost={{ isInitiallyOn: false, onSwitched: switched, isOffered: true }}
          >
            <ChosenAudio />
          </PlayerTracksProvider>
        </SoundPanelContext>,
        services,
      ),
    );
    expect(mounted.container.querySelector("select, input")).toBeNull();
    expect(panel.read()).toEqual({
      isBoostOn: false,
      isLevellingOn: true,
      audioStreamId: 1,
      audioTracks: [
        { streamId: 1, language: "en", kind: "plain" },
        { streamId: 2, language: "fr", kind: "plain" },
      ],
    });

    // Each choice the remote sends comes as its own message, the page drawn again between them.
    for (const choice of [
      { kind: "boost", isOn: true },
      { kind: "audioTrack", streamId: 2 },
      { kind: "levelling", isOn: false },
    ] as const) {
      await act(async () => {
        panel.choose(choice);
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }
    expect(switched).toHaveBeenCalledWith(true);
    // The track chosen plays with the boost as it then stands.
    expect(chosen).toHaveBeenCalledWith(tracks[1], true);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isDialogueBoostOn: true },
    });
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isLevellingOn: false },
    });
    expect(heard).toContainEqual({
      methodAndPath: "PUT /files/4/preferences",
      body: { audioChoice: { streamId: 2 } },
    });
    expect(panel.read()).toMatchObject({ isBoostOn: true, isLevellingOn: false, audioStreamId: 2 });
    // A piece over the film sees the choice the remote made.
    expect(mounted.container.querySelector("output")?.textContent).toBe("2");
  });

  it("shows the remote no boost where the sound offers none", async () => {
    const panel = newSoundPanel();
    mounted = await mount(
      withServices(
        <SoundPanelContext value={panel}>
          <PlayerTracksProvider
            fileId={4}
            decision={directDecision()}
            videoRef={createRef()}
            level={unlevelled}
            onAudioChosen={vi.fn()}
            boost={{ isInitiallyOn: true, onSwitched: vi.fn(), isOffered: false }}
          >
            {null}
          </PlayerTracksProvider>
        </SoundPanelContext>,
        servedBy({ "GET /files/4/waits": [200, {}] }),
      ),
    );
    expect(panel.read()?.isBoostOn).toBeUndefined();
  });
});
