// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { RouteChooser } from "./route-chooser.tsx";
import { click, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("a chooser", () => {
  // The owner, 2026-10-28: it looked "no different from a regular button, so there's no way to know
  // that it would bring up a choice list".
  it("says it opens a list, by a chevron and to a screen reader, and whether it is open", async () => {
    shown = await mount(
      withServices(
        <RouteChooser
          fieldLabel="Season"
          current="Season 1"
          options={[
            {
              key: "1",
              label: "Season 1",
              to: { screen: "series-page", collectionId: 5, season: 1 },
              isCurrent: true,
            },
            {
              key: "2",
              label: "Season 2",
              to: { screen: "series-page", collectionId: 5, season: 2 },
              isCurrent: false,
            },
          ]}
          isAView
        />,
      ),
    );
    const button = shown.container.querySelector(".route-chooser__button");
    expect(button?.getAttribute("aria-haspopup")).toBe("dialog");
    expect(button?.getAttribute("aria-expanded")).toBe("false");
    const chevron = button?.querySelector("svg.icon.route-chooser__chevron");
    expect(chevron?.getAttribute("aria-hidden")).toBe("true");
    // Its name is its choice alone: the chevron says nothing to a screen reader.
    expect(button?.textContent).toBe("Season 1");
    await click(button);
    expect(button?.getAttribute("aria-expanded")).toBe("true");
    expect(shown.container.querySelector("dialog")?.getAttribute("aria-label")).toBe("Season");
  });
});
