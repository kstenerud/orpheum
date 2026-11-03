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

import { describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { AppShell } from "./app-shell.tsx";
import { PosterGrid, type Entry } from "./browse.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";
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

/**
 * The places' icons on the tray's centre line and as white as its signs (the owner, 2026-10-28: "the
 * new icons are darker than the cast and watch together ones. Also the two icon sets aren't
 * vertically aligned").
 */
function expectPlacesLikeTheTray(container: Element): void {
  const sign = container.querySelector(
    ".app-shell__end .status-tray button.icon-button:not(.icon-button--running)",
  );
  const centreOf = (element: Element | null | undefined): number =>
    boxOf(element).top + boxOf(element).height / 2;
  for (const place of container.querySelectorAll(".app-shell__watching a")) {
    expect(
      Math.abs(centreOf(place.querySelector("svg")) - centreOf(sign?.querySelector("svg"))),
    ).toBeLessThan(1.5);
    expect(getComputedStyle(place).color).toBe(getComputedStyle(sign ?? place).color);
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

  // D364 §12: a sign whose function is off is white and still; only one that is on breathes. The
  // IconButton batch left every sign's icon pulsing, the off ones too (check-pr, 2026-11-03).
  it("keeps a status sign whose function is off still, its icon unanimated", async () => {
    const shell = (
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen="home"
        onSignOut={() => undefined}
        signedInAs={{ name: "owner", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>
    );
    await drawn(shell, ({ container }) => {
      const offs = container.querySelectorAll(
        ".app-shell__end .status-tray button.icon-button:not(.icon-button--running) svg",
      );
      expect(offs.length).toBeGreaterThan(0);
      for (const icon of offs) {
        expect(getComputedStyle(icon).animationName).toBe("none");
      }
    });
  });

  // The owner, 2026-10-28: "The icons on the right highlight on mouse-over while the ones on the
  // left do not."
  it("raises the ground behind every banner icon a pointer is over, the places' and the menu's as the tray's", async () => {
    const shell = (
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen="home"
        onSignOut={() => undefined}
        signedInAs={{ name: "owner", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>
    );
    await drawn(shell, async ({ container }) => {
      const hovered = async (element: Element | null): Promise<Element> => {
        if (element === null) {
          throw new Error("a banner icon the test hovers is not drawn");
        }
        await userEvent.hover(element);
        return element;
      };
      // A place's icon is a link, whose ground shows at once; the tray's signs, buttons since they
      // open their dialogs (D384 §2), fade theirs in as the menu's does.
      const place = await hovered(
        container.querySelector(".app-shell__watching a.app-shell__place-icon"),
      );
      const raised = getComputedStyle(place).backgroundColor;
      expect(raised).not.toBe("rgba(0, 0, 0, 0)");
      const icons = [
        ...container.querySelectorAll(".app-shell__watching a.app-shell__place-icon"),
        container.querySelector(
          ".app-shell__end .status-tray button.icon-button:not(.icon-button--running)",
        ),
        container.querySelector(".app-shell__end .app-shell__menu-button"),
      ];
      expect(icons.length).toBeGreaterThan(1);
      for (const icon of icons) {
        const over = await hovered(icon);
        // A button's ground fades in (base.css), so its colour is read once the fade is done.
        await expect
          .poll(() => getComputedStyle(over).backgroundColor, { message: over.className })
          .toBe(raised);
      }
    });
  });

  // The owner, 2026-10-28, on a phone: "the top bar stuff is overlapping".
  for (const [width, height] of [
    [360, 740],
    [412, 915],
  ] as const) {
    it(`lays out a phone ${String(width)} wide in one row: the laurel, the places as icons, the tray and the menu, nothing overlapping`, async () => {
      await page.viewport(width, height);
      const shell = (
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={() => undefined}
          signedInAs={{ name: "owner", deviceName: undefined }}
          realm="staging"
          showsDeviceTests={true}
        >
          <p>screen</p>
        </AppShell>
      );
      try {
        await drawn(shell, async ({ container }) => {
          const logo = container.querySelector<HTMLImageElement>("img.app-shell__logo");
          await loaded(logo);
          // The laurel alone at a phone's width: Firefox picks a picture's source again only once it
          // has laid out at the new width, after a test before resized the window.
          await expect.poll(() => logo?.currentSrc ?? "").toMatch(/favicon\.svg$/u);
          await loaded(logo);
          const [mark, end, places] = [
            boxOf(logo),
            boxOf(container.querySelector(".app-shell__end")),
            boxOf(container.querySelector(".app-shell__watching")),
          ];
          // One row: the laurel, the places for watching as icons in the room left, then the tray
          // and the menu (the owner, 2026-10-28), none over another or past the window.
          expect(places.left).toBeGreaterThanOrEqual(mark.right);
          expect(end.left).toBeGreaterThanOrEqual(places.right - 1);
          expect(end.right).toBeLessThanOrEqual(width);
          for (const box of [places, end]) {
            expect(Math.abs(box.top + box.height / 2 - (mark.top + mark.height / 2))).toBeLessThan(
              mark.height,
            );
          }
          expectPlacesLikeTheTray(container);
          // Every place for watching in view, none scrolled out of the row.
          for (const place of container.querySelectorAll(".app-shell__watching a")) {
            expect(boxOf(place).right).toBeLessThanOrEqual(places.right + 1);
          }
          // The tray's signs and the menu's icon a quarter closer than a fingertip square apart.
          const signs = [
            ...container.querySelectorAll(
              ".app-shell__end .status-tray .icon-button, .app-shell__end .app-shell__menu-button",
            ),
          ];
          expect(signs.length).toBeGreaterThanOrEqual(3);
          const centres = signs.map((sign) => boxOf(sign).left + boxOf(sign).width / 2);
          const fingertip =
            2.75 * Number.parseFloat(getComputedStyle(document.documentElement).fontSize);
          for (const [index, centre] of centres.slice(1).entries()) {
            expect(centre - (centres[index] ?? 0)).toBeCloseTo(fingertip * 0.75, 0);
          }
          // The realm is the menu's to say, and the places for watching fit the row with no lab.
          expect(
            container.querySelector(".app-shell__end .app-shell__realm")?.getBoundingClientRect()
              .width ?? 0,
          ).toBe(0);
          expect(container.querySelector('.app-shell__watching a[href="/lab"]')).toBeNull();
        });
      } finally {
        await page.viewport(1280, 720);
      }
    });
  }

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
      // The watched check, white on the tray's patch (D370 §2).
      [".watched-icon__tick", "stroke", "rgb(255, 255, 255)"],
      [".watched-icon__tick", "fill", "none"],
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
          !/warning-icon|uncertain-match-icon|still-reading-icon|watched-icon/u.test(
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

  // The owner, 2026-10-28: a list opened elsewhere "feels weird"; the menu opens at its icon.
  it.each([
    { name: "a Fire TV's Silk", width: 960, height: 480 },
    { name: "a phone", width: 390, height: 740 },
  ])("opens the menu at its icon, inside the screen, on $name", async ({ width, height }) => {
    await page.viewport(width, height);
    await drawn(
      <AppShell
        isAdmin={true}
        canHostWatchTogether={true}
        currentScreen="films"
        onSignOut={() => undefined}
        signedInAs={{ name: "owner", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>,
      async ({ container }) => {
        await userEvent.click(page.getByRole("button", { name: "Menu" }));
        const [menu, icon] = [
          boxOf(container.querySelector("dialog")),
          boxOf(container.querySelector(".app-shell__menu-button")),
        ];
        const roundingPx = 1;
        expect(menu.left).toBeLessThanOrEqual(icon.right + roundingPx);
        expect(menu.right).toBeGreaterThanOrEqual(icon.left - roundingPx);
        expect(menu.top).toBeLessThanOrEqual(icon.bottom + roundingPx);
        expect(menu.bottom).toBeGreaterThanOrEqual(icon.top - roundingPx);
        expect(menu.left).toBeGreaterThanOrEqual(0);
        expect(menu.top).toBeGreaterThanOrEqual(0);
        expect(menu.right).toBeLessThanOrEqual(width);
        expect(menu.bottom).toBeLessThanOrEqual(height);
      },
    );
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
    // The menu's height is capped by the banner's, which the page keeps once a resize observer has
    // measured it (useBannerHeight): none left from another test, and none read before it is kept.
    document.documentElement.style.removeProperty("--banner-height");
    await drawn(shell, async ({ container }) => {
      await expect
        .poll(() => document.documentElement.style.getPropertyValue("--banner-height"))
        .not.toBe("");
      await userEvent.click(page.getByRole("button", { name: "Menu" }));
      const menu = container.querySelector("dialog");
      if (menu === null) {
        throw new Error("the shell's menu does not open");
      }
      // Laid out open before a place is focused: Firefox scrolls a focused place into view in the
      // layout it has, which the opening has not reached yet in the same task.
      await new Promise((resolve) => requestAnimationFrame(resolve));
      const places = [...menu.querySelectorAll<HTMLElement>("ul a, ul button")];
      expect(places.at(-1)?.textContent).toBe("Sign out");
      // A pop-up, no Close of its own (the owner, 2026-11-02; D408 §2).
      expect(menu.querySelector('button[aria-label="Close"]')).toBeNull();
      for (const place of places) {
        place.focus();
        const box = boxOf(place);
        expect(box.top, place.textContent).toBeGreaterThanOrEqual(0);
        expect(box.bottom, place.textContent).toBeLessThanOrEqual(window.innerHeight);
      }
    });
  });

  // The owner's Fire TV, 2026-10-29: "The menu opens up partially scrolled down (I see from Needs a
  // match to Device tests) rather than showing from the top of the list." A menu has no choice to lay
  // over its icon, the page it is on among its places or not.
  it.each(["series", "jobs"] as const)(
    "opens the menu from its top on a Fire TV's Silk, its first place focused, on the %s page",
    async (currentScreen) => {
      await page.viewport(960, 480);
      document.documentElement.style.removeProperty("--banner-height");
      await drawn(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen={currentScreen}
          onSignOut={() => undefined}
          signedInAs={{ name: "owner", deviceName: "Silk on Fire TV" }}
          showsDeveloperPages={true}
          showsDeviceTests={true}
          onSwitchHousehold={() => undefined}
        >
          <p>screen</p>
        </AppShell>,
        async ({ container }) => {
          await expect
            .poll(() => document.documentElement.style.getPropertyValue("--banner-height"))
            .not.toBe("");
          await userEvent.click(page.getByRole("button", { name: "Menu" }));
          const menu = container.querySelector("dialog");
          await new Promise((resolve) => requestAnimationFrame(resolve));
          const places = menu?.querySelector<HTMLElement>(".list-box");
          const first = menu?.querySelector<HTMLElement>("ul a, ul button");
          expect(places?.scrollTop).toBe(0);
          expect(document.activeElement).toBe(first);
          expect(boxOf(menu).top).toBeGreaterThanOrEqual(0);
          expect(boxOf(first).top).toBeGreaterThanOrEqual(boxOf(places).top);
        },
      );
    },
  );

  // The owner, 2026-10-28: "the drop-down menu should be modal".
  it("takes a press outside the open menu itself, closing it and pressing nothing under it", async () => {
    await page.viewport(960, 480);
    const services = testServices("token", "/films");
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={() => undefined}
          signedInAs={{ name: "owner", deviceName: undefined }}
        >
          <p>screen</p>
        </AppShell>,
        services,
      ),
    );
    try {
      await userEvent.click(page.getByRole("button", { name: "Menu" }));
      const series = boxOf(
        mounted.container.querySelector('.app-shell__watching a[href="/series"]'),
      );
      const dialog = mounted.container.querySelector("dialog");
      // What a press at the Series link's place reaches is the menu's backdrop, its own dialog.
      const pressed = document.elementFromPoint(
        series.left + series.width / 2,
        series.top + series.height / 2,
      );
      expect(pressed).toBe(dialog);
      if (pressed instanceof HTMLElement) {
        pressed.dispatchEvent(
          new MouseEvent("click", {
            bubbles: true,
            clientX: series.left + series.width / 2,
            clientY: series.top + series.height / 2,
          }),
        );
      }
      await expect.poll(() => mounted.container.querySelector("dialog")).toBeNull();
      expect(services.navigation.route()).toEqual({ screen: "films" });
    } finally {
      mounted.unmount();
      await page.viewport(1280, 720);
    }
  });

  // The owner, 2026-10-30: "Selecting 'sign out' should open a confirmation." A remote answers it as
  // any other modal: focus in the question, OK on its Sign out, and Back (Escape) declining it.
  it("asks before it signs out, focus in the question, Escape keeping the person in", async () => {
    let signedOut = 0;
    await drawn(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={false}
        currentScreen="films"
        onSignOut={() => {
          signedOut += 1;
        }}
        signedInAs={{ name: "owner", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>,
      async (mounted) => {
        const asked = (): Element | null =>
          mounted.container.querySelector(".app-shell__sign-out-asked");
        await userEvent.click(page.getByRole("button", { name: "Menu" }));
        await userEvent.click(page.getByRole("button", { name: "Sign out" }));
        await expect.poll(asked).not.toBeNull();
        expect(asked()?.contains(document.activeElement)).toBe(true);
        await userEvent.keyboard("{Escape}");
        await expect.poll(asked).toBeNull();
        expect(signedOut).toBe(0);
        await userEvent.click(page.getByRole("button", { name: "Menu" }));
        await userEvent.click(page.getByRole("button", { name: "Sign out" }));
        await expect.poll(asked).not.toBeNull();
        expect(document.activeElement?.textContent).toBe("Sign out");
        await userEvent.keyboard("{Enter}");
        await vi.waitFor(() => {
          expect(signedOut).toBe(1); // Firefox answers the key after a turn
        });
      },
    );
  });
});
