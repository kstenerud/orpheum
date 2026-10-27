// SPDX-License-Identifier: MPL-2.0

/**
 * The shell, a poster's cues and the gallery's banners as a real browser lays them out: what only
 * their stylesheets decide, in Chromium and Firefox, with no server (vitest's browser mode). They
 * were the browser tier's shell.spec.ts, seconds each; here they take a tenth of one (the owner,
 * 2026-10-18).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { AppShell } from "./app-shell.tsx";
import { PosterGrid, type Entry } from "./browse.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { UiGallery } from "./ui-gallery.tsx";

/** Where element is drawn; it fails the test when there is none. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

/** Waits until the image has loaded, or failed to. */
async function loaded(image: HTMLImageElement | null): Promise<void> {
  if (image !== null && !image.complete) {
    await new Promise((resolve) => {
      image.addEventListener("load", resolve, { once: true });
      image.addEventListener("error", resolve, { once: true });
    });
  }
}

/** Runs check over what tree draws, then unmounts it. */
async function drawn(
  tree: Parameters<typeof mount>[0],
  check: (mounted: Mounted) => Promise<void> | void,
): Promise<void> {
  const mounted = await mount(withServices(tree));
  try {
    await check(mounted);
  } finally {
    mounted.unmount();
  }
}

describe("the shell, laid out by a real browser", () => {
  it("draws the logo as one link home, exactly the logo's size", async () => {
    const shell = (
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen="films"
        onSignOut={() => undefined}
        signedInAs={{ name: "owner", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>
    );
    await drawn(shell, async ({ container }) => {
      const logo = container.querySelector<HTMLImageElement>("img.app-shell__logo");
      await loaded(logo);
      const [link, image] = [boxOf(container.querySelector(".app-shell__home")), boxOf(logo)];
      expect(image.height).toBeGreaterThan(0);
      // An inline link around the logo is a line of text tall, not the logo's own box.
      expect(link.height).toBeCloseTo(image.height, 0);
      expect(link.width).toBeCloseTo(image.width, 0);
    });
  });

  it("draws a poster's status cues as a tray down from its top-right corner, over the poster (D301 §4)", async () => {
    // One that cannot play and is a guess: the tray holds both cues, in their order.
    const entry: Entry = {
      addedAt: 0,
      id: 1,
      identificationState: "matched",
      isGuessed: true,
      kind: "movie",
      name: "A Film",
      unavailable: { reason: "node_away", nodeName: "devnas" },
      year: 2020,
    };
    await drawn(<PosterGrid kind="movie" entries={[entry]} />, ({ container }) => {
      const item = container.querySelector(".poster-grid > .poster-card");
      const poster = boxOf(item?.querySelector(".poster"));
      const signs = item?.querySelectorAll(".poster-card__cues > .poster-cue") ?? [];
      expect(signs).toHaveLength(2);
      const [first, second] = [boxOf(signs[0]), boxOf(signs[1])];
      // In the poster's top-right corner, the second under the first, both over the poster.
      expect(poster.right - first.right).toBeLessThan(20);
      expect(first.top - poster.top).toBeLessThan(20);
      expect(first.top).toBeGreaterThanOrEqual(poster.top);
      expect(second.top).toBeGreaterThanOrEqual(first.bottom);
      expect(Math.abs(second.right - first.right)).toBeLessThan(2);
      expect(second.bottom).toBeLessThan(poster.bottom);
      // The uncertain match's disc in the owner's caution yellow (D301 §4).
      const disc = item?.querySelector(".uncertain-match-icon__disc");
      expect(disc === null || disc === undefined ? "" : getComputedStyle(disc).fill).toBe(
        "rgb(247, 206, 70)",
      );
      // Each cue the size of the warning sign's.
      expect(Math.abs(second.height - first.height)).toBeLessThan(2);
      expect(Math.abs(second.width - first.width)).toBeLessThan(2);
    });
  });

  it("draws the gallery's status trays at a banner's end, in one row, as the banner does (D280)", async () => {
    await drawn(<UiGallery />, ({ container }) => {
      const banners = [...container.querySelectorAll(".ui-gallery__banner")];
      expect(banners.length).toBeGreaterThan(1);
      for (const banner of banners) {
        const box = boxOf(banner);
        // A banner's width, not the page's, so its end is where a banner's is.
        expect(box.width).toBeLessThanOrEqual(24 * 16 + 1);
        const realm = boxOf(banner.querySelector(".app-shell__realm"));
        const menu = boxOf(banner.querySelector(".ui-gallery__menu-mark"));
        // The menu last, at the banner's end inside its padding, level with the realm beside it.
        expect(box.right - menu.right).toBeLessThan(20);
        expect(menu.left).toBeGreaterThan(realm.right - 1);
        expect(Math.abs(menu.top + menu.height / 2 - (realm.top + realm.height / 2))).toBeLessThan(
          3,
        );
      }
    });
  });

  it("colours each status cue by its tokens wherever it is drawn, and every other icon as its words", async () => {
    // The cues' colours, the tokens' (tokens.css), now set by each cue's region (the owner, 2026-10-27).
    const cues: readonly (readonly [string, "fill" | "stroke", string])[] = [
      [".warning-icon__triangle", "fill", "rgb(255, 204, 0)"],
      [".warning-icon__triangle", "stroke", "rgba(0, 0, 0, 0.35)"],
      [".warning-icon__mark", "fill", "rgb(17, 17, 17)"],
      [".uncertain-match-icon__ring", "fill", "rgb(28, 29, 34)"],
      [".uncertain-match-icon__disc", "fill", "rgb(247, 206, 70)"],
      [".uncertain-match-icon__curve", "stroke", "rgb(28, 29, 34)"],
      [".uncertain-match-icon__dot", "fill", "rgb(28, 29, 34)"],
      [".still-reading-icon__ring", "fill", "rgb(4, 38, 61)"],
      [".still-reading-icon__disc", "fill", "rgb(124, 200, 255)"],
      [".still-reading-icon__glass", "stroke", "rgb(4, 38, 61)"],
      [".still-reading-icon__sand", "fill", "rgb(4, 38, 61)"],
    ];
    await drawn(<UiGallery />, ({ container }) => {
      for (const [selector, paint, colour] of cues) {
        // Over posters, in the icons' row, and beside a note, the cannot-play one's words yellow.
        const parts = [...container.querySelectorAll(selector)];
        expect(parts.length, selector).toBeGreaterThan(1);
        for (const part of parts) {
          expect(getComputedStyle(part)[paint], `${selector} ${paint}`).toBe(colour);
        }
      }
      const icons = [...container.querySelectorAll(".ui-gallery__icons svg")].filter(
        (icon) =>
          !/warning-icon|uncertain-match-icon|still-reading-icon/u.test(
            icon.getAttribute("class") ?? "",
          ),
      );
      expect(icons.length).toBeGreaterThan(5);
      for (const icon of icons) {
        const words = getComputedStyle(icon.parentElement ?? icon).color;
        const shapes = [...icon.querySelectorAll("path, circle, rect")];
        expect(shapes.length).toBeGreaterThan(0);
        // Each at an icon's size beside the others, the blank poster's mark a share of a poster's width.
        const { width } = boxOf(icon);
        expect(width, icon.getAttribute("class") ?? "").toBeGreaterThanOrEqual(16);
        expect(width, icon.getAttribute("class") ?? "").toBeLessThanOrEqual(48);
        for (const shape of shapes) {
          const { fill, stroke } = getComputedStyle(shape);
          expect([fill, stroke], icon.getAttribute("class") ?? "").toContain(words);
        }
      }
    });
  });

  it("keeps every place of the open menu on the screen of a Fire TV's Silk, Sign out the last", async () => {
    // Silk's page on a Fire TV is 960 by 480 below its address bar: an admin's full menu ran past its
    // bottom, Sign out with it, out of the remote's sight (the owner's Fire TV, 2026-10-27).
    await page.viewport(960, 480);
    const shell = (
      <AppShell
        isAdmin={true}
        canHostWatchTogether={true}
        currentScreen="films"
        onSignOut={() => undefined}
        signedInAs={{ name: "owner", deviceName: "Silk on Fire TV" }}
        showsDeveloperPages={true}
        showsDeviceTests={true}
        onSwitchHousehold={() => undefined}
      >
        <p>screen</p>
      </AppShell>
    );
    await drawn(shell, ({ container }) => {
      const menu = container.querySelector("details");
      if (menu === null) {
        throw new Error("the shell draws no menu");
      }
      menu.open = true;
      const places = [...menu.querySelectorAll<HTMLElement>("ul a, ul button")];
      expect(places.at(-1)?.textContent).toBe("Sign out");
      for (const place of places) {
        place.focus();
        const box = boxOf(place);
        expect(box.top, place.textContent).toBeGreaterThanOrEqual(0);
        expect(box.bottom, place.textContent).toBeLessThanOrEqual(window.innerHeight);
      }
    });
  });
});
