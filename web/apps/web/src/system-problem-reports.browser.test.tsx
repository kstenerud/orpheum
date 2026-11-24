// SPDX-License-Identifier: MPL-2.0

/**
 * The System page's problem reports as a real browser lays them out: a report's suggested sound
 * timing stands as its downloads do, its words first and its Apply under them, at the Apply's own
 * width, set apart from the words by the section's gap (system-problem-reports.css).
 */

import "./i18n.ts";
import "./tokens.css";
import "./base.css";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { SystemProblemReports } from "./system-problem-reports.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** How tall a space token draws, in pixels, as this browser resolves it. */
function pixelsOf(token: string): number {
  const probe = document.createElement("div");
  probe.style.height = `var(${token})`;
  document.body.append(probe);
  const height = probe.getBoundingClientRect().height;
  probe.remove();
  return height;
}

describe("the System page's problem reports in a real browser (D537 §5)", () => {
  it("puts a suggested sound timing's Apply under its words, at its own width, a gap apart", async () => {
    const suggesting = {
      id: 4,
      userName: "dan",
      madeAt: 1_790_000_000_000,
      version: "0.9.0",
      words: "the sound drifts",
      soundOffset: { fileId: 31, soundOffsetMs: -120 },
    };
    mounted = await mount(
      withServices(
        <SystemProblemReports />,
        servedBy({ "GET /problem-reports": [200, { reports: [suggesting] }] }),
      ),
    );
    // A report's body shows once its summary is opened, as the admin does.
    await page.getByText("dan", { exact: false }).first().click();
    const applying = page.getByRole("button", { name: "Apply to everyone" });
    await expect.element(applying).toBeVisible();
    const row = mounted.container.querySelector(".system-problem-reports__sound-offset");
    const words = row?.querySelector("p");
    if (row === null || words === null || words === undefined) {
      throw new Error("the suggested sound timing's row and its words are drawn");
    }
    const wordsBox = words.getBoundingClientRect();
    const apply = applying.element().getBoundingClientRect();
    expect(apply.left).toBeCloseTo(wordsBox.left, 0);
    expect(apply.width).toBeLessThan(row.getBoundingClientRect().width / 2);
    // The words' own space under them and the row's gap, which a flex column does not collapse.
    expect(apply.top - wordsBox.bottom).toBeCloseTo(
      pixelsOf("--space-3") + pixelsOf("--space-2"),
      0,
    );
  });
});
