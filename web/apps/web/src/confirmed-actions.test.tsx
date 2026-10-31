// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { People } from "./people.tsx";
import { PersonPage } from "./person.tsx";
import { Settings } from "./settings.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/*
 * What cannot be undone is red and asked again (D278 §4, the owner, 2026-10-12: "red button +
 * confirm action is a general pattern that will occur often"): the first press sends nothing, the
 * second does.
 */

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const roles = {
  capabilities: ["administer", "edit_metadata", "host_watch_together"],
  roles: [
    {
      id: 1,
      name: "admin",
      isBuiltin: true,
      capabilities: ["administer"],
      isEveryLibrary: true,
      libraries: [],
      holderCount: 1,
    },
    {
      id: 2,
      name: "user",
      isBuiltin: true,
      capabilities: ["host_watch_together"],
      isEveryLibrary: false,
      libraries: [{ libraryId: 1 }, { libraryId: 2 }],
      holderCount: 1,
    },
    {
      id: 3,
      name: "friends",
      isBuiltin: false,
      capabilities: [],
      isEveryLibrary: false,
      libraries: [],
      holderCount: 0,
    },
  ],
};
const libraries = {
  libraries: [
    {
      id: 1,
      name: "Films",
      kind: "movies",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
    {
      id: 2,
      name: "Private",
      kind: "movies",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
  ],
};

/** Presses label, expecting nothing sent, then confirmLabel, expecting methodAndPath sent. */
async function pressAndConfirm(
  heard: Heard[],
  press: { readonly label: string; readonly confirmLabel: string; readonly methodAndPath: string },
): Promise<void> {
  if (mounted === undefined) {
    throw new Error("nothing mounted");
  }
  const button = buttonWithText(mounted.container, press.label);
  expect(button?.className).toBe("irreversible-outline");
  await click(button);
  expect(heard.map((each) => each.methodAndPath)).not.toContain(press.methodAndPath);
  await click(buttonWithText(mounted.container, press.confirmLabel));
  expect(heard.map((each) => each.methodAndPath)).toContain(press.methodAndPath);
}

it("deletes a role, and withdraws an invitation, only when asked again", async () => {
  const heard: Heard[] = [];
  const invitation = { id: 7, role: "friends", adminNote: "my cousin", issuedAt: 1, expiresAt: 2 };
  mounted = await mount(
    withServices(
      <People />,
      servedBy(
        {
          "GET /users": [200, { accounts: [] }],
          "GET /roles": [200, roles],
          "GET /libraries": [200, libraries],
          "GET /invitations": [200, { invitations: [invitation] }],
          "DELETE /roles/3": [204, null],
          "DELETE /invitations/7": [204, null],
        },
        heard,
      ),
    ),
  );
  await pressAndConfirm(heard, {
    label: "Delete the role",
    confirmLabel: "Delete it",
    methodAndPath: "DELETE /roles/3",
  });
  await pressAndConfirm(heard, {
    label: "Withdraw",
    confirmLabel: "Withdraw it",
    methodAndPath: "DELETE /invitations/7",
  });
});

it("makes a person a new link only when asked again, their old one stopping", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <PersonPage userId={5} />,
      servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
          "GET /users/5": [
            200,
            {
              account: { id: 5, name: "cousin", role: "user", hasPassword: true },
              watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
            },
          ],
          "GET /users/5/devices": [200, { devices: [] }],
          "GET /roles": [200, roles],
          "POST /users/5/registration-invitation": [201, { secret: "s", expiresAt: 2 }],
        },
        heard,
      ),
    ),
  );
  expect(mounted.container.textContent).not.toContain("keeps working until one of them is used");
  await pressAndConfirm(heard, {
    label: "New link",
    confirmLabel: "Make a new link",
    methodAndPath: "POST /users/5/registration-invitation",
  });
});

it("removes a stored secret only when asked again", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <Settings />,
      servedBy(
        {
          "GET /secrets": [
            200,
            {
              secrets: [
                { name: "tmdb_api_key", description: "TMDB", isSet: true, needsReentry: false },
              ],
            },
          ],
          "GET /config": [200, { settings: [] }],
          "GET /system/update": [200, { version: "v0.3.1", checkedAt: 0 }],
          "DELETE /secrets/tmdb_api_key": [204, null],
        },
        heard,
      ),
    ),
  );
  // The bin asks first; only its question's red button removes the key.
  await click(mounted.container.querySelector('button[aria-label="Remove tmdb_api_key"]'));
  expect(heard.map((each) => each.methodAndPath)).not.toContain("DELETE /secrets/tmdb_api_key");
  const confirm = buttonWithText(mounted.container, "Remove it");
  expect(confirm?.className).toBe("irreversible");
  await click(confirm);
  expect(heard.map((each) => each.methodAndPath)).toContain("DELETE /secrets/tmdb_api_key");
});

it("ends a person's own link only when asked again, saying until when it led them back (D292 §4)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <PersonPage userId={5} />,
      servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
          "GET /users/5": [
            200,
            {
              account: { id: 5, name: "cousin", role: "user", hasPassword: true },
              watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
              ownLinkExpiresAt: Date.UTC(2027, 0, 12),
            },
          ],
          "GET /users/5/devices": [200, { devices: [] }],
          "GET /roles": [200, roles],
          "DELETE /users/5/own-link": [204, null],
        },
        heard,
      ),
    ),
  );
  expect(mounted.container.textContent).toContain("leads them back to this server until");
  await pressAndConfirm(heard, {
    label: "End that link…",
    confirmLabel: "End it",
    methodAndPath: "DELETE /users/5/own-link",
  });
});

it("shows no own link for a person who has none", async () => {
  mounted = await mount(
    withServices(
      <PersonPage userId={5} />,
      servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
          "GET /users/5": [
            200,
            {
              account: { id: 5, name: "cousin", role: "user", hasPassword: true },
              watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
            },
          ],
          "GET /users/5/devices": [200, { devices: [] }],
          "GET /roles": [200, roles],
        },
        [],
      ),
    ),
  );
  expect(mounted.container.textContent).toContain("cousin");
  expect(mounted.container.textContent).not.toContain("leads them back");
  expect(buttonWithText(mounted.container, "End that link…")).toBeUndefined();
});
