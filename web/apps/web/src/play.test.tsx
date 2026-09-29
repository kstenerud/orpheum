// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { boostPair, mediaFailure, PlayView, requestPlayback } from "./play.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** A sound not measured yet: played as it is (D226). */
const unlevelled = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };

describe("playing a file", () => {
  it("asks the server with this browser's menu, over the local link", async () => {
    const menu = testServices().menu();
    let sent: unknown;
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      async onRequest({ request }) {
        sent = await request.json();
        return new Response(
          JSON.stringify({ delivery: "direct", url: "/u", notDirectReasons: [] }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      },
    });
    const decision = await requestPlayback(api, 7, menu);
    expect(sent).toEqual({ fileId: 7, playableFormats: menu, network: "local" });
    expect(decision.delivery).toBe("direct");
  });

  it("plays the original in a video element and says so", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={{
            delivery: "direct",
            url: "/api/v1/bytes/media/direct/abc",
            notDirectReasons: [],
            isPreparing: false,
            subtitles: [],
            prefs: { subtitleOffsetMs: 0, isDialogueBoostOn: false, isLevellingOn: true },
            resumePositionMs: 0,
            mediaToken: "t",
            levelling: unlevelled.levelling,
          }}
          decideAgain={() => undefined}
        />,
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
          decision={{
            delivery: "hls",
            profile: "remux",
            url: "/api/v1/bytes/media/hls/abc/master.m3u8",
            notDirectReasons: ["container", "audio_codec"],
            isPreparing: true,
            subtitles: [],
            prefs: { subtitleOffsetMs: 0, isDialogueBoostOn: false, isLevellingOn: true },
            resumePositionMs: 0,
            mediaToken: "t",
            levelling: unlevelled.levelling,
            audio: [
              { sourceAudioIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled },
              { sourceAudioIndex: 0, language: "en", isDialogueBoosted: true, ...unlevelled },
            ],
          }}
          decideAgain={() => undefined}
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
    const decision = {
      delivery: "hls" as const,
      url: "/u",
      notDirectReasons: [],
      isPreparing: false,
      subtitles: [],
      prefs: { subtitleOffsetMs: 0, isDialogueBoostOn: false, isLevellingOn: true },
      resumePositionMs: 0,
      mediaToken: "t",
      levelling: unlevelled.levelling,
      audio: [
        { sourceAudioIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled },
        { sourceAudioIndex: 0, language: "en", isDialogueBoosted: true, ...unlevelled },
        { sourceAudioIndex: 1, language: "de", isDialogueBoosted: false, ...unlevelled },
      ],
    };
    expect(boostPair(decision)).toEqual({ standard: 0, boosted: 1 });
    expect(
      boostPair({
        ...decision,
        audio: [{ sourceAudioIndex: 0, language: "en", isDialogueBoosted: false, ...unlevelled }],
      }),
    ).toBe(undefined);
  });

  it("words a media element's failure in the browser's own terms, or its code", () => {
    const failed = (code: number, message: string) => ({ code, message });
    expect(mediaFailure(failed(4, "No video with supported format and MIME type found"))).toBe(
      "No video with supported format and MIME type found",
    );
    expect(mediaFailure(failed(3, ""))).toBe("MediaError 3");
    expect(mediaFailure(null)).toBe("unknown");
  });
});
