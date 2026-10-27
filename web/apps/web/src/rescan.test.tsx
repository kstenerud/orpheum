// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Libraries } from "./libraries.tsx";
import { MatchCorrection } from "./match-correction.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films = {
  id: 3,
  name: "Films",
  kind: "movies",
  metadataSource: "tmdb",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  roots: [],
} as const;

const posts = (heard: readonly Heard[]): string[] =>
  heard.filter((each) => each.methodAndPath.startsWith("POST")).map((each) => each.methodAndPath);

it("rescans every library, or one, from the Libraries page, and says what started (D357 §1, §5)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [
        [200, { libraries: [films] }],
        [
          200,
          {
            libraries: [{ ...films, rescan: { isScanning: true, itemCount: 0, matchedCount: 0 } }],
          },
        ],
      ],
      "GET /nodes": [200, { nodes: [] }],
      "GET /config": [200, { settings: [] }],
      "POST /libraries/rescan": [202, { startedCount: 1 }],
      "POST /libraries/3/rescan": [202, { isStarted: false }],
    },
    heard,
  );
  mounted = await mount(withServices(<Libraries />, services));
  const page = mounted.container;
  await click(buttonWithText(page, "Rescan everything"));
  expect(page.textContent).toContain("Rescanning 1 libraries of films or series.");
  expect(page.textContent).toContain("Rescanning… scanning first.");
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain("A rescan of it is already running.");
  expect(posts(heard)).toEqual(["POST /libraries/rescan", "POST /libraries/3/rescan"]);
});

it("rescans one film or series from its page, for an admin alone (D357 §1)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /items/12/rescan": [
        [202, { isStarted: true }],
        [202, { isStarted: false }],
      ],
    },
    heard,
  );
  mounted = await mount(
    withServices(<MatchCorrection itemId={12} onCorrected={() => undefined} />, services),
  );
  expect(buttonWithText(mounted.container, "Rescan this")).toBeUndefined();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <MatchCorrection itemId={12} onCorrected={() => undefined} />
      </IsAdminContext>,
      services,
    ),
  );
  const page = mounted.container;
  await click(buttonWithText(page, "Rescan this"));
  expect(page.textContent).toContain("its files are scanned, then it is matched again from them.");
  await click(buttonWithText(page, "Rescan this"));
  expect(page.textContent).toContain("A rescan of it, or of its library, is already running.");
  expect(posts(heard)).toEqual(["POST /items/12/rescan", "POST /items/12/rescan"]);
});
