// SPDX-License-Identifier: MPL-2.0

/**
 * The dialogs that ask before a dangerous action, in a real browser: their safe choice first in the
 * row and where focus arrives as they open (D421), decided by the mark on the dangerous one.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { ConfirmDialog } from "./confirm-button.tsx";
import { RepointDialog } from "./repoint-root.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The choices of the open dialog's row, by what they say, its Close apart. */
function choicesShown(): (string | null)[] {
  return [...document.querySelectorAll("dialog[open] button")]
    .filter((button) => button.closest("[data-dialog-close]") === null)
    .map((button) => button.textContent);
}

/** Waits a frame, past the dialog's first look for its arrival. */
async function aFrame(): Promise<void> {
  await new Promise((resolve) => requestAnimationFrame(resolve));
}

describe("a dialog asking before a dangerous action (D421)", () => {
  it("asks with Keep it first and focused, its action marked, no arrival written by hand", async () => {
    const mounted = await mount(
      withServices(
        <ConfirmDialog
          heading="Retire this folder?"
          consequence="It is never scanned again."
          confirmLabel="Retire the folder"
          onConfirm={() => undefined}
          onClose={() => undefined}
        />,
      ),
    );
    expect(choicesShown()).toEqual(["Keep it", "Retire the folder"]);
    await expect.poll(() => document.activeElement?.textContent).toBe("Keep it");
    await aFrame();
    expect(document.activeElement?.textContent).toBe("Keep it");
    expect(document.querySelector("dialog [data-focus-start]")).toBeNull();
    mounted.unmount();
  });

  it("asks where a folder moved with Keep it where it is before the move, and focused", async () => {
    const mounted = await mount(
      withServices(
        <RepointDialog
          from={{ nodeName: "devnas", path: "/media/films" }}
          places={[{ nodeName: "attic", path: "/films" }]}
          onRepoint={() => undefined}
          onClose={() => undefined}
        />,
      ),
    );
    const row = choicesShown();
    expect(row.indexOf("Keep it where it is")).toBeLessThan(row.indexOf("Use the new place"));
    await expect.poll(() => document.activeElement?.textContent).toBe("Keep it where it is");
    await aFrame();
    expect(document.activeElement?.textContent).toBe("Keep it where it is");
    mounted.unmount();
  });
});
