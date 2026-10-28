// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

import { act, type JSX } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { createNavigation } from "./navigation.ts";
import { buttonNamed, buttonWithText, click, fire, mount, settleUntil } from "./test-dom.tsx";
import { fakePlace, testServices, withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

/**
 * What a shell draws with its menu open, whose places are drawn only while it is (drop-down.tsx):
 * the whole page, the open list among it.
 */
async function drawnWithMenuOpen(shell: JSX.Element): Promise<string> {
  const mounted = await mount(withServices(shell));
  await click(mounted.container.querySelector(".app-shell__menu-button"));
  const markup = mounted.container.innerHTML;
  mounted.unmount();
  return markup;
}

describe("AppShell", () => {
  it("renders a banner with the product name and the menu, and one main landmark", async () => {
    const shell = (
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen="films"
        onSignOut={ignore}
        signedInAs={{ name: "partner", deviceName: "Firefox on Mac" }}
      >
        <p>screen</p>
      </AppShell>
    );
    const markup = renderToStaticMarkup(withServices(shell));
    const open = await drawnWithMenuOpen(shell);
    expect(markup.match(/<main/gu)).toHaveLength(1);
    expect(markup).toMatch(
      /<header class="app-shell__banner" data-focus-container="" data-focus-covers-top=""><nav class="app-shell__nav" aria-label="Main"><div class="app-shell__places"><p class="app-shell__title"><a href="\/" class="app-shell__home"><picture><source media="\(max-width: 599px\)" srcSet="\/favicon\.svg"\/><img class="app-shell__logo" src="[^"]*orpheum-logo\.svg" alt="Orpheum"\/><\/picture><\/a><\/p>/u,
    );
    // A place for watching is its icon, named for a screen reader and a pointer's hover (2026-10-28).
    expect(markup).toContain(
      '<li aria-current="page"><a href="/films" class="app-shell__place-icon" aria-label="Films" title="Films"><svg',
    );
    // No Watch Together or Cast in the top bar: their signs in the status tray (D364 §12).
    expect(markup).not.toContain('<a href="/watch-together">Watch Together</a>');
    expect(markup).toContain('href="/watch-together" class="status-tray__button');
    expect(markup).toContain('href="/casting" class="status-tray__button');
    expect(open).not.toContain("/libraries");
    expect(open).not.toContain("/jobs");
    expect(open).toContain("partner on Firefox on Mac");
  });

  it("tells an admin away from home where they administer, under Manage, in place of its places (D297 §6)", async () => {
    const shell = async (administersFromHomeAt: string | undefined): Promise<string> =>
      await drawnWithMenuOpen(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={{ name: "karl", deviceName: undefined }}
          administersFromHomeAt={administersFromHomeAt}
        >
          <p>screen</p>
        </AppShell>,
      );
    const away = await shell("https://192-168-1-250.k7xq2.orpheum-core.com:8443");
    expect(away).toContain("Manage");
    expect(away).toContain(
      'You administer this server from home, at <a href="https://192-168-1-250.k7xq2.orpheum-core.com:8443">',
    );
    expect(away).not.toContain("/libraries");
    expect(await shell("")).toContain("on its own page or the discovery's page at home");
    // A person who is no admin is told nothing of administering.
    expect(await shell(undefined)).not.toContain("You administer this server");
  });

  it("shows the realm its person signed in to in the banner, and none where there is none (D297 §5)", () => {
    const shell = (realm: string | undefined): string =>
      renderToStaticMarkup(
        withServices(
          <AppShell
            isAdmin={false}
            canHostWatchTogether={false}
            currentScreen="films"
            onSignOut={ignore}
            signedInAs={{ name: "karl", deviceName: undefined }}
            realm={realm}
          >
            <p>screen</p>
          </AppShell>,
        ),
      );
    expect(shell("karlnas.live")).toContain(
      '<span class="app-shell__realm" title="Signed in to the realm karlnas.live">karlnas.live</span>',
    );
    expect(shell(undefined)).not.toContain("app-shell__realm");
  });

  it("marks home by its logo, underlined there, with no Home entry of its own (the owner, 2026-10-25)", () => {
    const shellAt = (currentScreen: "home" | "films"): string =>
      renderToStaticMarkup(
        withServices(
          <AppShell
            isAdmin={false}
            canHostWatchTogether={true}
            currentScreen={currentScreen}
            onSignOut={ignore}
            signedInAs={{ name: "partner", deviceName: "Firefox on Mac" }}
          >
            <p>screen</p>
          </AppShell>,
        ),
      );
    expect(shellAt("home")).toContain('<a href="/" class="app-shell__home" aria-current="page">');
    expect(shellAt("films")).toContain('<a href="/" class="app-shell__home">');
    expect(shellAt("home")).not.toMatch(/>Home</u);
  });

  it("takes a person home when the logo is clicked", async () => {
    const services = testServices(undefined, "/about");
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="about"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        services,
      ),
    );
    await click(mounted.container.querySelector(".app-shell__home"));
    expect(services.navigation.route()).toEqual({ screen: "home" });
    mounted.unmount();
  });

  it("shows the admin's destinations to an admin", async () => {
    const markup = await drawnWithMenuOpen(
      <AppShell
        isAdmin={true}
        canHostWatchTogether={true}
        currentScreen="about"
        onSignOut={ignore}
        signedInAs={undefined}
      >
        <p>screen</p>
      </AppShell>,
    );
    expect(markup).toContain('href="/libraries"');
    expect(markup).toContain('href="/identify"');
    expect(markup).toContain('href="/settings"');
    expect(markup).toContain('href="/jobs"');
    expect(markup).toContain('href="/system"');
  });

  it("offers Watch Together only to a person whose role may host it", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    expect(markup).not.toContain("/watch-together");
    expect(markup).toContain('href="/films"');
  });

  it("keeps the places for watching in view, and the admin's, under Manage, and the person's in one menu at the end", async () => {
    const shell = (
      <AppShell
        isAdmin={true}
        canHostWatchTogether={true}
        currentScreen="jobs"
        onSignOut={ignore}
        signedInAs={{ name: "karl", deviceName: undefined }}
      >
        <p>screen</p>
      </AppShell>
    );
    const markup = renderToStaticMarkup(withServices(shell));
    const watching = /<ul class="app-shell__watching">(.*?)<\/ul>/u.exec(markup)?.[1] ?? "";
    expect(watching).toContain('href="/films"');
    expect(watching).not.toContain('href="/watch-together"');
    expect(watching).not.toContain('href="/jobs"');
    // One menu, behind its icon, marked as where one is (the owner, 2026-10-11).
    expect(markup.match(/class="app-shell__menu-button"/gu)).toHaveLength(1);
    // The status tray sits just before it, the two together at the banner's end at every width.
    expect(markup).toMatch(
      /<div class="app-shell__end"><section class="status-tray".*?<\/section><div class="app-shell__menu/u,
    );
    // The chooser's drop-down, its button the menu's icon alone, with no chevron (the owner,
    // 2026-10-28: "the only real difference being that there's no extra visual indicator that it's
    // a dropdown").
    expect(markup).toMatch(
      /<div class="app-shell__menu app-shell__menu--current"><button type="button" class="app-shell__menu-button" aria-haspopup="dialog" aria-expanded="false" aria-label="Menu" title="Menu"><svg[^>]*class="icon app-shell__menu-icon"[^]*?<\/svg><\/button><\/div>/u,
    );
    expect(markup).not.toContain("route-chooser__chevron");
    expect(await drawnWithMenuOpen(shell)).toMatch(
      /<ul><li class="app-shell__who">karl<\/li><li class="app-shell__section">Manage<\/li>.*href="\/jobs".*class="app-shell__divider".*href="\/this-device".*href="\/account".*href="\/about".*Sign out/u,
    );
  });

  it("offers the pages for development under their own heading only when asked to (the owner, 2026-10-17)", async () => {
    const markupWith = async (showsDeveloperPages: boolean): Promise<string> =>
      await drawnWithMenuOpen(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
          showsDeveloperPages={showsDeveloperPages}
        >
          <p>screen</p>
        </AppShell>,
      );
    expect(await markupWith(true)).toMatch(
      /<li class="app-shell__section">Development<\/li><li><a href="\/ui-gallery"[^>]*>UI gallery<\/a><\/li><li class="app-shell__divider".*href="\/this-device"/u,
    );
    expect(await markupWith(false)).not.toContain("Development");
    expect(await markupWith(false)).not.toContain('href="/ui-gallery"');
  });

  it("offers Device tests in the menu, to everyone, only while the server offers them (the owner, 2026-10-25, 2026-10-28)", async () => {
    const shellWith = async (showsDeviceTests: boolean | undefined): Promise<string> =>
      await drawnWithMenuOpen(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
          {...(showsDeviceTests === undefined ? {} : { showsDeviceTests })}
        >
          <p>screen</p>
        </AppShell>,
      );
    const watching = (markup: string): string =>
      /<ul class="app-shell__watching">(.*?)<\/ul>/u.exec(markup)?.[1] ?? "";
    // On a phone the row of places ran past the window: the device tests are the menu's.
    expect(await shellWith(true)).toMatch(
      /<div class="app-shell__menu[^]*<dialog[^]*href="\/lab"[^>]*>Device tests<\/a>/u,
    );
    expect(watching(await shellWith(true))).not.toContain('href="/lab"');
    expect(await shellWith(false)).not.toContain('href="/lab"');
    expect(await shellWith(undefined)).not.toContain('href="/lab"');
  });

  it("offers to switch household only while a discovery's loader runs the page (D308 §8)", async () => {
    let switched = 0;
    const shellWith = (onSwitchHousehold: (() => void) | undefined) =>
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
          onSwitchHousehold={onSwitchHousehold}
        >
          <p>screen</p>
        </AppShell>,
      );
    expect(renderToStaticMarkup(shellWith(undefined))).not.toContain("Switch household");
    const shown = await mount(
      shellWith(() => {
        switched += 1;
      }),
    );
    await click(shown.container.querySelector(".app-shell__menu-button"));
    await click(buttonWithText(shown.container, "Switch household"));
    shown.unmount();
    expect(switched).toBe(1);
  });

  it("offers no Manage to someone who is not an admin", async () => {
    const markup = await drawnWithMenuOpen(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={false}
        currentScreen="films"
        onSignOut={ignore}
        signedInAs={undefined}
      >
        <p>screen</p>
      </AppShell>,
    );
    expect(markup).not.toContain("Manage");
    expect(markup).not.toContain("app-shell__divider");
    expect(markup).toContain('aria-label="Menu" title="Menu"');
    expect(markup).toContain(">Sign out</button>");
  });

  it("closes a menu when one of its places is chosen, on a press outside it, by its Close, and on Escape", async () => {
    const services = testServices("token", "/films");
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        services,
      ),
    );
    const button = mounted.container.querySelector(".app-shell__menu-button");
    const list = (): HTMLDialogElement | null => mounted.container.querySelector("dialog");
    await click(button);
    expect(button?.getAttribute("aria-expanded")).toBe("true");
    await click(list()?.querySelector('a[href="/jobs"]'));
    expect(list()).toBeNull();
    expect(services.navigation.route()).toEqual({ screen: "jobs" });
    // A press outside it lands on its backdrop, the page behind being the browser's inert.
    await click(button);
    await fire(list(), new MouseEvent("click", { bubbles: true, clientX: 500, clientY: 500 }));
    expect(list()).toBeNull();
    expect(services.navigation.route()).toEqual({ screen: "jobs" });
    await click(button);
    await click(buttonNamed(mounted.container, "Close"));
    expect(list()).toBeNull();
    await click(button);
    // Escape, which the browser tells a modal dialog as its cancel.
    await fire(list(), new Event("cancel", { cancelable: true }));
    expect(list()).toBeNull();
    expect(services.navigation.route()).toEqual({ screen: "jobs" });
    mounted.unmount();
  });

  it("closes the open menu on a remote's Back, its icon keeping focus, before Back goes anywhere", async () => {
    const services = testServices("token", "/films");
    services.navigation.go({ screen: "about" });
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="about"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        services,
      ),
    );
    const button = mounted.container.querySelector<HTMLElement>(".app-shell__menu-button");
    act(() => {
      button?.focus();
    });
    await click(button);
    const place = mounted.container.querySelector("dialog ul a");
    act(() => {
      if (place instanceof HTMLElement) {
        place.focus();
      }
    });
    await fire(
      place,
      new KeyboardEvent("keydown", {
        key: "Unidentified",
        keyCode: 461,
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
    expect(services.navigation.route()).toEqual({ screen: "about" });
    mounted.unmount();
  });

  // The owner's Fire TV, 2026-10-28: Silk's Back is the history's, never a key; "with the chooser
  // widget I can only exit without choosing by pressing the back button, which actually brings me
  // back a page."
  it("closes the open menu on the browser's own Back, a step of the history, and the page stays", async () => {
    const place = fakePlace("/films");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "about" });
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="about"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        { ...testServices("token"), navigation },
      ),
    );
    await click(mounted.container.querySelector(".app-shell__menu-button"));
    expect(mounted.container.querySelector("dialog")).not.toBeNull();
    await act(async () => {
      place.history.back();
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(navigation.route()).toEqual({ screen: "about" });
    expect(navigation.behind()).toEqual([{ screen: "films" }]);
    // The next Back leaves the page, as it would have with the menu never opened.
    await act(async () => {
      place.history.back();
      await Promise.resolve();
    });
    expect(navigation.route()).toEqual({ screen: "films" });
    mounted.unmount();
  });

  // The owner, 2026-10-28: "moving left/right while in a chooser or menu closes it".
  it("closes the open menu on Left or Right, focus back on its icon, Up and Down moving within it", async () => {
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    const button = mounted.container.querySelector<HTMLElement>(".app-shell__menu-button");
    for (const key of ["ArrowLeft", "ArrowRight"]) {
      act(() => {
        button?.focus();
      });
      await click(button);
      const first = mounted.container.querySelector<HTMLElement>("dialog ul a");
      act(() => {
        first?.focus();
      });
      const arrow = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
      await fire(first, arrow);
      expect(mounted.container.querySelector("dialog"), key).toBeNull();
      expect(arrow.defaultPrevented, key).toBe(true);
      expect(document.activeElement, key).toBe(button);
    }
    mounted.unmount();
  });

  it("says why the browser asks for the local network while it waits on the person, and goes once they answer or dismiss it (W2c item 8)", async () => {
    const asking = "Your browser is asking to reach devices on your network.";
    let tell: (isAsking: boolean) => void = () => undefined;
    const shell = withServices(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={false}
        currentScreen="films"
        onSignOut={ignore}
        signedInAs={undefined}
      >
        <p>screen</p>
      </AppShell>,
      {
        ...testServices(),
        onLocalNetworkAsking: (listener) => {
          tell = listener;
          listener(true);
          return () => {
            tell = () => undefined;
          };
        },
      },
    );
    // Nothing asks where no loader tells of the prompt.
    expect(
      renderToStaticMarkup(
        withServices(
          <AppShell
            isAdmin={false}
            canHostWatchTogether={false}
            currentScreen="films"
            onSignOut={ignore}
            signedInAs={undefined}
          >
            <p>screen</p>
          </AppShell>,
        ),
      ),
    ).not.toContain(asking);
    const shown = await mount(shell);
    await settleUntil(() => shown.container.textContent.includes(asking));
    expect(shown.container.textContent).toContain(asking);
    act(() => {
      tell(false);
    });
    await settleUntil(() => !shown.container.textContent.includes(asking));
    expect(shown.container.textContent).not.toContain(asking);
    act(() => {
      tell(true);
    });
    await settleUntil(() => shown.container.textContent.includes(asking));
    await click(buttonWithText(shown.container, "Dismiss"));
    expect(shown.container.textContent).not.toContain(asking);
    shown.unmount();
  });
});
