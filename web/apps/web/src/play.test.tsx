// SPDX-License-Identifier: MPL-2.0

import { playbackFailures } from "@orpheum/player";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { boostMixIndices, playbackFailureText, PlayView, requestPlayback } from "./play.tsx";
import { servedBy, type Heard } from "./test-dom.tsx";
import { directDecision, unlevelled } from "./test-fixtures.ts";
import { testServices, withServices } from "./test-services.tsx";

describe("playing a file", () => {
  it("asks the server with this browser's playable formats, on the local network", async () => {
    const playableFormats = testServices().playableFormats();
    const heard: Heard[] = [];
    const { api } = servedBy(
      { "POST /playback": [200, { delivery: "direct", url: "/u", notDirectReasons: [] }] },
      heard,
    );
    const decision = await requestPlayback(api, 7, playableFormats);
    expect(heard).toEqual([
      { methodAndPath: "POST /playback", body: { fileId: 7, playableFormats, network: "local" } },
    ]);
    expect(decision.delivery).toBe("direct");
  });

  it("plays the original in a video element and says so", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
      ),
    );
    expect(markup).toContain('src="/api/v1/bytes/media/direct/abc"');
    expect(markup).toContain("playsInline");
    expect(markup).toContain("Playing the original file, as it is.");
    // Levelling sits under the video, on as the person left it, and says what it does (D226).
    expect(markup).toContain("Level loudness");
    expect(markup).toContain("Its loudness is not measured yet, so it plays as it is.");
  });

  it("plays a prepared copy, names every reason, and offers the boost where there is one", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({
            delivery: "hls",
            profile: "remux",
            url: "/api/v1/bytes/media/hls/abc/master.m3u8",
            notDirectReasons: ["container", "audio_codec"],
            isPreparing: true,
            audioRenditions: [
              { audioTrackIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled },
              { audioTrackIndex: 0, language: "en", isDialogueBoosted: true, ...unlevelled },
            ],
          })}
          onDecideAgain={() => undefined}
        />,
      ),
    );
    expect(markup).toContain(
      "because the browser cannot open its container, the browser cannot decode its sound",
    );
    expect(markup).toContain("Preparing — ready in a few seconds.");
    expect(markup).toContain("Dialogue boost");
  });

  it("finds the standard and boosted mixes of the first track", () => {
    const decision = directDecision({
      delivery: "hls",
      url: "/u",
      audioRenditions: [
        { audioTrackIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled },
        { audioTrackIndex: 0, language: "en", isDialogueBoosted: true, ...unlevelled },
        { audioTrackIndex: 1, language: "de", isDialogueBoosted: false, ...unlevelled },
      ],
    });
    expect(boostMixIndices(decision)).toEqual({ standard: 0, boosted: 1 });
    expect(
      boostMixIndices({
        ...decision,
        audioRenditions: [
          { audioTrackIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled },
        ],
      }),
    ).toBe(undefined);
  });

  it("words every playback failure by the player's code, never the browser's text", () => {
    const t = i18n.t.bind(i18n);
    for (const failure of playbackFailures) {
      const text = playbackFailureText(t, failure);
      expect(text).not.toContain("play.failure");
      expect(text).not.toBe("");
    }
    expect(playbackFailureText(t, "unsupported")).toBe("this browser does not play its format");
  });
});
