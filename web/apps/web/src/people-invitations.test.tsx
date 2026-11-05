// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it, vi } from "vitest";

import { Users } from "./users.tsx";
import {
  buttonContaining,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const roles = {
  capabilities: ["administer"],
  roles: [
    {
      id: 2,
      name: "user",
      isBuiltin: true,
      capabilities: [],
      isEveryLibrary: true,
      libraries: [],
      holderCount: 1,
    },
  ],
};

const waiting = {
  id: 7,
  role: "user",
  adminNote: "my cousin",
  issuedAt: 1,
  liveLinks: 0,
  expiresAt: 0,
  links: [],
};
const served = {
  "GET /users": [200, { users: [] }],
  "GET /roles": [200, roles],
  "GET /libraries": [200, { libraries: [] }],
} as const;

it("opens a waiting invitation's dialog with its live links, each with Copy (D267 §5, D399)", async () => {
  const live = {
    ...waiting,
    liveLinks: 2,
    expiresAt: 1_790_000_000_000,
    links: [
      { secret: "first-secret", expiresAt: 1_789_000_000_000 },
      {
        secret: "second-secret",
        expiresAt: 1_790_000_000_000,
        link: "https://discovery.example/register#core.second-secret.print",
      },
    ],
  };
  const copied: string[] = [];
  vi.stubGlobal("isSecureContext", true);
  vi.spyOn(globalThis.navigator.clipboard, "writeText").mockImplementation((text: string) =>
    Promise.resolve(void copied.push(text)),
  );
  try {
    mounted = await mount(
      withServices(
        <Users />,
        servedBy({ ...served, "GET /invitations": [200, { invitations: [live] }] }),
      ),
    );
    // The page itself shows no link: a row opens its dialog.
    expect(mounted.container.querySelector(".share__link")).toBeNull();
    await click(buttonContaining(mounted.container, "my cousin"));
    const dialog = mounted.container.querySelector("dialog");
    const shown = [...(dialog?.querySelectorAll(".share__link") ?? [])].map(
      (link) => link.textContent,
    );
    expect(shown).toHaveLength(2);
    expect(shown[0]).toMatch(/\/register#first-secret$/u);
    expect(shown[1]).toBe("https://discovery.example/register#core.second-secret.print");
    expect(dialog?.querySelectorAll("img")).toHaveLength(2);
    const copies = [...(dialog?.querySelectorAll("button") ?? [])].filter(
      (button) => button.textContent === "Copy",
    );
    expect(copies).toHaveLength(2);
    await click(copies[1] ?? null);
    await settleUntil(() => copied.length === 1);
    expect(copied).toEqual(["https://discovery.example/register#core.second-secret.print"]);
    await settleUntil(() => dialog?.textContent.includes("Copied.") === true);
    expect(buttonWithText(dialog as HTMLElement, "Withdraw")).toBeDefined();
  } finally {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  }
});

it("lists the waiting invitations by note, role and date, with nothing to send again (D318, D403)", async () => {
  mounted = await mount(
    withServices(
      <Users />,
      servedBy({ ...served, "GET /invitations": [200, { invitations: [waiting] }] }),
    ),
  );
  const page = mounted.container;
  const rows = [
    ...(page.querySelector("#people-invitations")?.parentElement?.querySelectorAll("li") ?? []),
  ];
  expect(rows).toHaveLength(1);
  expect(rows[0]?.textContent).toContain("my cousin");
  expect(rows[0]?.textContent).toContain("Can watch");
  expect(rows[0]?.textContent).toContain("Issued ");
  // Nothing sends another link any more, on the page or in a dialog.
  expect(buttonWithText(page, "Send another link")).toBeUndefined();
  await click(buttonContaining(page, "my cousin"));
  expect(page.textContent).toContain("Its links have ended");
  expect(buttonWithText(page, "Send another link")).toBeUndefined();
  expect(buttonWithText(page, "Withdraw")).toBeDefined();
});

it("shows no invitations section once the server lists none, a used one's account being among the accounts (D403)", async () => {
  mounted = await mount(
    withServices(
      <Users />,
      servedBy({
        ...served,
        "GET /users": [
          200,
          {
            users: [{ id: 4, name: "Gran", role: "user", adminNote: "Grandma", hasPassword: true }],
          },
        ],
        "GET /invitations": [200, { invitations: [] }],
      }),
    ),
  );
  const page = mounted.container;
  expect(page.querySelector("#people-invitations")).toBeNull();
  expect(page.querySelector("#people-accounts")?.parentElement?.textContent).toContain("Gran");
});
