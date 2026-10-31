// SPDX-License-Identifier: MPL-2.0

/**
 * The realm sign-in, its banner and the loader's household list as a real browser lays them out
 * (vitest's browser mode): the browser tier's realm test checked these after a whole sign-in
 * through a discovery, where a stylesheet alone decides them (the owner, 2026-10-18).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";
import { userEvent } from "vitest/browser";

import { AppShell } from "./app-shell.tsx";
import { guardFormsOk } from "./form-ok.ts";
import { HouseholdList } from "./loader-at-loader-origin.tsx";
import { RealmSignIn } from "./realm-sign-in.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("the realm's pages, laid out by a real browser", () => {
  it("sets the remember box in one row beside its words", async () => {
    const mounted = await mount(
      <RealmSignIn
        rememberedRealm={undefined}
        isSending={false}
        problem={undefined}
        onSubmit={ignore}
      />,
    );
    const check = mounted.container.querySelector(".credentials-form__check");
    expect(check === null ? "" : getComputedStyle(check).display).toBe("flex");
    mounted.unmount();
  });

  // The owner's Fire TV, 2026-10-30: OK on "remember my realm" signed in at once. A real Enter on a
  // checkbox in a form is the browser's implicit submission; the loader's guard (form-ok.ts) ticks the
  // box and submits nothing.
  it("ticks the remember box on OK, as the loader's page guards it, and submits nothing", async () => {
    const submitted: boolean[] = [];
    const stopGuard = guardFormsOk(
      { target: window, focused: () => document.activeElement },
      () => undefined,
    );
    const mounted = await mount(
      <RealmSignIn
        rememberedRealm={undefined}
        isSending={false}
        problem={undefined}
        onSubmit={(typed) => submitted.push(typed.isRemembered)}
      />,
    );
    try {
      // Filled in, as a person does before the box: an empty required field holds the submit back.
      for (const field of mounted.container.querySelectorAll<HTMLInputElement>(
        'input:not([type="checkbox"])',
      )) {
        await userEvent.type(field, "karlnas1");
      }
      const box = mounted.container.querySelector<HTMLInputElement>('input[type="checkbox"]');
      box?.focus();
      await userEvent.keyboard("{Enter}");
      await new Promise((resolve) => setTimeout(resolve, 50));
      expect(box?.checked).toBe(true);
      expect(submitted).toEqual([]);
    } finally {
      mounted.unmount();
      stopGuard();
    }
  });

  it("keeps the realm in the banner on one line, however narrow the banner (D297 §5)", async () => {
    const mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={{ name: "owner", deviceName: undefined }}
          realm="karlnas"
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    const realm = mounted.container.querySelector(".app-shell__realm");
    expect(realm === null ? "" : getComputedStyle(realm).whiteSpace).toBe("nowrap");
    mounted.unmount();
  });

  it("lists each household with a small Forget at its row's end, level with its name (the owner, 2026-10-17)", async () => {
    const mounted = await mount(
      <HouseholdList
        listed={[{ coreId: "core1", fingerprint: "f", realm: "karlnas", name: "", lastUsedAt: 0 }]}
        originOf={() => "https://core1.example"}
        onForget={ignore}
        onJoin={ignore}
      />,
    );
    const { container } = mounted;
    const list = container.querySelector(".loader-households")?.getBoundingClientRect();
    const name = container.querySelector(".loader-households a")?.getBoundingClientRect();
    const forget = container.querySelector(".loader-households button")?.getBoundingClientRect();
    if (list === undefined || name === undefined || forget === undefined) {
      throw new Error("the household list is drawn nowhere");
    }
    expect(forget.width).toBeLessThan(list.width / 2);
    expect(Math.abs(forget.top + forget.height / 2 - (name.top + name.height / 2))).toBeLessThan(3);
    mounted.unmount();
  });
});
