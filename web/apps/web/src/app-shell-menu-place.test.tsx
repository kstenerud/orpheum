// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { ShellReadContext, type ShellBody } from "./shell-read.ts";
import { click, mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("the menu's places", () => {
  it("marks an admin's place in the menu while something on its page needs them, the warning icon at its end, its name saying so, no number (D538 §12)", async () => {
    const shell = {
      needsAttention: {
        pages: { settings: { isProblem: true, itemCount: 3, items: [] } },
      },
    } as unknown as ShellBody;
    const mounted = await mount(
      withServices(
        <ShellReadContext value={{ state: "read", shell }}>
          <AppShell
            isAdmin={true}
            canHostWatchTogether={true}
            currentScreen="jobs"
            onSignOut={ignore}
            signedInAs={{ name: "karl", deviceName: undefined }}
          >
            <p>screen</p>
          </AppShell>
        </ShellReadContext>,
      ),
    );
    await click(mounted.container.querySelector(".app-shell__menu-button"));
    const menu = mounted.container;
    const settings = menu.querySelector('.list-box a[href="/settings"]');
    expect(settings?.getAttribute("aria-label")).toBe("Settings, needs attention");
    expect(settings?.querySelector(".app-shell__attention-mark .warning-icon")).not.toBeNull();
    expect(settings?.textContent).toBe("Settings");
    const system = menu.querySelector('.list-box a[href="/system"]');
    expect(system?.getAttribute("aria-label")).toBeNull();
    expect(system?.querySelector(".warning-icon")).toBeNull();
    mounted.unmount();
  });
});
