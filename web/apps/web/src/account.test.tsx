// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Account } from "./account.tsx";
import {
  click,
  mount,
  refusal,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The account page for a session that may, or may not, change the account's security. */
async function accountOf(
  canManageAccount: boolean,
  ownLink: Answer = refusal(404, "notFound"),
): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <Account />,
      servedBy({
        "GET /me/devices": [200, { devices: [television()] }],
        "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false, canManageAccount }],
        "GET /me/own-link": ownLink,
        "GET /me/preferences": [
          200,
          { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn: true },
        ],
        "GET /libraries": [200, { libraries: [] }],
      }),
    ),
  );
  return mounted.container;
}

describe("your account", () => {
  it("is ordered by how often each part is used, Testing last (ui-audit-2026-11-04.md §3)", async () => {
    const page = await accountOf(true);
    await settleUntil(() => page.querySelector("#playing-heading") !== null);
    const headings = [...page.querySelectorAll("h2")].map((heading) => heading.textContent);
    expect(headings).toEqual([
      "Activate a device",
      "Playing",
      "Change password",
      "Your devices",
      "Testing",
    ]);
  });

  it("places the sections the audit left: from away after the password, the link after home (D419 §2)", async () => {
    mounted = await mount(
      withServices(
        <Account />,
        servedBy({
          "GET /me/devices": [200, { devices: [television()] }],
          "GET /me": [
            200,
            {
              id: 1,
              name: "owner",
              role: "admin",
              isAdmin: true,
              canManageAccount: true,
              adminAway: "never",
              homeAddresses: ["https://192-168-1-250.k7xq2.orpheum-core.com:8443"],
            },
          ],
          "GET /me/own-link": [200, { secret: "a-links-secret", expiresAt: 2 }],
          "GET /me/preferences": [
            200,
            { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn: true },
          ],
          "GET /libraries": [200, { libraries: [] }],
        }),
      ),
    );
    const page = mounted.container;
    await settleUntil(() => page.querySelector("#your-link-heading") !== null);
    const headings = [...page.querySelectorAll("h2")].map((heading) => heading.textContent);
    expect(headings).toEqual([
      "Activate a device",
      "Playing",
      "Change password",
      "Administering from away from home",
      "Your server at home",
      "Your link",
      "Your devices",
      "Testing",
    ]);
  });

  it("puts a limited session's note at the top, under who is signed in (D419 §2)", async () => {
    const page = await accountOf(false);
    await settleUntil(() => page.querySelector("#playing-heading") !== null);
    const who = page.querySelector(".account__who");
    expect(who?.nextElementSibling?.tagName).toBe("P");
    expect(who?.nextElementSibling?.nextElementSibling?.querySelector("h2")?.textContent).toBe(
      "Playing",
    );
  });

  it("says who is signed in, under its heading (the owner, 2026-10-30)", async () => {
    const page = await accountOf(true);
    await settleUntil(() => page.querySelector(".account__who") !== null);
    expect(page.querySelector(".account__who")?.textContent).toBe("Signed in as owner");
  });

  it("offers a session that may not change the account's security only what it may do, and says where to do the rest (D292 §4)", async () => {
    const limited = await accountOf(false);
    expect(limited.textContent).toContain("Living room TV");
    expect(limited.textContent).not.toContain("Change password");
    expect(limited.textContent).not.toContain("Activate a device");
    expect(limited.textContent).not.toContain("Remove");
    expect(limited.textContent).not.toContain("Sign out everywhere");
    expect(limited.textContent).toContain("not change your password");
    mounted?.unmount();
    const full = await accountOf(true);
    for (const offered of ["Change password", "Activate a device", "Remove"]) {
      expect(full.textContent).toContain(offered);
    }
    expect(full.textContent).not.toContain("not change your password");
  });

  it("offers the person's own autoplay to any session of theirs (plans/up-next.md)", async () => {
    for (const canManage of [false, true]) {
      const page = await accountOf(canManage);
      expect(page.textContent).toContain("Play the next episode automatically");
      mounted?.unmount();
    }
    mounted = undefined;
  });

  it("shows the link the person registered by, to open or scan on another device, and nothing without one (D292 §4)", async () => {
    const link =
      "https://app.example/register#k7xq2.a-links-secret.Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
    const page = await accountOf(true, [200, { secret: "a-links-secret", link, expiresAt: 2 }]);
    expect(page.querySelector("output")?.textContent).toBe(link);
    expect(page.querySelector('img[alt="Your link, as a QR code"]')).not.toBeNull();
    mounted?.unmount();
    // On the server's own origin, before a discovery claims it, the link is the origin's.
    const home = await accountOf(true, [200, { secret: "a-links-secret", expiresAt: 2 }]);
    expect(home.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/register#a-links-secret",
    );
    mounted?.unmount();
    const none = await accountOf(true);
    expect(none.textContent).not.toContain("Your link");
  });

  it("shows everyone the server's home address, as a link and a QR code, once it has one (D292 §6)", async () => {
    const home = "https://192-168-1-250.k7xq2.orpheum-core.com:8443";
    mounted = await mount(
      withServices(
        <Account />,
        servedBy({
          "GET /me/devices": [200, { devices: [] }],
          "GET /me": [
            200,
            {
              id: 2,
              name: "partner",
              role: "user",
              isAdmin: false,
              canManageAccount: true,
              homeAddresses: [home],
            },
          ],
          "GET /me/own-link": refusal(404, "notFound"),
          "GET /me/preferences": refusal(404, "notFound"),
          "GET /libraries": [200, { libraries: [] }],
        }),
      ),
    );
    const section = mounted.container.querySelector(
      'section[aria-labelledby="home-address-heading"]',
    );
    expect(section?.querySelector("a")?.getAttribute("href")).toBe(home);
    expect(
      section?.querySelector('img[alt="The server\'s home address, as a QR code"]'),
    ).not.toBeNull();
    mounted.unmount();
    const without = await accountOf(true);
    expect(without.querySelector('section[aria-labelledby="home-address-heading"]')).toBeNull();
  });
});

describe("an admin's administering from away (D302)", () => {
  it("is offered to an admin alone, off by default, and turned on from home", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <Account />,
        servedBy(
          {
            "GET /me/devices": [200, { devices: [] }],
            "GET /me": [
              [
                200,
                {
                  id: 1,
                  name: "owner",
                  role: "admin",
                  isAdmin: true,
                  canManageAccount: true,
                  adminAway: "never",
                },
              ],
              [
                200,
                {
                  id: 1,
                  name: "owner",
                  role: "admin",
                  isAdmin: true,
                  canManageAccount: true,
                  adminAway: "always",
                },
              ],
            ],
            "GET /me/own-link": refusal(404, "notFound"),
            "GET /me/preferences": refusal(404, "notFound"),
            "GET /libraries": [200, { libraries: [] }],
            "PUT /me/admin-away": [204, {}],
          },
          heard,
        ),
      ),
    );
    const box = mounted.container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    expect(box?.checked).toBe(false);
    await click(box);
    // The page reads /me again once the server kept it, and draws the setting anew from its answer.
    const checkedNow = (): boolean | undefined =>
      mounted?.container.querySelector<HTMLInputElement>('input[type="checkbox"]')?.checked;
    await settleUntil(() => checkedNow() === true);
    expect(checkedNow()).toBe(true);
    expect(heard.find((request) => request.methodAndPath === "PUT /me/admin-away")?.body).toEqual({
      adminAway: "always",
    });
  });

  it("is not offered to a person who is no admin", async () => {
    const person = await accountOf(true);
    expect(person.textContent).not.toContain("Administering from away from home");
  });
});
