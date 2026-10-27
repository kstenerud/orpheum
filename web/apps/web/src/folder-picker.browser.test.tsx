// SPDX-License-Identifier: MPL-2.0

/**
 * The folder browser's dialog as a real browser runs it (vitest's browser mode, D361 §3): focus goes
 * into it as it opens, stays in it while it is open, and returns to the button that opened it when
 * Escape closes it, as the app's other dialogs do (modal-dialog.tsx).
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";
import { userEvent } from "vitest/browser";

import { FolderBrowser } from "./folder-picker.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("the folder browser's dialog, in a real browser", () => {
  it("holds focus while open, and gives it back to its button when Escape closes it", async () => {
    const mounted = await mount(
      withServices(
        <main>
          <button type="button">behind</button>
          <FolderBrowser
            opener="Pick…"
            heading="Pick a folder"
            starts={[{ nodeName: "nas", path: "/tank/media" }]}
            onChoose={() => undefined}
          />
        </main>,
      ),
    );
    const opener = [...mounted.container.querySelectorAll("button")].find(
      (button) => button.textContent === "Pick…",
    );
    await userEvent.click(opener ?? document.body);
    expect(document.activeElement?.closest("dialog")).not.toBeNull();
    await userEvent.keyboard("{Tab}{Tab}{Tab}{Tab}");
    expect(document.activeElement?.textContent).not.toBe("behind");
    await userEvent.keyboard("{Escape}");
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
    mounted.unmount();
  });
});
