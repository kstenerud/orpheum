// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { createRef } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  keepOffset,
  formatOffsetSeconds,
  Subtitles,
  trackLabel,
  type SubtitleTrack,
} from "./subtitles.tsx";
import { withServices } from "./test-services.tsx";

function track(overrides: Partial<SubtitleTrack>): SubtitleTrack {
  return {
    streamId: 5,
    language: "en",
    title: "",
    isDefault: false,
    isForced: false,
    isHearingImpaired: false,
    assUrl: "/api/v1/bytes/media/sub/t/5.ass",
    vttUrl: "/api/v1/bytes/media/sub/t/5.vtt",
    ...overrides,
  };
}

const words = { unknownLanguage: "Unknown language", forced: "forced", hearingImpaired: "SDH" };

describe("subtitles", () => {
  it("names a track by its language in the viewer's language, its title and its kind", () => {
    expect(trackLabel(track({}), "en", words)).toBe("English");
    expect(trackLabel(track({ language: "de" }), "de", words)).toBe("Deutsch");
    expect(
      trackLabel(track({ title: "Signs", isForced: true, isHearingImpaired: true }), "en", words),
    ).toBe("English · Signs · forced · SDH");
    expect(trackLabel(track({ language: "" }), "en", words)).toBe("Unknown language");
  });

  it("shows an offset as signed seconds", () => {
    expect(formatOffsetSeconds(0)).toBe("0.0");
    expect(formatOffsetSeconds(300)).toBe("+0.3");
    expect(formatOffsetSeconds(-1500)).toBe("-1.5");
  });

  it("offers Off and each track, and starts at the offset the person kept", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <Subtitles
          fileId={2}
          tracks={[track({}), track({ streamId: 7, language: "fr" })]}
          startingOffsetMs={-1500}
          videoRef={createRef()}
        />,
      ),
    );
    expect(markup).toContain('<option value="" selected="">Off</option>');
    expect(markup).toContain('<option value="5">English</option>');
    expect(markup).toContain('<option value="7">French</option>');
    expect(markup).toContain("Subtitle offset: -1.5 s");
  });

  it("shows nothing for a file without subtitles", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <Subtitles fileId={2} tracks={[]} startingOffsetMs={0} videoRef={createRef()} />,
      ),
    );
    expect(markup).toBe("");
  });

  it("keeps the offset for the file", async () => {
    let sent: { url: string; method: string; body: unknown } | undefined;
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      async onRequest({ request }) {
        sent = { url: request.url, method: request.method, body: await request.json() };
        return new Response(null, { status: 204 });
      },
    });
    await keepOffset(api, 2, 250);
    expect(sent).toEqual({
      url: "http://orpheum.test/api/v1/files/2/prefs",
      method: "PUT",
      body: { subtitleOffsetMs: 250 },
    });
  });
});
