// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { startingLanguage, FindSubtitles } from "./subtitle-search.tsx";
import {
  buttonWithText,
  chooseIn,
  click,
  mount,
  servedBy,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const offer: Answer = [
  200,
  {
    allowance: { remainingDownloads: 19, allowedDownloads: 20, isVip: false },
    didAskByIds: true,
    didAskByOpensubtitlesHash: true,
    candidates: [
      {
        providerFileId: 7,
        releaseName: "Film.2002.1080p",
        language: "de",
        isOpensubtitlesHashMatch: true,
        isHearingImpaired: false,
        isMachineTranslated: false,
        downloadCount: 120,
      },
      {
        providerFileId: 8,
        releaseName: "Film.2002.720p",
        language: "de",
        isOpensubtitlesHashMatch: false,
        isHearingImpaired: true,
        isMachineTranslated: false,
        downloadCount: 3,
      },
    ],
  },
];

it("starts in the viewer's language when it can be asked for, else English", () => {
  expect(startingLanguage("de-AT")).toBe("de");
  expect(startingLanguage("pt-BR")).toBe("pt-BR");
  expect(startingLanguage("tlh")).toBe("en");
});

it("lists what OpenSubtitles has, unverified and made-for-this-file first, and fetches the one chosen (D224)", async () => {
  const heard: Heard[] = [];
  let fetched = 0;
  const services = servedBy(
    {
      "GET /files/2/subtitle-candidates": offer,
      "POST /files/2/subtitles": [200, { fetchedSubtitleId: 3, remainingDownloads: 18 }],
    },
    heard,
  );
  mounted = await mount(
    withServices(
      <FindSubtitles
        fileId={2}
        onFetched={() => {
          fetched++;
        }}
      />,
      services,
    ),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  const text = mounted.container.textContent;
  expect(text).toContain("19 of 20 downloads left today.");
  expect(text).toContain("Downloaded subtitles aren't checked against the sound");
  expect(text).not.toContain("isn't identified yet");
  expect(text).toContain("Film.2002.1080p");
  expect(text).toContain("made for this very file");
  await click(mounted.container.querySelectorAll("li button")[0]);
  expect(heard.at(-1)).toEqual({
    methodAndPath: "POST /files/2/subtitles",
    body: { providerFileId: 7, language: "en" },
  });
  expect(fetched).toBe(1);
});

// The chooser, not a native select, which a television's browser cannot work (the 2026-10-27
// widget audit; plans/toolkit.md step 6).
it("asks in the language chosen from its chooser, and fetches in it", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /files/2/subtitle-candidates": offer,
      "POST /files/2/subtitles": [200, { fetchedSubtitleId: 3, remainingDownloads: 18 }],
    },
    heard,
  );
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} onFetched={() => undefined} />, services),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  expect(mounted.container.querySelector("select")).toBeNull();
  await chooseIn(mounted.container, "Language", "German");
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  expect(heard.at(-1)).toMatchObject({
    methodAndPath: "GET /files/2/subtitle-candidates",
    query: "?language=de",
  });
  await click(mounted.container.querySelectorAll(".subtitle-search li button")[0]);
  expect(heard.at(-1)?.body).toEqual({ providerFileId: 7, language: "de" });
});

it("says an empty search of a file not identified yet asked only for subtitles made for that file", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      200,
      {
        allowance: { remainingDownloads: 19, allowedDownloads: 20, isVip: false },
        didAskByIds: false,
        didAskByOpensubtitlesHash: true,
        candidates: [],
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} onFetched={() => undefined} />, services),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  const text = mounted.container.textContent;
  expect(text).toContain("OpenSubtitles has no English subtitles made for this exact file.");
  expect(text).toContain(
    "This video isn't identified yet, so only subtitles made for this exact file",
  );
  expect(text).not.toContain("Earlier and Later");
});

it("says when today's downloads are spent, and when they come back", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      429,
      {
        error: {
          code: "quotaExhausted",
          message: "a note for developers",
          detail: { downloadsResetAt: 1790661600000 },
        },
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} onFetched={() => undefined} />, services),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  expect(mounted.container.querySelector('[role="alert"]')?.textContent).toMatch(
    /Today's downloads are spent; they start again at \d/u,
  );
});

it("tells a person when no OpenSubtitles account is set up", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      503,
      {
        error: {
          code: "providerError",
          message: "x",
          detail: { reason: "noLogin" },
          failedHop: "provider",
        },
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} onFetched={() => undefined} />, services),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  expect(mounted.container.textContent).toContain("No OpenSubtitles account is set up.");
});

it("tells a person when OpenSubtitles is not answering", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      503,
      {
        error: {
          code: "providerError",
          message: "x",
          detail: { reason: "unavailable" },
          failedHop: "provider",
        },
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} onFetched={() => undefined} />, services),
  );
  await click(buttonWithText(mounted.container, "Find subtitles…"));
  await click(buttonWithText(mounted.container, "Search OpenSubtitles"));
  expect(mounted.container.textContent).toContain("OpenSubtitles is not answering right now.");
});
