// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { SecurityUpdateNote } from "./security-update-note.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { HowToUpdateView, UpdateSettings, UpdateSystem } from "./update-page.tsx";
import { securityUpdateSnoozeKey, snoozeLastsMs, type Update } from "./update.ts";

let mounted: Mounted | undefined;

beforeEach(() => {
  localStorage.removeItem(securityUpdateSnoozeKey);
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-31T09:00:00Z"));
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
  localStorage.removeItem(securityUpdateSnoozeKey);
});

const command = "docker run --rm ghcr.io/kstenerud/orpheum:v0.3.3 install-script | sh";

/** A server whose version lacks a security fix, which v0.3.2 has; v0.3.3 is the latest. */
const pending: Update = {
  version: "v0.3.1",
  latest: "v0.3.3",
  digest: `sha256:${"a".repeat(64)}`,
  security: { summary: "A sign-in could be guessed.", fixedIn: "v0.3.2" },
  command,
  checkedAt: 1_793_000_000_000,
};

/** A server with a newer release out that has no security fix for it. */
const plain: Update = {
  version: "v0.3.2",
  latest: "v0.3.3",
  command,
  checkedAt: 1_793_000_000_000,
};

const banner = "A security update is available (v0.3.2): A sign-in could be guessed.";

async function mountNote(isAdmin: boolean, update: Update, heard: Heard[] = []): Promise<Mounted> {
  return await mount(
    withServices(
      <SecurityUpdateNote isAdmin={isAdmin} />,
      servedBy({ "GET /system/update": [200, update] }, heard),
    ),
  );
}

// security-updates.md Part B: an admin is shown a security fix pending on every page, with the
// command and the way to How to update; nobody else sees it, and a plain release is no banner.
describe("SecurityUpdateNote", () => {
  it("shows an admin the fix pending, the command and the way to How to update", async () => {
    mounted = await mountNote(true, pending);
    const text = mounted.container.textContent;
    expect(text).toContain(banner);
    expect(text).toContain(command);
    expect(mounted.container.querySelector('a[href="/update"]')?.textContent).toBe("Update now");
    expect(buttonWithText(mounted.container, "Snooze for a day")).toBeDefined();
  });

  it("gives a server run from its compose file the compose file's command (D450 §2)", async () => {
    mounted = await mountNote(true, { ...pending, installRoute: "compose" });
    const text = mounted.container.textContent;
    expect(text).toContain(banner);
    expect(mounted.container.querySelector("code")?.textContent).toBe(
      "docker compose pull && docker compose up -d",
    );
    expect(text).not.toContain(command);
  });

  it("shows anyone but an admin nothing, and asks the server nothing for them", async () => {
    const heard: Heard[] = [];
    mounted = await mountNote(false, pending, heard);
    expect(mounted.container.textContent).toBe("");
    expect(heard).toEqual([]);
  });

  it("is no banner for a newer release without a security fix", async () => {
    mounted = await mountNote(true, plain);
    expect(mounted.container.textContent).toBe("");
  });

  it("snoozed, is hidden in this browser for a day, and then shown again", async () => {
    mounted = await mountNote(true, pending);
    await click(buttonWithText(mounted.container, "Snooze for a day"));
    expect(mounted.container.textContent).toBe("");
    mounted.unmount();
    // The page loaded again within the day: still hidden.
    vi.setSystemTime(Date.now() + snoozeLastsMs - 60_000);
    mounted = await mountNote(true, pending);
    expect(mounted.container.textContent).toBe("");
    mounted.unmount();
    // A day on: shown again, the fix still pending.
    vi.setSystemTime(Date.now() + 60_000);
    mounted = await mountNote(true, pending);
    expect(mounted.container.textContent).toContain(banner);
  });

  it("shows a new fix at once, whatever an earlier one's snooze", async () => {
    mounted = await mountNote(true, pending);
    await click(buttonWithText(mounted.container, "Snooze for a day"));
    mounted.unmount();
    const newer: Update = {
      ...pending,
      security: { summary: "A link could be reused.", fixedIn: "v0.3.3" },
    };
    mounted = await mountNote(true, newer);
    expect(mounted.container.textContent).toContain(
      "A security update is available (v0.3.3): A link could be reused.",
    );
  });

  it("stands above every page of an admin's shell", async () => {
    mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={() => undefined}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        servedBy({
          "GET /system/update": [200, pending],
          // The rest of the shell's own asking: the libraries menu and the tray's sign.
          "GET /libraries": [200, { libraries: [] }],
          "GET /needs-attention": [
            200,
            {
              rootCount: 0,
              diskCount: 0,
              waitingRootCount: 0,
              unreadCount: 0,
              matchCount: 0,
              seriesReadWronglyCount: 0,
              filmsWithoutSoundCount: 0,
            },
          ],
        }),
      ),
    );
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes(banner));
    expect(shown.container.querySelector("main")?.textContent).toContain(banner);
  });
});

describe("How to update", () => {
  it("gives the command and the steps, and the fix the version lacks", async () => {
    mounted = await mount(withServices(<HowToUpdateView update={pending} />));
    const text = mounted.container.textContent;
    expect(text).toContain("This server runs v0.3.1. v0.3.3 is available.");
    expect(text).toContain(
      "Your version lacks a security fix, which v0.3.2 has: A sign-in could be guessed.",
    );
    expect(mounted.container.querySelector("pre code")?.textContent).toBe(command);
    expect(text).toContain("If you installed the server with its install script");
    expect(text).toContain(pending.digest);
  });

  it("gives a compose-file server's way too: OpenMediaVault's Pull then Up, or docker compose (D450 §2)", async () => {
    mounted = await mount(withServices(<HowToUpdateView update={pending} />));
    const text = mounted.container.textContent;
    expect(text).toContain("press Pull, then Up.");
    expect(text).toContain("If you run the server from its compose file");
    const commands = [...mounted.container.querySelectorAll("pre code")].map(
      (each) => each.textContent,
    );
    expect(commands).toEqual([command, "docker compose pull && docker compose up -d"]);
  });

  it("gives only the way the server was installed, when it knows it (D450 §2)", async () => {
    mounted = await mount(
      withServices(<HowToUpdateView update={{ ...pending, installRoute: "compose" }} />),
    );
    const byCompose = mounted.container;
    expect([...byCompose.querySelectorAll("pre code")].map((each) => each.textContent)).toEqual([
      "docker compose pull && docker compose up -d",
    ]);
    expect(byCompose.textContent).toContain("press Pull, then Up.");
    expect(byCompose.textContent).not.toContain("with its install script");
    expect(byCompose.textContent).not.toContain("If you run the server from its compose file");
    mounted.unmount();
    mounted = await mount(
      withServices(<HowToUpdateView update={{ ...pending, installRoute: "script" }} />),
    );
    const byScript = mounted.container;
    expect([...byScript.querySelectorAll("pre code")].map((each) => each.textContent)).toEqual([
      command,
    ]);
    expect(byScript.textContent).toContain("Open a terminal on the machine the server runs on");
    expect(byScript.textContent).not.toContain(
      "If you installed the server with its install script",
    );
    expect(byScript.textContent).not.toContain("press Pull, then Up.");
  });

  it("says what a newer compose file brings, even while the server runs the latest (D461 §3)", async () => {
    const latest: Update = { version: "v0.3.3", checkedAt: 1_793_000_000_000 };
    mounted = await mount(
      withServices(
        <HowToUpdateView
          update={{
            ...latest,
            installRoute: "compose",
            composeFile: { version: 1, newest: 2, brings: ["Hardware transcoding on Intel."] },
          }}
        />,
      ),
    );
    const text = mounted.container.textContent;
    expect(text).toContain("started from version 1 of its compose file. Version 2 is out");
    expect([...mounted.container.querySelectorAll("li")].map((each) => each.textContent)).toEqual([
      "Hardware transcoding on Intel.",
    ]);
    expect(text).toContain("paste the new compose file over the old one");
    mounted.unmount();
    mounted = await mount(withServices(<HowToUpdateView update={latest} />));
    expect(mounted.container.textContent).not.toContain("compose file");
  });

  it("says the server runs the latest, with no command", async () => {
    mounted = await mount(
      withServices(
        <HowToUpdateView update={{ version: "v0.3.3", checkedAt: 1_793_000_000_000 }} />,
      ),
    );
    expect(mounted.container.textContent).toBe(
      "This server runs v0.3.3. It is the latest version its discovery knows of.",
    );
  });
});

describe("UpdateSettings", () => {
  it("is where a newer release without a security fix shows, with the way to How to update", async () => {
    mounted = await mount(
      withServices(<UpdateSettings />, servedBy({ "GET /system/update": [200, plain] })),
    );
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes("v0.3.3 is available."));
    expect(shown.container.textContent).toContain("This server runs v0.3.2.");
    expect(shown.container.querySelector('a[href="/update"]')?.textContent).toBe("How to update");
  });
});

describe("UpdateSystem", () => {
  it("says on the System page that a newer version is available, and how to update (D450 §2)", async () => {
    mounted = await mount(
      withServices(<UpdateSystem />, servedBy({ "GET /system/update": [200, pending] })),
    );
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes("v0.3.3 is available."));
    expect(shown.container.querySelector("h2")?.textContent).toBe("Updates");
    expect(shown.container.textContent).toContain("docker compose pull && docker compose up -d");
  });
});
