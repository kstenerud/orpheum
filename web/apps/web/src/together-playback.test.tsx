// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import {
  mountToRevisit,
  servedBy,
  settleUntil,
  unanswered,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { useGroupFilmDecision } from "./together-playback.ts";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A guest's page that reads the group's film's playback, and shows the media token it plays. */
function GroupFilm(): JSX.Element {
  const decision = useGroupFilmDecision({ fileId: 2, credential: "membership" });
  return (
    <output>{decision.state === "loaded" ? decision.value.mediaToken : decision.state}</output>
  );
}

describe("a Watch Together guest's page come back to in the same tab", () => {
  it("plays only the group film's playback it opened itself, never the one an earlier visit opened (D478)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /together/playback": [
          [200, directDecision({ mediaToken: "the first visit's" })],
          unanswered,
        ],
      },
      heard,
      "token",
    );
    const page = await mountToRevisit(withServices(<GroupFilm />, services));
    mounted = page;
    const shown = (): string | null | undefined =>
      page.container.querySelector("output")?.textContent;
    await settleUntil(() => shown() === "the first visit's");
    await page.leave();
    await page.comeBack();
    expect(heard.filter((one) => one.methodAndPath === "POST /together/playback")).toHaveLength(2);
    expect(shown()).toBe("loading");
  });
});
