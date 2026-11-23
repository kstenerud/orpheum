// SPDX-License-Identifier: MPL-2.0

/**
 * Browse, a library's Folders view (library-folders.tsx, D539 §3, §4), as a real browser runs it:
 * a click, a tap, the keyboard and a remote's arrows and OK open a folder or play a video where it
 * was left; a video's More offers its three entries, Play from the start playing it from 0; Back goes
 * up one folder, and leaves the page only at the library's top; the ring shows, and focus never
 * lands on nothing.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { LibraryBrowse } from "./library-browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: a list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

const homeMovies = {
  id: 5,
  name: "Home movies",
  kind: "other",
  metadataSource: "nfo",
  changeDetection: "watch",
  isWritable: true,
  createdAt: 0,
};

/** The folder 2024/Summer: a folder, Beach, and a video a quarter watched, matched to a title. */
const summer = {
  folder: "2024/Summer",
  subfolders: [{ name: "Beach", path: "2024/Summer/Beach", videoCount: 3 }],
  titles: [],
  videos: [
    {
      fileId: 7,
      name: "picnic.mp4",
      title: { id: 90, kind: "other", name: "Picnic", identificationState: "identified" },
      durationMs: 600_000,
      resumePositionMs: 150_000,
    },
  ],
};

let input: Input | undefined;
let mounted: Mounted | undefined;
let services: ReturnType<typeof servedBy> | undefined;
/** How many Backs went past the page, to leave it. */
let backsLeft = 0;

/** The Folders view of Home movies at folder. */
async function browseAt(folder: string | undefined): Promise<void> {
  services = servedBy({
    "GET /libraries": [200, { libraries: [homeMovies] }],
    "GET /libraries/5/folders": [200, { ...summer, folder: folder ?? "" }],
  });
  mounted = await mount(
    <BackgroundChangesContext value={neverAsks}>
      {withServices(
        <LibraryBrowse
          library={5}
          view="folders"
          folder={folder}
          sort={undefined}
          order={undefined}
          query=""
        />,
        services,
      )}
    </BackgroundChangesContext>,
  );
  await settleUntil(() => document.querySelector(".file-browser__row") !== null);
}

/** The row named name. */
function row(name: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(`.file-browser__row[aria-label="${name}"]`);
  if (found === null) {
    throw new Error(`no row ${name}`);
  }
  return found;
}

/** The route the page went to. */
function route(): unknown {
  return services?.navigation.route();
}

/** The entries of the menu open. */
function menuEntries(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>("dialog[open] .list-box__option")];
}

beforeEach(async () => {
  backsLeft = 0;
  input = startInput({
    goBack: () => {
      backsLeft += 1;
      return true;
    },
    isNavigating: () => true,
  });
  await page.viewport(1280, 800);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

describe("a library's Folders view, by every way of pressing it", () => {
  it("plays a video at a click where it was left, and opens a folder at a tap", async () => {
    await browseAt("2024/Summer");
    await userEvent.click(row("picnic.mp4"));
    expect(route()).toMatchObject({ screen: "play", fileId: 7, itemId: 90, atMs: 150_000 });
    tapOn(row("Beach"));
    await expect.poll(route).toMatchObject({ screen: "browse", folder: "2024/Summer/Beach" });
  });

  it("is walked by a remote: Down to the video, ringed, OK plays it where it was left", async () => {
    await browseAt("2024/Summer");
    row("Beach").focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("picnic.mp4");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(route()).toMatchObject({ screen: "play", fileId: 7, atMs: 150_000 });
  });

  it("offers in a video's More its three entries, and plays it from the start by the first", async () => {
    await browseAt("2024/Summer");
    const more = document.querySelector<HTMLElement>('[aria-label="More for picnic.mp4"]');
    await userEvent.click(more ?? document.body);
    await expect
      .poll(() => menuEntries().map((each) => each.textContent.trim()))
      .toEqual(["Play from the start", "Open its title page", "Show in Files"]);
    await userEvent.keyboard("{Enter}");
    expect(route()).toMatchObject({ screen: "play", fileId: 7, atMs: 0 });
  });

  it("goes up one folder by Back, the page staying, and leaves only at the library's top", async () => {
    await browseAt("2024/Summer");
    row("Beach").focus();
    await userEvent.keyboard("{Backspace}");
    expect(route()).toMatchObject({ screen: "browse", folder: "2024" });
    expect(backsLeft).toBe(0);
    mounted?.unmount();
    await browseAt(undefined);
    row("Beach").focus();
    await userEvent.keyboard("{Backspace}");
    expect(backsLeft).toBe(1);
  });

  it("draws its rows a fingertip tall and inside the screen at each width", async () => {
    await browseAt("2024/Summer");
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const box = row("picnic.mp4").getBoundingClientRect();
      expect(box.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(box.right, `${String(width)} px`).toBeLessThanOrEqual(width);
    }
  });
});
