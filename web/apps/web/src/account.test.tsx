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
  ownLink: Answer = refusal(404, "not_found"),
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
          "GET /me/own-link": refusal(404, "not_found"),
          "GET /me/preferences": refusal(404, "not_found"),
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
            "GET /me/own-link": refusal(404, "not_found"),
            "GET /me/preferences": refusal(404, "not_found"),
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

describe("the libraries a person leaves out of Films and Series (D349 §8)", () => {
  /** Two libraries that gather into Films and Series, and one other-videos library beside them. */
  const libraries = [
    {
      id: 1,
      name: "Films",
      kind: "movies",
      metadataSource: "tmdb",
      changeDetection: "watch",
      isWritable: true,
      createdAt: 0,
    },
    {
      id: 2,
      name: "Kids' films",
      kind: "movies",
      metadataSource: "tmdb",
      changeDetection: "watch",
      isWritable: true,
      createdAt: 0,
    },
    {
      id: 3,
      name: "Home movies",
      kind: "other",
      metadataSource: "nfo",
      changeDetection: "watch",
      isWritable: true,
      createdAt: 0,
    },
  ];

  it("offers every library Films or Series gathers, none an other library, checked until left out", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <Account />,
        servedBy(
          {
            "GET /me/devices": [200, { devices: [] }],
            "GET /me": [
              200,
              { id: 1, name: "owner", role: "user", isAdmin: false, canManageAccount: true },
            ],
            "GET /me/own-link": refusal(404, "not_found"),
            "GET /me/preferences": [
              200,
              {
                isDialogueBoostOn: false,
                isLevellingOn: true,
                isAutoplayOn: true,
                leftOutLibraryIds: [2],
              },
            ],
            "GET /libraries": [200, { libraries }],
            "PUT /me/preferences": [204, {}],
          },
          heard,
        ),
      ),
    );
    await settleUntil(() => mounted?.container.textContent.includes("Kids' films") === true);
    expect(mounted.container.textContent).not.toContain("Home movies");
    const boxes = [
      ...mounted.container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'),
    ].filter((box) => box.closest("li") !== null);
    const filmsBox = boxes.find((box) => box.closest("li")?.textContent === "Films");
    const kidsBox = boxes.find((box) => box.closest("li")?.textContent === "Kids' films");
    expect(filmsBox?.checked).toBe(true);
    expect(kidsBox?.checked).toBe(false);
    await click(kidsBox);
    await settleUntil(() =>
      heard.some((request) => request.methodAndPath === "PUT /me/preferences"),
    );
    const put = heard.find((request) => request.methodAndPath === "PUT /me/preferences");
    expect(put?.body).toEqual({ leftOutLibraryIds: [] });
  });

  it("is shown to no one who reaches no library Films or Series gather", async () => {
    const page = await accountOf(true);
    expect(page.textContent).not.toContain("Films and Series libraries");
  });
});
