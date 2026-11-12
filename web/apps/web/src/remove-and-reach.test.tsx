// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { BrowseView } from "./browse.tsx";
import { Users } from "./users.tsx";
import { UserPage } from "./user.tsx";
import {
  buttonWithText,
  choose,
  click,
  enter,
  fire,
  mount,
  servedBy,
  submit,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import type { Services } from "./services.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const ignore = (): void => undefined;

const roles = {
  capabilities: ["administer"],
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
      capabilities: [],
      isEveryLibrary: false,
      libraries: [],
      holderCount: 1,
    },
    {
      id: 3,
      name: "family",
      isBuiltin: false,
      capabilities: [],
      isEveryLibrary: false,
      libraries: [{ libraryId: 1 }],
      holderCount: 0,
    },
  ],
};

const noLibraryChosen = "This role has no library yet";
const noLibraryTicked = "No library is ticked for this role";

function me(id: number, hasNoLibrary = false): Answers[string] {
  return [
    200,
    {
      id,
      name: "someone",
      role: "admin",
      isAdmin: true,
      canHostWatchTogether: true,
      hasNoLibrary,
    },
  ];
}

/** A person's page, as the admin with id adminId sees it, and the services it reaches. */
async function mountPerson(
  adminId: number,
  heard: Heard[] = [],
): Promise<{ readonly page: Mounted; readonly services: Services }> {
  const services = servedBy(
    {
      "GET /me": me(adminId),
      "GET /users/5": [
        200,
        {
          user: { id: 5, name: "Dan", role: "user", hasPassword: true },
          watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
        },
      ],
      "GET /users/5/devices": [200, { devices: [] }],
      "GET /roles": [200, roles],
      "DELETE /users/5": [204, null],
    },
    heard,
  );
  return { page: await mount(withServices(<UserPage userId={5} />, services)), services };
}

it("warns when the role chosen for someone new, or ticked for nobody, reaches no library", async () => {
  const answers = {
    "GET /users": [200, { users: [] }],
    "GET /roles": [200, roles],
    "GET /libraries": [200, { libraries: [] }],
    "GET /invitations": [200, { invitations: [] }],
  } as const;
  mounted = await mount(withServices(<Users />, servedBy(answers)));
  await click(buttonWithText(mounted.container, "Invite Someone"));
  const invitation = buttonWithText(mounted.container, "Make their link")?.closest("form");
  expect(invitation?.textContent).toContain(noLibraryChosen);
  await choose(invitation?.querySelector("select"), "family");
  expect(invitation?.textContent).not.toContain(noLibraryChosen);
});

it("marks a role that reaches no library in Roles, and its dialog says what to do", async () => {
  const answers = {
    "GET /users": [200, { users: [] }],
    "GET /roles": [200, roles],
    "GET /libraries": [200, { libraries: [] }],
    "GET /invitations": [200, { invitations: [] }],
  } as const;
  mounted = await mount(withServices(<Users />, servedBy(answers)));
  const rowOf = (name: string): Element | undefined =>
    [...(mounted?.container.querySelectorAll("li.grid-list__row") ?? [])].find(
      (row) => row.querySelector("button")?.textContent.includes(name) === true,
    );
  expect(rowOf("Can watch")?.querySelectorAll("button")).toHaveLength(2);
  expect(rowOf("Administrator")?.querySelectorAll("button")).toHaveLength(1);
  await click(rowOf("Can watch")?.querySelector("button") ?? null);
  expect(mounted.container.querySelector("dialog")?.textContent).toContain(noLibraryTicked);
  mounted.unmount();
  mounted = await mount(withServices(<Users />, servedBy(answers)));
  await click(rowOf("Administrator")?.querySelector("button") ?? null);
  expect(mounted.container.querySelector("dialog")?.textContent).not.toContain(noLibraryTicked);
});

it("says on a person's page whether they have used their link", async () => {
  const served = (hasPassword: boolean): Services =>
    servedBy({
      "GET /me": me(1),
      "GET /users/5": [
        200,
        {
          user: { id: 5, name: "Dan", role: "user", hasPassword },
          watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
        },
      ],
      "GET /users/5/devices": [200, { devices: [] }],
      "GET /roles": [200, roles],
    });
  mounted = await mount(withServices(<UserPage userId={5} />, served(true)));
  expect(mounted.container.textContent).toContain("Has a password");
  mounted.unmount();
  mounted = await mount(withServices(<UserPage userId={5} />, served(false)));
  expect(mounted.container.textContent).toContain("Has not used their link yet");
});

it("removes a person once the admin confirms, and goes back to everyone (S40)", async () => {
  const heard: Heard[] = [];
  const { page, services } = await mountPerson(1, heard);
  mounted = page;
  expect(mounted.container.textContent).toContain(noLibraryChosen);
  await click(buttonWithText(mounted.container, "Remove Dan…"));
  expect(mounted.container.textContent).toContain("Dan is signed out everywhere at once");
  expect(heard.some((each) => each.methodAndPath === "DELETE /users/5")).toBe(false);
  await click(buttonWithText(mounted.container, "Remove them"));
  expect(heard.some((each) => each.methodAndPath === "DELETE /users/5")).toBe(true);
  expect(services.navigation.route()).toEqual({ screen: "users" });
});

it("offers no removal on the admin's own page", async () => {
  mounted = (await mountPerson(5)).page;
  expect(buttonWithText(mounted.container, "Remove Dan…")).toBeUndefined();
});

it("tells someone given no library why their list is empty", async () => {
  const empty = { state: "loaded", value: { nextCursor: "", items: [] } } as const;
  const view = (
    <BrowseView kind="film" loadedEntries={empty} isLoadingMore={false} onLoadMore={ignore} />
  );
  mounted = await mount(withServices(view, servedBy({ "GET /me": me(5, true) })));
  expect(mounted.container.textContent).toContain("You have not been given any library yet");
  mounted.unmount();
  mounted = await mount(withServices(view, servedBy({ "GET /me": me(5, false) })));
  expect(mounted.container.textContent).toContain("Once a library is scanned");
});

/** Seven devices of Dan's, more than a person's page shows before All devices. */
const sevenDevices = Array.from({ length: 7 }, (_, at) => ({
  id: 10 + at,
  name: `Device ${String(at + 1)}`,
  platform: "web",
  addedAt: 0,
  lastUsedAt: 0,
  isSignedIn: true,
  isByCode: false,
  isThisDevice: false,
}));

/** The text field labelled label on a page. */
function fieldLabelled(page: Element, label: string): Element | null | undefined {
  return [...page.querySelectorAll("label")]
    .find((each) => each.textContent.startsWith(label))
    ?.querySelector("input");
}

// ui-audit-2026-11-04.md §3, A person: as Your account, five devices and All devices, the actions
// in a header row; and each setting saved at once (the owner, 2026-11-04, Q5).
it("shows a person's five latest devices with All devices, their actions in a row under their name, and saves their name and note at once", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <UserPage userId={5} />,
      servedBy(
        {
          "GET /me": me(1),
          "GET /users/5": [
            200,
            {
              user: { id: 5, name: "Dan", role: "user", hasPassword: true },
              watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
            },
          ],
          "GET /users/5/devices": [200, { devices: sevenDevices }],
          "GET /roles": [200, roles],
          "PUT /users/5": [204, null],
          "PUT /users/5/admin-note": [204, null],
        },
        heard,
      ),
    ),
  );
  const page = mounted.container;
  expect([...page.querySelectorAll(".your-devices__name")].map((name) => name.textContent)).toEqual(
    ["Device 1", "Device 2", "Device 3", "Device 4", "Device 5"],
  );
  expect(page.querySelector(".your-devices__all summary")?.textContent).toBe("All devices (7)");
  // The actions sit in one row straight under the name, before the settings.
  const row = page.querySelector("h1 + p + .person__actions");
  expect([...(row?.querySelectorAll("button") ?? [])].map((button) => button.textContent)).toEqual([
    "New reset password link",
    "Remove Dan…",
  ]);
  expect(buttonWithText(page, "Rename")).toBeUndefined();
  const name = fieldLabelled(page, "Name");
  await enter(name, "Daniel");
  await fire(name, new FocusEvent("focusout", { bubbles: true }));
  await fire(name, new FocusEvent("focusout", { bubbles: true }));
  expect(
    heard.filter((each) => each.methodAndPath === "PUT /users/5").map((each) => each.body),
  ).toEqual([{ name: "Daniel" }]);
  expect(name?.closest("form")?.querySelector("output.save-note")?.textContent).toBe("Saved");
  const note = fieldLabelled(page, "Who it is for");
  await enter(note, "my brother");
  await submit(note?.closest("form"));
  expect(heard.find((each) => each.methodAndPath === "PUT /users/5/admin-note")?.body).toEqual({
    adminNote: "my brother",
  });
});
