// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, expect, it, vi } from "vitest";

import { JobList, type Job } from "./jobs.tsx";
import { LibraryPage } from "./library-manage.tsx";
import { SectionSettingsView } from "./server-settings.tsx";
import { Users } from "./users.tsx";
import { UserPage } from "./user.tsx";
import { Settings } from "./settings.tsx";
import {
  buttonContaining,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/*
 * What cannot be undone is red and asked again (D278 §4, the owner, 2026-10-12: "red button +
 * confirm action is a general pattern that will occur often"), in the one confirm (ui.md rule 8,
 * D538 §4): the first press sends nothing and opens it, focus on Cancel (the owner, 2026-11-22: "the
 * confirmation defaults to not doing it"); Cancel sends nothing; only its action does.
 * dangerous-actions.test.ts lists every action that asks.
 */

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const roles = {
  capabilities: ["administer", "editMetadata", "hostWatchTogether"],
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
      capabilities: ["hostWatchTogether"],
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
      kind: "film",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
    {
      id: 2,
      name: "Private",
      kind: "film",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
  ],
};

/** The confirm open now, if one is. */
function confirmShown(): HTMLElement | null {
  return document.querySelector<HTMLElement>('dialog[role="alertdialog"]');
}

/**
 * Expects the confirm open, its foot Cancel then confirmLabel, focus on Cancel, the action marked
 * dangerous, and returns the action's button.
 */
async function expectAsked(confirmLabel: string): Promise<HTMLElement | undefined> {
  const asked = confirmShown();
  expect(asked).not.toBeNull();
  expect(
    [...(asked?.querySelectorAll(".modal-dialog__foot button") ?? [])].map((b) => b.textContent),
  ).toEqual(["Cancel", confirmLabel]);
  await vi.waitFor(() => {
    expect(document.activeElement?.textContent).toBe("Cancel");
  });
  const action = buttonWithText(asked ?? document.body, confirmLabel);
  expect(action?.hasAttribute("data-dangerous-action")).toBe(true);
  return action;
}

/**
 * Presses opener, expecting nothing sent and the confirm open on Cancel; presses Cancel, expecting
 * nothing sent and the confirm gone; then presses opener again and the confirm's action, expecting
 * methodAndPath sent. The opener is outlined red unless the action is harmless.
 */
async function pressAndConfirm(
  heard: Heard[],
  press: {
    readonly label: string;
    readonly confirmLabel: string;
    readonly methodAndPath: string;
    readonly isHarmless?: boolean;
  },
): Promise<void> {
  if (mounted === undefined) {
    throw new Error("nothing mounted");
  }
  const container = mounted.container;
  const opener = (): HTMLElement | undefined => buttonWithText(container, press.label);
  expect(opener()?.className).toBe(press.isHarmless === true ? "" : "irreversible-outline");
  await click(opener());
  const sent = (): string[] => heard.map((each) => each.methodAndPath);
  expect(sent()).not.toContain(press.methodAndPath);
  await expectAsked(press.confirmLabel);
  await click(buttonWithText(confirmShown() ?? container, "Cancel"));
  expect(confirmShown()).toBeNull();
  expect(sent()).not.toContain(press.methodAndPath);
  await click(opener());
  await click(buttonWithText(confirmShown() ?? container, press.confirmLabel));
  expect(sent()).toContain(press.methodAndPath);
}

it("deletes a role from its dialog, and withdraws an invitation from its dialog, only when asked again", async () => {
  const heard: Heard[] = [];
  const invitation = {
    id: 7,
    role: "friends",
    adminNote: "my cousin",
    issuedAt: 1,
    expiresAt: 2,
    liveLinks: 1,
    links: [{ secret: "s", expiresAt: 2 }],
  };
  const answers = {
    "GET /users": [200, { users: [] }],
    "GET /roles": [200, roles],
    "GET /libraries": [200, libraries],
    "GET /invitations": [200, { invitations: [invitation] }],
    "DELETE /roles/3": [204, null],
    "DELETE /invitations/7": [204, null],
  } as const;
  mounted = await mount(withServices(<Users />, servedBy(answers, heard)));
  const roleRow = [...mounted.container.querySelectorAll("button.grid-list__target")].find((each) =>
    each.textContent.startsWith("friends"),
  );
  await click(roleRow ?? null);
  await pressAndConfirm(heard, {
    label: "Delete the role",
    confirmLabel: "Delete it",
    methodAndPath: "DELETE /roles/3",
  });
  mounted.unmount();
  mounted = await mount(withServices(<Users />, servedBy(answers, heard)));
  await click(buttonContaining(mounted.container, "my cousin"));
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
          "POST /users/5/registration-invitation": [201, { secret: "s", expiresAt: 2 }],
        },
        heard,
      ),
    ),
  );
  expect(mounted.container.textContent).not.toContain("keeps working until one of them is used");
  await pressAndConfirm(heard, {
    label: "New reset password link",
    confirmLabel: "Make a reset link",
    methodAndPath: "POST /users/5/registration-invitation",
    isHarmless: true,
  });
});

it("removes a stored secret only when asked again", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <Settings page="keys" />,
      servedBy(
        {
          "GET /secrets": [
            200,
            {
              secrets: [
                { name: "tmdbApiKey", description: "TMDB", isSet: true, needsReentry: false },
              ],
            },
          ],
          "GET /config": [200, { settings: [] }],
          "GET /needs-attention": [200, { items: [] }],
          "DELETE /secrets/tmdbApiKey": [204, null],
        },
        heard,
      ),
    ),
  );
  // Remove… asks first, in the confirm, on Cancel; only its red action removes the key.
  const bin = buttonWithText(mounted.container, "Remove…");
  await click(bin);
  expect(heard.map((each) => each.methodAndPath)).not.toContain("DELETE /secrets/tmdbApiKey");
  expect(confirmShown()?.getAttribute("aria-label")).toBe("Remove the TMDB API key?");
  const confirm = await expectAsked("Remove it");
  expect(confirm?.className).toBe("irreversible");
  await click(buttonWithText(confirmShown() ?? document.body, "Cancel"));
  expect(confirmShown()).toBeNull();
  await click(bin);
  await click(buttonWithText(confirmShown() ?? document.body, "Remove it"));
  expect(heard.map((each) => each.methodAndPath)).toContain("DELETE /secrets/tmdbApiKey");
});

it("ends a person's own link only when asked again, saying until when it led them back (D292 §4)", async () => {
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
        },
        [],
      ),
    ),
  );
  expect(mounted.container.textContent).toContain("cousin");
  expect(mounted.container.textContent).not.toContain("leads them back");
  expect(buttonWithText(mounted.container, "End that link…")).toBeUndefined();
});

it("stops a running job only when asked again, its work lost (ui.md rule 8)", async () => {
  const running: Job = {
    id: 8,
    kind: "backup",
    state: "running",
    reason: "an admin asked for a backup now",
    attemptCount: 1,
    createdAt: 1,
  };
  const stopped: Job[] = [];
  mounted = await mount(
    withServices(
      <JobList
        jobs={[running]}
        onCancel={(job) => {
          stopped.push(job);
        }}
      />,
    ),
  );
  const stop = mounted.container.querySelector<HTMLElement>('button[aria-label="Stop Backing up"]');
  expect(stop?.className).toBe("irreversible-outline");
  await click(stop);
  expect(stopped).toEqual([]);
  expect(confirmShown()?.getAttribute("aria-label")).toBe("Stop Backing up?");
  await expectAsked("Stop the job");
  await click(buttonWithText(confirmShown() ?? document.body, "Cancel"));
  expect(stopped).toEqual([]);
  await click(stop);
  await click(buttonWithText(confirmShown() ?? document.body, "Stop the job"));
  expect(stopped).toEqual([running]);
});

it("removes what a scan did not find, and a gone disk's files, only when asked again (D208)", async () => {
  const heard: Heard[] = [];
  const withheld = {
    id: 3,
    name: "Films",
    kind: "film",
    metadataSource: "nfo",
    changeDetection: "manual",
    isWritable: false,
    createdAt: 1,
    roots: [
      {
        nodeName: "local",
        rootPath: "/media/films",
        lastScanOutcome: "withheld",
        withheldReason: "The scan would remove 7 files.",
        missingMountPaths: ["/media/films/disk1"],
      },
    ],
  } as unknown as components["schemas"]["LibraryBody"];
  mounted = await mount(
    withServices(
      <LibraryPage libraryId={3} />,
      servedBy(
        {
          "GET /libraries": [200, { libraries: [withheld] }],
          "GET /nodes": [200, { nodes: [] }],
          "POST /libraries/3/roots/removals": [202, null],
          "POST /libraries/3/roots/mount-gone": [202, null],
        },
        heard,
      ),
    ),
  );
  await pressAndConfirm(heard, {
    label: "Remove what the last scan did not find",
    confirmLabel: "Remove them",
    methodAndPath: "POST /libraries/3/roots/removals",
  });
  await pressAndConfirm(heard, {
    label: "What was mounted here is gone for good",
    confirmLabel: "Remove its files",
    methodAndPath: "POST /libraries/3/roots/mount-gone",
  });
});

// D538 §11: Use the default, in What it is for's dialog beside its Cancel, took the bin's confirm's place.
it("returns a setting to its default only when asked in What it is for, Cancel leaving it", async () => {
  const reset: string[] = [];
  mounted = await mount(
    withServices(
      <SectionSettingsView
        section="together"
        isAdvanced={false}
        problems={new Map()}
        onSave={() => undefined}
        onReset={(key) => {
          reset.push(key);
        }}
        settings={[
          {
            key: "tmdbRefreshAfterDays",
            kind: "integer",
            description: "days",
            value: 30,
            startingValue: 180,
            isModified: true,
            isImportant: true,
            section: "together",
            setAt: 1,
          },
        ]}
      />,
    ),
  );
  const info = mounted.container.querySelector<HTMLElement>(
    'button[aria-label="What Refresh details from TMDB after (days) is for"]',
  );
  await click(info);
  expect(reset).toEqual([]);
  const asked = (): HTMLElement | null => document.querySelector<HTMLElement>("dialog");
  expect(asked()?.textContent).toContain("The default is 180.");
  expect(
    [...(asked()?.querySelectorAll(".modal-dialog__foot button") ?? [])].map((b) => b.textContent),
  ).toEqual(["Cancel", "Use the default"]);
  await click(buttonWithText(asked() ?? document.body, "Cancel"));
  expect(asked()).toBeNull();
  expect(reset).toEqual([]);
  await click(info);
  await click(buttonWithText(asked() ?? document.body, "Use the default"));
  expect(reset).toEqual(["tmdbRefreshAfterDays"]);
});
