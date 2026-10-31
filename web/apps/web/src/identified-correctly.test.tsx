// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SeriesView, type SeriesDetail } from "./series.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const guessed: SeriesDetail = {
  id: 20,
  kind: "series",
  name: "A Series",
  art: {},
  seasons: [],
  isGuessed: true,
};

it("offers an admin Identified correctly on a series whose match is a guess, which confirms it (D301 §4)", async () => {
  const heard: Heard[] = [];
  let confirmed = 0;
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <SeriesView
          series={guessed}
          onMatchCorrected={() => {
            confirmed += 1;
          }}
        />
      </IsAdminContext>,
      servedBy({ "POST /items/20/confirm-match": [204, null] }, heard),
    ),
  );
  const page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  expect(page.textContent).toContain("Identified incorrectly");
  await click(buttonWithText(page, "Identified correctly"));
  await click(buttonWithText(page, "Confirm"));
  await settleUntil(() => confirmed === 1);
  expect(heard.map((request) => request.methodAndPath)).toContain("POST /items/20/confirm-match");
  expect(page.querySelector("dialog")).toBeNull();
});

it("offers no Identified correctly on a series whose match is no guess", async () => {
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <SeriesView series={{ ...guessed, isGuessed: false }} onMatchCorrected={() => undefined} />
      </IsAdminContext>,
    ),
  );
  const page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  expect(page.textContent).toContain("Identified incorrectly");
  expect(page.textContent).not.toContain("Identified correctly");
});
