// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { startingLanguage, FindSubtitles } from "./subtitle-search.tsx";
import {
  button,
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
    allowance: { remaining: 19, allowed: 20, isVip: false },
    candidates: [
      {
        providerFileId: 7,
        release: "Film.2002.1080p",
        language: "de",
        isHashMatch: true,
        isHearingImpaired: false,
        isMachineTranslated: false,
        downloads: 120,
      },
      {
        providerFileId: 8,
        release: "Film.2002.720p",
        language: "de",
        isHashMatch: false,
        isHearingImpaired: true,
        isMachineTranslated: false,
        downloads: 3,
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
      "POST /files/2/subtitles": [200, { fetchedId: 3, remaining: 18 }],
    },
    heard,
  );
  mounted = await mount(
    withServices(
      <FindSubtitles
        fileId={2}
        fetched={() => {
          fetched++;
        }}
      />,
      services,
    ),
  );
  await click(button(mounted.container, "Find subtitles…"));
  await click(button(mounted.container, "Search OpenSubtitles"));
  const text = mounted.container.textContent;
  expect(text).toContain("19 of 20 downloads left today.");
  expect(text).toContain("None of these is checked against this film's sound");
  expect(text).toContain("Film.2002.1080p");
  expect(text).toContain("made for this very file");
  await click(mounted.container.querySelectorAll("li button")[0]);
  expect(heard.at(-1)).toEqual({
    asked: "POST /files/2/subtitles",
    body: { providerFileId: 7, language: "en" },
  });
  expect(fetched).toBe(1);
});

it("says when today's downloads are spent, and when they come back", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      429,
      {
        error: {
          code: "quota_exhausted",
          message: "a note for developers",
          detail: { resetAt: "2026-09-29T06:00:00Z" },
        },
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} fetched={() => undefined} />, services),
  );
  await click(button(mounted.container, "Find subtitles…"));
  await click(button(mounted.container, "Search OpenSubtitles"));
  expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
    "Today's downloads are spent; they start again at",
  );
});

it("tells a person when no OpenSubtitles account is set up", async () => {
  const services = servedBy({
    "GET /files/2/subtitle-candidates": [
      503,
      {
        error: {
          code: "provider_error",
          message: "x",
          detail: { reason: "no_login" },
          hop: "provider",
        },
      },
    ],
  });
  mounted = await mount(
    withServices(<FindSubtitles fileId={2} fetched={() => undefined} />, services),
  );
  await click(button(mounted.container, "Find subtitles…"));
  await click(button(mounted.container, "Search OpenSubtitles"));
  expect(mounted.container.textContent).toContain("No OpenSubtitles account is set up.");
});
