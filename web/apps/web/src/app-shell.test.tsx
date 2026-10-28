// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

import { act } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { buttonWithText, click, fire, mount, settleUntil } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("AppShell", () => {
  it("renders a banner with the product name and the menu, and one main landmark", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={{ name: "partner", deviceName: "Firefox on Mac" }}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    expect(markup.match(/<main/gu)).toHaveLength(1);
    expect(markup).toMatch(
      /<header class="app-shell__banner" data-focus-container="" data-focus-covers-top=""><p class="app-shell__title"><a href="\/" class="app-shell__home"><picture><source media="\(max-width: 599px\)" srcSet="\/favicon\.svg"\/><img class="app-shell__logo" src="[^"]*orpheum-logo\.svg" alt="Orpheum"\/><\/picture><\/a><\/p>/u,
    );
    expect(markup).toContain('<li aria-current="page"><a href="/films">Films</a></li>');
    // No Watch Together or Cast in the top bar: their signs in the status tray (D364 §12).
    expect(markup).not.toContain('<a href="/watch-together">Watch Together</a>');
    expect(markup).toContain('href="/watch-together" class="status-tray__button');
    expect(markup).toContain('href="/casting" class="status-tray__button');
    expect(markup).not.toContain("/libraries");
    expect(markup).not.toContain("/jobs");
    expect(markup).toContain("partner on Firefox on Mac");
  });

  it("tells an admin away from home where they administer, under Manage, in place of its places (D297 §6)", () => {
    const shell = (administersFromHomeAt: string | undefined): string =>
      renderToStaticMarkup(
        withServices(
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
        ),
      );
    const away = shell("https://192-168-1-250.k7xq2.orpheum-core.com:8443");
    expect(away).toContain("Manage");
    expect(away).toContain(
      'You administer this server from home, at <a href="https://192-168-1-250.k7xq2.orpheum-core.com:8443">',
    );
    expect(away).not.toContain("/libraries");
    expect(shell("")).toContain("on its own page or the discovery&#x27;s page at home");
    // A person who is no admin is told nothing of administering.
    expect(shell(undefined)).not.toContain("You administer this server");
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

  it("shows the admin's destinations to an admin", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="about"
          onSignOut={ignore}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
      ),
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

  it("keeps the places for watching in view, and the admin's, under Manage, and the person's in one menu at the end", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="jobs"
          onSignOut={ignore}
          signedInAs={{ name: "karl", deviceName: undefined }}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    const watching = /<ul class="app-shell__watching">(.*?)<\/ul>/u.exec(markup)?.[1] ?? "";
    expect(watching).toContain('href="/films"');
    expect(watching).not.toContain('href="/watch-together"');
    expect(watching).not.toContain('href="/jobs"');
    // One menu, behind its icon, marked as where one is (the owner, 2026-10-11).
    expect(markup.match(/<details/gu)).toHaveLength(1);
    // The status tray sits just before it, the two together at the banner's end at every width.
    expect(markup).toMatch(
      /<div class="app-shell__end"><section class="status-tray".*?<\/section><details/u,
    );
    expect(markup).toMatch(
      /<details class="app-shell__menu app-shell__menu--current" data-focus-container=""><summary aria-label="Menu" title="Menu"><svg/u,
    );
    expect(markup).toMatch(
      /<ul><li class="app-shell__who">karl<\/li><li class="app-shell__section">Manage<\/li>.*href="\/jobs".*class="app-shell__divider".*href="\/this-device".*href="\/account".*href="\/about".*Sign out/u,
    );
  });

  it("offers the pages for development under their own heading only when asked to (the owner, 2026-10-17)", () => {
    const markupWith = (showsDeveloperPages: boolean): string =>
      renderToStaticMarkup(
        withServices(
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
        ),
      );
    expect(markupWith(true)).toMatch(
      /<li class="app-shell__section">Development<\/li><li><a href="\/ui-gallery"[^>]*>UI gallery<\/a><\/li><li class="app-shell__divider".*href="\/this-device"/u,
    );
    expect(markupWith(false)).not.toContain("Development");
    expect(markupWith(false)).not.toContain('href="/ui-gallery"');
  });

  it("offers Device tests in the menu, to everyone, only while the server offers them (the owner, 2026-10-25, 2026-10-28)", () => {
    const shellWith = (showsDeviceTests: boolean | undefined): string => {
      const markup = renderToStaticMarkup(
        withServices(
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
        ),
      );
      return markup;
    };
    const watching = (markup: string): string =>
      /<ul class="app-shell__watching">(.*?)<\/ul>/u.exec(markup)?.[1] ?? "";
    // On a phone the row of places ran past the window: the device tests are the menu's.
    expect(shellWith(true)).toMatch(
      /<details class="app-shell__menu[^]*href="\/lab"[^>]*>Device tests<\/a>/u,
    );
    expect(watching(shellWith(true))).not.toContain('href="/lab"');
    expect(shellWith(false)).not.toContain('href="/lab"');
    expect(shellWith(undefined)).not.toContain('href="/lab"');
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
    await click(buttonWithText(shown.container, "Switch household"));
    shown.unmount();
    expect(switched).toBe(1);
  });

  it("offers no Manage to someone who is not an admin", () => {
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
    expect(markup).not.toContain("Manage");
    expect(markup).not.toContain("app-shell__divider");
    expect(markup).toContain('<summary aria-label="Menu" title="Menu">');
  });

  it("closes a menu when one of its places is chosen, on a press outside it, and on Escape", async () => {
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
    const manage = mounted.container.querySelector("details");
    if (manage === null) {
      throw new Error("no Manage menu");
    }
    manage.open = true;
    await click(manage.querySelector('a[href="/jobs"]'));
    expect(manage.open).toBe(false);
    manage.open = true;
    await fire(
      mounted.container.querySelector("main"),
      new Event("pointerdown", { bubbles: true }),
    );
    expect(manage.open).toBe(false);
    manage.open = true;
    await fire(manage, new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(manage.open).toBe(false);
    manage.open = true;
    await fire(manage.querySelector("summary"), new Event("pointerdown", { bubbles: true }));
    expect(manage.open).toBe(true);
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
    const menu = mounted.container.querySelector("details");
    if (menu === null) {
      throw new Error("the shell has no menu");
    }
    menu.open = true;
    const place = menu.querySelector("ul a");
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
    expect(menu.open).toBe(false);
    expect(document.activeElement).toBe(menu.querySelector("summary"));
    expect(services.navigation.route()).toEqual({ screen: "about" });
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
