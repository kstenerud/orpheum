// SPDX-License-Identifier: MPL-2.0

/**
 * A setting's row as a real browser lays it out (settings.css; ui-audit-2026-11-04.md §3): in one
 * column, its name, then its key under it, then its field with its controls, and no wider than a
 * reader's line on a wide screen.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { SecretsView } from "./settings.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** How far layout's rounding may put a box, in px. */
const roundingPx = 2;

/** The row's widest, 40rem, in px. */
const widestRowPx = 640;

function ignore(): void {
  // The layout alone is looked at.
}

describe("a setting's row", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("stands in one column: its name, its key under it, then its field, a line's width at most", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <SecretsView
          search=""
          problems={new Map()}
          onSave={ignore}
          onRemove={ignore}
          secrets={[{ name: "tmdbApiKey", description: "TMDB", isSet: false, needsReentry: false }]}
        />,
      ),
    );
    const box = (selector: string): DOMRect => {
      const element = mounted?.container.querySelector(selector);
      if (element === null || element === undefined) {
        throw new Error(`nothing drawn at ${selector}`);
      }
      return element.getBoundingClientRect();
    };
    const name = box(".settings__label");
    const key = box(".settings__key");
    const field = box(".settings__field");
    expect(key.top).toBeGreaterThanOrEqual(name.bottom - roundingPx);
    expect(field.top).toBeGreaterThanOrEqual(key.bottom - roundingPx);
    expect(Math.abs(key.left - name.left)).toBeLessThanOrEqual(roundingPx);
    expect(Math.abs(field.left - name.left)).toBeLessThanOrEqual(roundingPx);
    expect(box(".settings__row").width).toBeLessThanOrEqual(widestRowPx + roundingPx);
  });
});
