// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { createApi } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ReceiverPicturePanel, shiftedVtt, type PictureDecision } from "./picture-panel.ts";

const vtt = "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nHello\n\n01:02.000 --> 01:03.000\nThere\n";

/** A film with a track of its file's (stream 4) and one fetched (9), the person's choice as given. */
function twoSubtitles(preferences: object): PictureDecision {
  const flags = { isDefault: false, isForced: false, isHearingImpaired: false };
  return {
    subtitles: [
      { assUrl: "/s/4.ass", vttUrl: "/s/4.vtt", language: "en", streamId: 4, ...flags },
      {
        assUrl: "/s/9.ass",
        vttUrl: "/s/9.vtt",
        language: "fr",
        releaseName: "Web",
        fetchedSubtitleId: 9,
        ...flags,
      },
    ],
    preferences,
  } as unknown as PictureDecision;
}

/** A panel on a video, with what it read and kept, and the text of each track it drew. */
function panelOn(decision: PictureDecision) {
  const read: string[] = [];
  const kept: string[] = [];
  const drawn: Promise<string>[] = [];
  vi.spyOn(URL, "createObjectURL").mockImplementation((blob) => {
    drawn.push((blob as Blob).text());
    return `blob:${String(drawn.length)}`;
  });
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: async (request) => {
      kept.push(`${new URL(request.url).pathname} ${await request.text()}`);
      return Response.json({});
    },
  });
  const video = document.createElement("video");
  const panel = new ReceiverPicturePanel(
    decision,
    { api, fileId: 7, video, note: () => undefined },
    {
      fetch: (request) => {
        read.push(`${new URL(request.url).pathname} ${request.headers.get("Authorization") ?? ""}`);
        return Promise.resolve(new Response(vtt));
      },
      authorization: "Bearer token",
      language: "en",
    },
  );
  return { panel, video, read, kept, drawn };
}

describe("a Chromecast's subtitles, as its cast's remote works them", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("offers the core's tracks, keyed as the Play page keys them, and no original", () => {
    const { panel } = panelOn(twoSubtitles({}));
    expect(panel.read()).toEqual({
      subtitles: [
        { key: "s4", label: "English" },
        { key: "f9", label: "French · Web" },
      ],
      canPlayOriginal: false,
    });
  });

  it("shows the person's choice of the file at their timing, read signed in", async () => {
    const { panel, video, read, drawn } = panelOn(
      twoSubtitles({ subtitleChoice: { fetchedSubtitleId: 9 }, subtitleOffsetMs: 500 }),
    );
    expect(panel.read()).toMatchObject({ subtitleKey: "f9", subtitleOffsetMs: 500 });
    await vi.waitFor(() => {
      expect(video.querySelector("track")?.kind).toBe("subtitles");
    });
    expect(read).toEqual(["/s/9.vtt Bearer token"]);
    expect(await drawn[0]).toContain("00:00:01.500 --> 00:00:03.000");
  });

  it("shows, retimes and takes off a track as the remote chooses, kept for the person", async () => {
    const { panel, video, drawn, kept } = panelOn(twoSubtitles({}));
    expect(video.querySelector("track")).toBeNull();
    panel.choose({ kind: "subtitles", key: "s4" });
    await vi.waitFor(() => {
      expect(video.querySelectorAll("track")).toHaveLength(1);
    });
    panel.choose({ kind: "subtitleOffset", ms: -1500 });
    expect(video.querySelectorAll("track")).toHaveLength(1);
    expect(await drawn[1]).toContain("00:00:00.000 --> 00:00:01.000");
    panel.choose({ kind: "subtitles", key: undefined });
    expect(video.querySelector("track")).toBeNull();
    await expect
      .poll(() => kept)
      .toEqual([
        '/api/v1/files/7/preferences {"subtitleChoice":{"streamId":4}}',
        '/api/v1/files/7/preferences {"subtitleOffsetMs":-1500}',
        '/api/v1/files/7/preferences {"subtitleChoice":{"isOff":true}}',
      ]);
  });

  it("says nothing to choose for a film with no tracks, as from an older core", () => {
    expect(panelOn({}).panel.read()).toBeUndefined();
  });
});

describe("a WebVTT track moved in time", () => {
  it("moves every cue, hours or none, never before the film's start", () => {
    expect(shiftedVtt(vtt, 1000)).toContain("00:00:02.000 --> 00:00:03.500");
    expect(shiftedVtt(vtt, 1000)).toContain("00:01:03.000 --> 00:01:04.000");
    expect(shiftedVtt(vtt, -5000)).toContain("00:00:00.000 --> 00:00:00.000");
    expect(shiftedVtt(vtt, 0)).toBe(vtt);
  });
});
