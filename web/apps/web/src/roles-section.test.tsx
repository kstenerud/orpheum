// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { People } from "./people.tsx";
import { PersonPage } from "./person.tsx";
import { RolePage } from "./role-page.tsx";
import {
  buttonWithText,
  choose,
  click,
  enter,
  fire,
  mount,
  servedBy,
  submit,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

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

const answers = {
  "GET /users": [
    200,
    {
      accounts: [
        { id: 1, name: "owner", role: "admin", hasPassword: true },
        { id: 5, name: "cousin", role: "user", adminNote: "Anna", hasPassword: true },
      ],
    },
  ],
  "GET /roles": [200, roles],
  "GET /libraries": [200, libraries],
  "PUT /roles/2": [204, null],
  "PUT /roles/3": [204, null],
  "POST /roles": [201, { id: 4 }],
  "POST /users": [201, { account: { id: 6, name: "Eve", role: "user", hasPassword: true } }],
  "GET /invitations": [200, { invitations: [] }],
  "POST /invitations": [
    201,
    {
      invitation: {
        id: 3,
        role: "friends",
        adminNote: "my cousin",
        issuedAt: 1,
        expiresAt: 2,
        liveLinks: 1,
        links: [{ secret: "s", expiresAt: 2 }],
      },
      link: { secret: "s", expiresAt: 2 },
    },
  ],
} as const;

/** The People page, answered with the roles and libraries above. */
async function mountPeople(heard: Heard[]): Promise<Mounted> {
  return await mount(withServices(<People />, servedBy(answers, heard)));
}

/** A role's page, answered with the roles and libraries above. */
async function mountRole(roleId: number, heard: Heard[]): Promise<Mounted> {
  return await mount(withServices(<RolePage roleId={roleId} />, servedBy(answers, heard)));
}

/** The body of each change sent to a role. */
function changesTo(heard: Heard[], methodAndPath: string): unknown[] {
  return heard.filter((each) => each.methodAndPath === methodAndPath).map((each) => each.body);
}

it("offers Create Role, Create Account and Invite Someone side by side above the sections (D399)", async () => {
  mounted = await mountPeople([]);
  const actions = mounted.container.querySelector(".people__actions");
  expect([...(actions?.querySelectorAll("button") ?? [])].map((each) => each.textContent)).toEqual([
    "Create Role",
    "Create Account",
    "Invite Someone",
  ]);
  expect(mounted.container.querySelector("dialog")).toBeNull();
  const sections = [...mounted.container.querySelectorAll("h2")].map((each) => each.textContent);
  expect(sections).toEqual(["Accounts", "Roles"]);
});

it("lists accounts by name, note and role, and roles by name and holders, each leading to its page (D399)", async () => {
  mounted = await mountPeople([]);
  const links = [...mounted.container.querySelectorAll("a")].map((each) => ({
    href: each.getAttribute("href"),
    text: each.textContent,
  }));
  expect(links).toContainEqual({ href: "/people/5", text: "cousinAnnaCan watch" });
  expect(links).toContainEqual({ href: "/roles/1", text: "Administrator1 people" });
  expect(links).toContainEqual({ href: "/roles/3", text: "friends0 people" });
});

it("makes a role in a dialog that closes and opens nothing, as the admin asks (D243 §4, D399)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  await click(buttonWithText(mounted.container, "Create Role"));
  const newRole = buttonWithText(mounted.container, "Add the role")?.closest("form");
  await enter(newRole?.querySelector("input"), "cousins");
  await submit(newRole);
  expect(changesTo(heard, "POST /roles")).toEqual([
    { name: "cousins", capabilities: [], libraries: [] },
  ]);
  expect(mounted.container.querySelector("dialog")).toBeNull();
});

it("makes an account in a dialog that closes and opens nothing (D246 §2, D399)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  await click(buttonWithText(mounted.container, "Create Account"));
  const form = buttonWithText(mounted.container, "Add the account")?.closest("form");
  const [name, password] = [...(form?.querySelectorAll("input") ?? [])];
  await enter(name, "Eve");
  await enter(password, "pw");
  await submit(form);
  expect(changesTo(heard, "POST /users")).toEqual([{ name: "Eve", password: "pw", role: "user" }]);
  expect(mounted.container.querySelector("dialog")).toBeNull();
});

it("saves each change to a role's libraries at once, with no Save button (D243 §4, D399)", async () => {
  const heard: Heard[] = [];
  mounted = await mountRole(2, heard);
  expect(buttonWithText(mounted.container, "Save")).toBeUndefined();
  const privateBox = [...mounted.container.querySelectorAll("label")].find((label) =>
    label.textContent.includes("Private"),
  );
  await click(privateBox?.querySelector("input"));
  expect(changesTo(heard, "PUT /roles/2")).toEqual([
    { name: "user", capabilities: ["host_watch_together"], libraries: [{ libraryId: 1 }] },
  ]);
});

it("saves a capability ticked at once, and a rename once the name is left (D399)", async () => {
  const heard: Heard[] = [];
  mounted = await mountRole(3, heard);
  const editMetadata = [...mounted.container.querySelectorAll("label")].find((label) =>
    label.textContent.includes("Fix identification"),
  );
  await click(editMetadata?.querySelector("input"));
  expect(changesTo(heard, "PUT /roles/3")).toEqual([
    { name: "friends", capabilities: ["edit_metadata"], libraries: [] },
  ]);
  const name = mounted.container.querySelector("input:not([type=checkbox])");
  await enter(name, "pals");
  await fire(name, new FocusEvent("focusout", { bubbles: true }));
  expect(changesTo(heard, "PUT /roles/3")[1]).toEqual({
    name: "pals",
    capabilities: [],
    libraries: [],
  });
});

it("says the administrator role reaches every library, and lets none of it change (D243 §4)", async () => {
  mounted = await mountRole(1, []);
  expect(mounted.container.textContent).toContain("Every library: administrators see everything.");
  expect(mounted.container.querySelector("fieldset[disabled]")).not.toBeNull();
});

it("offers Delete only while no one has the role, and never for a built-in one (D399)", async () => {
  mounted = await mountRole(3, []);
  expect(buttonWithText(mounted.container, "Delete the role")).toBeDefined();
  mounted.unmount();
  mounted = await mountRole(2, []);
  expect(buttonWithText(mounted.container, "Delete the role")).toBeUndefined();
  mounted.unmount();
  const held = {
    ...roles,
    roles: roles.roles.map((role) => (role.id === 3 ? { ...role, holderCount: 2 } : role)),
  };
  mounted = await mount(
    withServices(
      <RolePage roleId={3} />,
      servedBy({ "GET /roles": [200, held], "GET /libraries": [200, libraries] }),
    ),
  );
  expect(buttonWithText(mounted.container, "Delete the role")).toBeUndefined();
});

it("invites someone into a role with a note of who it is for, and opens the invitation at once with its link (D246 §2, D399)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  await click(buttonWithText(mounted.container, "Invite Someone"));
  const invitation = buttonWithText(mounted.container, "Make their link")?.closest("form");
  await enter(invitation?.querySelector("input"), "my cousin");
  await choose(invitation?.querySelector("select"), "friends");
  await submit(invitation);
  expect(changesTo(heard, "POST /invitations")).toEqual([
    { role: "friends", adminNote: "my cousin" },
  ]);
  const dialogs = mounted.container.querySelectorAll("dialog");
  expect(dialogs).toHaveLength(1);
  const dialog = dialogs[0];
  expect(dialog?.getAttribute("aria-label")).toBe("Invitation for my cousin");
  expect(dialog?.querySelector(".share__link")?.textContent).toMatch(/\/register#s$/u);
  expect(dialog?.querySelector("img")).not.toBeNull();
  expect(buttonWithText(mounted.container, "Withdraw")).toBeDefined();
});

it("gives a person another role from their page", async () => {
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
          "PUT /users/5/role": [204, null],
        },
        heard,
      ),
    ),
  );
  const choice = buttonWithText(mounted.container, "Give this role")?.closest("form");
  await choose(choice?.querySelector("select"), "friends");
  await submit(choice);
  expect(heard.find((each) => each.methodAndPath === "PUT /users/5/role")?.body).toEqual({
    role: "friends",
  });
});
