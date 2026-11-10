// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

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
import { HowToUpdate, HowToUpdateView } from "./update-page.tsx";
import type { Update } from "./update.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.unstubAllGlobals();
});

const command = "docker run --rm ghcr.io/kstenerud/orpheum:v0.3.4 install-script | sh";
const latest: Update = { version: "v0.3.3", checkedAt: 1_793_000_000_000 };
const newer: Update = {
  version: "v0.3.3",
  latest: "v0.3.4",
  command,
  checkedAt: 1_793_000_360_000,
};

async function mountView(update: Update): Promise<HTMLElement> {
  mounted = await mount(withServices(<HowToUpdateView update={update} />));
  return mounted.container;
}

const codes = (container: HTMLElement): (string | null)[] =>
  [...container.querySelectorAll("pre code")].map((each) => each.textContent);

// D462 §8, §10.9: the page's Check now has the server read the release record at once and shows
// what it found; one sooner than a minute after the last is told so; a failed reading says when one
// last succeeded.
describe("Check now", () => {
  it("shows what the server found", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <HowToUpdate />,
        servedBy(
          { "GET /system/update": [200, latest], "POST /system/update/check": [200, newer] },
          heard,
        ),
      ),
    );
    const shown = mounted;
    await settleUntil(() => buttonWithText(shown.container, "Check now") !== undefined);
    expect(shown.container.textContent).toContain("It is the latest version");
    await click(buttonWithText(shown.container, "Check now"));
    await settleUntil(() => shown.container.textContent.includes("v0.3.4 is available."));
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /system/update/check");
    expect(shown.container.textContent).toContain("Checked ");
  });

  it("says a check sooner than a minute after the last waits, and a server with none has nothing to check", async () => {
    for (const [answer, said] of [
      [
        [429, { error: { code: "rateLimited", message: "checked just now" } }],
        "It was checked under a minute ago. Try again in a minute.",
      ],
      [
        [409, { error: { code: "conflict", message: "no release checker" } }],
        "This server reads no release record, so there is nothing to check.",
      ],
    ] as const) {
      mounted = await mount(
        withServices(
          <HowToUpdate />,
          servedBy({ "GET /system/update": [200, latest], "POST /system/update/check": answer }),
        ),
      );
      const shown = mounted;
      await settleUntil(() => buttonWithText(shown.container, "Check now") !== undefined);
      await click(buttonWithText(shown.container, "Check now"));
      await settleUntil(() => shown.container.querySelector('[role="alert"]') !== null);
      expect(shown.container.querySelector('[role="alert"]')?.textContent).toBe(said);
      mounted.unmount();
      mounted = undefined;
    }
  });

  it("says a reading that failed, and when one last succeeded", async () => {
    const container = await mountView({ ...latest, problem: "no discovery answered." });
    expect(container.textContent).toContain(
      "The last check for updates failed: no discovery answered. The last one that succeeded was ",
    );
    mounted?.unmount();
    const never = await mountView({
      version: "v0.3.3",
      checkedAt: 0,
      problem: "no discovery answered",
    });
    expect(never.textContent).toContain("None has succeeded yet.");
  });

  it("is not offered on a development build, which hears of no updates", async () => {
    mounted = await mount(
      withServices(
        <HowToUpdate />,
        servedBy({ "GET /system/update": [200, { version: "dev", checkedAt: 0 }] }),
      ),
    );
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes("development build"));
    expect(buttonWithText(shown.container, "Check now")).toBeUndefined();
  });
});

// D462 §3, §9, §10.1, §10.4: each way's own steps, Synology's Stop then Build with what to do while the
// old version still runs, and before an update, that it may bring a new file to paste after it.
describe("the steps of each way", () => {
  it("gives Synology's Stop then Build, and Image › Update while the old version still runs", async () => {
    const container = await mountView({ ...newer, installRoute: "synology" });
    expect(container.textContent).toContain(
      "under Project: select orpheum, press Stop, then Build.",
    );
    expect(container.textContent).toContain(
      "if this page still says the server runs v0.3.3 after that, Container Manager kept the old image: under Image, select orpheum, press Update, then Build the project again.",
    );
    expect(codes(container)).toEqual([]);
    expect(container.textContent).not.toContain("press Pull, then Up.");
  });

  it("gives Portainer, CasaOS and a household's own file the docker compose command", async () => {
    for (const installRoute of ["portainer", "casaos", "docker"] as const) {
      const container = await mountView({ ...newer, installRoute });
      expect(codes(container)).toEqual(["docker compose pull && docker compose up -d"]);
      expect(container.textContent).not.toContain("Container Manager");
      mounted?.unmount();
      mounted = undefined;
    }
  });

  it("says before a compose file's update that it may bring a new file, but not on the script's way", async () => {
    const omv = await mountView({ ...newer, installRoute: "omv" });
    expect(omv.textContent).toContain("The update may bring a new compose file.");
    mounted?.unmount();
    const script = await mountView({ ...newer, installRoute: "script" });
    expect(script.textContent).not.toContain("new compose file");
  });

  it("gives every way's steps while the server does not know its own, Synology's among them", async () => {
    const container = await mountView(newer);
    expect(container.textContent).toContain("press Pull, then Up.");
    expect(container.textContent).toContain("press Stop, then Build.");
    expect(codes(container)).toEqual([command, "docker compose pull && docker compose up -d"]);
  });
});

// D462 §10.7: a compose file pinning a version, which the NAS's update keeps pulling, is said with
// how to follow the releases again.
describe("a pinned version", () => {
  it("is said, with ORPHEUM_TAG to change, or on Synology the image lines", async () => {
    const omv = await mountView({ ...latest, installRoute: "omv", pinnedTag: "v0.3.2" });
    expect(omv.textContent).toContain(
      "Your compose file pins version v0.3.2 in place of latest, so pulling takes nothing newer. To follow each release again, set ORPHEUM_TAG to latest, then update.",
    );
    mounted?.unmount();
    const synology = await mountView({ ...latest, installRoute: "synology", pinnedTag: "v0.3.2" });
    expect(synology.textContent).toContain(
      "put latest back in place of v0.3.2 on the file's image lines",
    );
    mounted?.unmount();
    expect((await mountView(latest)).textContent).not.toContain("pins version");
  });
});

// D462 §4, §10.5: a newer compose file is handed over filled in, with Copy, the step that takes it on
// the server's way, and to keep a copy of the old one.
describe("a newer compose file", () => {
  const filled = "services:\n  orpheum-core:\n    # Yours\n    user: 1030:100\n";
  const withFile = (installRoute: Update["installRoute"]): Update => ({
    ...latest,
    ...(installRoute === undefined ? {} : { installRoute }),
    composeFile: {
      version: 1,
      newest: 2,
      brings: ["Subtitles from the media folders."],
      file: filled,
    },
  });

  it("is handed over filled in, to copy, with the step and the copy of the old one to keep", async () => {
    const writes: string[] = [];
    vi.stubGlobal("isSecureContext", true);
    vi.stubGlobal("navigator", {
      ...navigator,
      clipboard: {
        writeText: async (text: string) => {
          writes.push(text);
          await Promise.resolve();
        },
      },
    });
    const container = await mountView(withFile("synology"));
    expect(codes(container)).toEqual([filled]);
    expect(container.textContent).toContain("Keep a copy of your old compose file first");
    expect(container.textContent).toContain(
      "under Project: stop orpheum, open its YAML, paste this over it, save, then press Build.",
    );
    await click(buttonWithText(container, "Copy"));
    expect(writes).toEqual([filled]);
    expect(container.textContent).toContain("Copied.");
  });

  it("names the step of each way", async () => {
    expect((await mountView(withFile("omv"))).textContent).toContain(
      "edit the server's file, paste this over it, save, then press Up.",
    );
    mounted?.unmount();
    expect((await mountView(withFile("docker"))).textContent).toContain(
      "Paste this over your compose file, then, in its folder, run docker compose up -d.",
    );
  });

  it("says a copy the browser refused, the file still there to select", async () => {
    vi.stubGlobal("isSecureContext", false);
    const container = await mountView(withFile("omv"));
    await click(buttonWithText(container, "Copy"));
    await settleUntil(() => container.textContent.includes("would not copy it"));
    expect(codes(container)).toEqual([filled]);
  });
});
