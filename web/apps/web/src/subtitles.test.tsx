// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { createRef } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  keepOffset,
  keepSubtitleChoice,
  startingTrack,
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

const words = {
  unknownLanguage: "Unknown language",
  forced: "forced",
  hearingImpaired: "SDH",
  fetched: "downloaded, unverified",
};

describe("subtitles", () => {
  it("names a track by its language in the viewer's language, its title and its kind", () => {
    expect(trackLabel(track({}), "en", words)).toBe("English");
    expect(trackLabel(track({ language: "de" }), "de", words)).toBe("Deutsch");
    expect(
      trackLabel(track({ title: "Signs", isForced: true, isHearingImpaired: true }), "en", words),
    ).toBe("English · Signs · forced · SDH");
    expect(trackLabel(track({ language: "" }), "en", words)).toBe("Unknown language");
    expect(
      trackLabel(track({ streamId: 0, fetchedId: 3, title: "Film.2002.1080p" }), "en", words),
    ).toBe("English · Film.2002.1080p · downloaded, unverified");
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
          prefs={{ subtitleOffsetMs: -1500, isDialogueBoost: false, subtitle: { streamId: 7 } }}
          videoRef={createRef()}
        />,
      ),
    );
    expect(markup).toContain('<option value="">Off</option>');
    expect(markup).toContain('<option value="s5">English</option>');
    expect(markup).toContain('<option value="s7" selected="">French</option>');
    expect(markup).toContain("Subtitle offset: -1.5 s");
  });

  it("shows nothing for a file without subtitles", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <Subtitles
          fileId={2}
          tracks={[]}
          prefs={{ subtitleOffsetMs: 0, isDialogueBoost: false }}
          videoRef={createRef()}
        />,
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

  it("starts with the file's choice only when it names one of the file's tracks", () => {
    const tracks = [track({}), track({ streamId: 7, language: "fr" })];
    expect(
      startingTrack(
        { subtitleOffsetMs: 0, isDialogueBoost: false, subtitle: { streamId: 7 } },
        tracks,
      ),
    ).toBe("s7");
    expect(
      startingTrack({ subtitleOffsetMs: 0, isDialogueBoost: false, subtitle: { fetchedId: 3 } }, [
        ...tracks,
        track({ streamId: 0, fetchedId: 3 }),
      ]),
    ).toBe("f3");
    expect(
      startingTrack(
        { subtitleOffsetMs: 0, isDialogueBoost: false, subtitle: { isOff: true } },
        tracks,
      ),
    ).toBeUndefined();
    expect(
      startingTrack(
        { subtitleOffsetMs: 0, isDialogueBoost: false, subtitle: { streamId: 99 } },
        tracks,
      ),
    ).toBeUndefined();
    expect(startingTrack({ subtitleOffsetMs: 0, isDialogueBoost: false }, tracks)).toBeUndefined();
  });

  it("keeps a choice of a track, or of off", async () => {
    const sent: unknown[] = [];
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      async onRequest({ request }) {
        sent.push(await request.json());
        return new Response(null, { status: 204 });
      },
    });
    await keepSubtitleChoice(api, 2, "s5");
    await keepSubtitleChoice(api, 2, "f3");
    await keepSubtitleChoice(api, 2, undefined);
    expect(sent).toEqual([
      { subtitle: { streamId: 5 } },
      { subtitle: { fetchedId: 3 } },
      { subtitle: { isOff: true } },
    ]);
  });
});
