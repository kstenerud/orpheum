// SPDX-License-Identifier: MPL-2.0

/**
 * The Jobs page's ended days as a real browser lays them out (jobs.css): each folded under its date
 * and count, a line with room above and below it, so the folded days are each a press's target, not
 * a crowded list.
 */

import "./tokens.css";
import "./base.css";
import "./jobs.css";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { mount, type Mounted } from "./test-dom.tsx";

describe("an ended day of jobs", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is folded under a line with room above and below its date", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      <details className="jobs__day">
        <summary>4 November · 3 jobs</summary>
      </details>,
    );
    const summary = mounted.container.querySelector("summary");
    if (summary === null) {
      throw new Error("no day drawn");
    }
    const style = getComputedStyle(summary);
    expect(Number.parseFloat(style.paddingTop)).toBeGreaterThan(0);
    expect(Number.parseFloat(style.paddingBottom)).toBeGreaterThan(0);
  });
});
