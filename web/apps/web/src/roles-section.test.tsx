// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

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
      kind: "movie",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
    {
      id: 2,
      name: "Private",
      kind: "movie",
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
      users: [
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
  "POST /users": [201, { user: { id: 6, name: "Eve", role: "user", hasPassword: true } }],
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

/** The Users page, answered with the roles and libraries above. */
async function mountPeople(heard: Heard[]): Promise<Mounted> {
  return await mount(withServices(<Users />, servedBy(answers, heard)));
}

/** The Users page with a role's dialog opened from its row, answered with the roles above. */
async function mountRole(roleId: number, heard: Heard[]): Promise<Mounted> {
  const page = await mountPeople(heard);
  await openRole(page, roleId);
  return page;
}

/** What each role's row says, by the role's id. */
const rowNames: Record<number, string> = { 1: "Administrator", 2: "Can watch", 3: "friends" };

/** Opens the dialog of the role with id roleId by clicking its row in Roles. */
async function openRole(page: Mounted, roleId: number): Promise<void> {
  const row = [
    ...page.container.querySelectorAll("#roles-heading + p + ul button.people__row"),
  ].find((each) => each.textContent.startsWith(rowNames[roleId] ?? ""));
  await click(row ?? null);
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

it("lists accounts by name, note and role leading to their pages, and roles by name and holders opening a dialog (D399, D403)", async () => {
  mounted = await mountPeople([]);
  const links = [...mounted.container.querySelectorAll("a")].map((each) => ({
    href: each.getAttribute("href"),
    text: each.textContent,
  }));
  expect(links).toContainEqual({ href: "/users/5", text: "cousinAnnaCan watch" });
  expect(links.some((link) => link.href?.startsWith("/roles/") === true)).toBe(false);
  const rows = [...mounted.container.querySelectorAll("button.people__row")].map(
    (each) => each.textContent,
  );
  expect(rows).toContain("Administrator1 person");
  expect(rows).toContain("friends0 people");
});

it("opens the same dialog to make a role and to look at one, which has no Add button (D403)", async () => {
  mounted = await mountPeople([]);
  await click(buttonWithText(mounted.container, "Create Role"));
  const making = mounted.container.querySelector("dialog");
  expect(making?.querySelector("h2")?.textContent).toBe("New role");
  expect(buttonWithText(making as HTMLElement, "Add the role")).toBeDefined();
  expect(making?.textContent).toContain("What its people may do");
  expect(making?.textContent).toContain("The libraries its people see");
  mounted.unmount();
  mounted = await mountRole(3, []);
  const showing = mounted.container.querySelector("dialog");
  expect(showing?.querySelector("h2")?.textContent).toBe("friends");
  expect(buttonWithText(showing as HTMLElement, "Add the role")).toBeUndefined();
  expect(showing?.textContent).toContain("What its people may do");
  expect(showing?.textContent).toContain("The libraries its people see");
});

it("says in a role's dialog how many hold it, one as a person", async () => {
  mounted = await mountRole(2, []);
  const holders = (): string[] =>
    [...(mounted?.container.querySelectorAll("dialog h2 ~ p") ?? [])].map(
      (each) => each.textContent,
    );
  expect(holders()).toContain("1 person");
  mounted.unmount();
  mounted = await mountRole(3, []);
  expect(holders()).toContain("0 people");
});

it("makes a role with the capabilities and libraries ticked in the same dialog (D403)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  await click(buttonWithText(mounted.container, "Create Role"));
  const dialog = mounted.container.querySelector("dialog");
  const box = (text: string): Element | null | undefined =>
    [...(dialog?.querySelectorAll("label") ?? [])]
      .find((label) => label.textContent.includes(text))
      ?.querySelector("input");
  await enter(dialog?.querySelector("input:not([type=checkbox])"), "pals");
  await click(box("Fix identification"));
  await click(box("Private"));
  expect(changesTo(heard, "POST /roles")).toEqual([]);
  await submit(dialog?.querySelector("form"));
  expect(changesTo(heard, "POST /roles")).toEqual([
    { name: "pals", capabilities: ["edit_metadata"], libraries: [{ libraryId: 2 }] },
  ]);
  expect(mounted.container.querySelector("dialog")).toBeNull();
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
    withServices(<Users />, servedBy({ ...answers, "GET /roles": [200, held] })),
  );
  await openRole(mounted, 3);
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
      <UserPage userId={5} />,
      servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
          "GET /users/5": [
            200,
            {
              user: { id: 5, name: "cousin", role: "user", hasPassword: true },
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
  // Saved as it is chosen (the owner, 2026-11-04, Q5): no button to press after.
  const choice = [...mounted.container.querySelectorAll("label")]
    .find((label) => label.textContent.startsWith("Role"))
    ?.querySelector("select");
  await choose(choice, "friends");
  expect(heard.find((each) => each.methodAndPath === "PUT /users/5/role")?.body).toEqual({
    role: "friends",
  });
  expect(choice?.closest(".form-row")?.querySelector("output.save-note")?.textContent).toBe(
    "Saved",
  );
});
