// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

/**
 * The banner's Libraries icon as a menu of the person's libraries (the owner, 2026-10-28: "make the
 * libraries icon a menu that lists the libraries under it"; app-shell.tsx's LibrariesMenu).
 */

import { act } from "react";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import type { Services } from "./services.tsx";
import { click, fire, mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

/** A library the person reaches, as GET /libraries lists it. */
function reached(id: number, name: string): Record<string, unknown> {
  return { id, name, kind: "movies", metadataSource: "tmdb", isWritable: true, createdAt: 0 };
}

/** A shell at the Libraries page for library, over a server that lists libraries. */
async function shellOverLibraries(
  libraries: readonly Record<string, unknown>[],
  library: number | "all",
): Promise<{ readonly mounted: Mounted; readonly services: Services }> {
  const services = servedBy({ "GET /libraries": [200, { libraries }] }, [], "token");
  services.navigation.go({ screen: "browse", library });
  const mounted = await mount(
    withServices(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen="browse"
        onSignOut={ignore}
        signedInAs={undefined}
      >
        <p>{"screen"}</p>
      </AppShell>,
      services,
    ),
  );
  return { mounted, services };
}

/** The banner's Libraries icon where it opens a menu: its button. */
const librariesButton = '.app-shell__watching button[aria-label="Libraries"]';

describe("the banner's Libraries", () => {
  it("opens a menu of the person's libraries by name, All libraries last, the one shown marked, each its page", async () => {
    const { mounted, services } = await shellOverLibraries(
      [reached(2, "Zoo"), reached(1, "Alpha")],
      2,
    );
    const { container } = mounted;
    await settleUntil(() => container.querySelector(librariesButton) !== null);
    await click(container.querySelector(librariesButton));
    const links = [...container.querySelectorAll<HTMLAnchorElement>("dialog ul a")];
    expect(links.map((link) => link.textContent)).toEqual(["Alpha", "Zoo", "Libraries"]);
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/browse?library=1",
      "/browse?library=2",
      "/browse?library=all",
    ]);
    expect(links.map((link) => link.getAttribute("aria-current"))).toEqual([null, "page", null]);
    await click(links[0]);
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 1 });
    expect(container.querySelector("dialog")).toBeNull();
    mounted.unmount();
  });

  it("goes straight to the page of a person's one library, with no menu of one", async () => {
    const { mounted } = await shellOverLibraries([reached(7, "Films")], 7);
    const { container } = mounted;
    const onlyOne = '.app-shell__watching a[href="/browse?library=7"]';
    await settleUntil(() => container.querySelector(onlyOne) !== null);
    expect(container.querySelector(librariesButton)).toBeNull();
    expect(container.querySelector(onlyOne)?.getAttribute("aria-label")).toBe("Libraries");
    mounted.unmount();
  });

  it("closes its menu as any chooser does, on Left, Right or a remote's Back, focus back on its icon", async () => {
    const { mounted, services } = await shellOverLibraries(
      [reached(1, "Alpha"), reached(2, "Zoo")],
      "all",
    );
    const { container } = mounted;
    await settleUntil(() => container.querySelector(librariesButton) !== null);
    const button = container.querySelector<HTMLElement>(librariesButton);
    const keys = [
      { key: "ArrowLeft" },
      { key: "ArrowRight" },
      { key: "Unidentified", keyCode: 461 },
    ];
    for (const key of keys) {
      act(() => {
        button?.focus();
      });
      await click(button);
      const first = container.querySelector<HTMLElement>("dialog ul a");
      act(() => {
        first?.focus();
      });
      await fire(first, new KeyboardEvent("keydown", { ...key, bubbles: true, cancelable: true }));
      expect(container.querySelector("dialog"), key.key).toBeNull();
      expect(document.activeElement, key.key).toBe(button);
    }
    expect(services.navigation.route()).toEqual({ screen: "browse", library: "all" });
    mounted.unmount();
  });
});
