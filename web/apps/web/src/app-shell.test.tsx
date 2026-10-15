// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { click, fire, mount } from "./test-dom.tsx";
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
      /<header class="app-shell__banner"><p class="app-shell__title"><a href="\/films" class="app-shell__home"><img class="app-shell__logo" src="[^"]*orpheum-logo\.svg" alt="Orpheum"\/><\/a><\/p>/u,
    );
    expect(markup).toContain('<li aria-current="page"><a href="/films">Films</a></li>');
    expect(markup).toContain('<a href="/watch-together">Watch Together</a>');
    expect(markup).not.toContain("/libraries");
    expect(markup).not.toContain("/jobs");
    expect(markup).toContain("partner on Firefox on Mac");
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

  it("takes a person home, to the films, when the logo is clicked", async () => {
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
    expect(services.navigation.route()).toEqual({ screen: "films" });
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
    expect(watching).toContain('href="/watch-together"');
    expect(watching).not.toContain('href="/jobs"');
    // One menu, behind its icon, marked as where one is (the owner, 2026-10-11).
    expect(markup.match(/<details/gu)).toHaveLength(1);
    // The status tray sits just before it, the two together at the banner's end at every width.
    expect(markup).toMatch(
      /<div class="app-shell__end"><section class="status-tray".*?<\/section><details/u,
    );
    expect(markup).toMatch(
      /<details class="app-shell__menu app-shell__menu--current"><summary aria-label="Menu" title="Menu"><svg/u,
    );
    expect(markup).toMatch(
      /<ul><li class="app-shell__who">karl<\/li><li class="app-shell__section">Manage<\/li>.*href="\/jobs".*class="app-shell__divider".*href="\/this-device".*href="\/account".*href="\/about".*Sign out/u,
    );
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
});
