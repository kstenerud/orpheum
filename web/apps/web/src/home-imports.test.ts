// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

/** Every source file of the app, as text, tests left out, by its path. */
// Untyped at the call: vitest keeps no cached transform of a file holding "import.meta.glob(", whose
// matches change as files come and go; one given its type at the call escaped that, and a test read
// a stale list of sources (2026-10-26; glob-cache.test.ts).
const sources: Record<string, string> = import.meta.glob(
  ["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}"],
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
);

/** What a source imports as it loads: its static imports, never `import type` or `import()`. */
function staticImportsOf(source: string): string[] {
  const imports = source.matchAll(/^import\s+(?!type\s)(?:[^;]*?\sfrom\s+)?"([^"]+)";/gmsu);
  return [...imports].map((each) => each[1] ?? "");
}

/** The app's own path that specifier names from the file at path, or undefined for a package. */
function ownPathOf(path: string, specifier: string): string | undefined {
  if (!specifier.startsWith("./")) {
    return undefined;
  }
  const directory = path.slice(0, path.lastIndexOf("/"));
  return `${directory}/${specifier.slice(2)}`;
}

/** Each of the app's files entry loads as it loads, the home page's by default, by what loaded it first. */
function loadedAtStart(entry = "./client-main.tsx"): Map<string, string> {
  const loadedBy = new Map<string, string>([[entry, ""]]);
  const toRead = [entry];
  for (let path = toRead.pop(); path !== undefined; path = toRead.pop()) {
    toRead.push(...newlyLoaded(path, loadedBy));
  }
  return loadedBy;
}

/** What path loads that nothing loaded before, noted in loadedBy; the app's own of them, to read. */
function newlyLoaded(path: string, loadedBy: Map<string, string>): string[] {
  const own: string[] = [];
  for (const specifier of staticImportsOf(sources[path] ?? "")) {
    const ownPath = ownPathOf(path, specifier);
    const key = ownPath ?? specifier;
    if (!loadedBy.has(key)) {
      loadedBy.set(key, path);
      own.push(...(ownPath === undefined ? [] : [ownPath]));
    }
  }
  return own;
}

/** How what loaded path came to load it, the client's entry first. */
function chainTo(loadedBy: ReadonlyMap<string, string>, path: string): string {
  const chain = [path];
  for (let by = loadedBy.get(path); by !== undefined && by !== ""; by = loadedBy.get(by)) {
    chain.unshift(by);
  }
  return chain.join(" → ");
}

describe("what the home page loads before it draws (the audit of 2026-10-25, W1)", () => {
  it("is no player, hls.js with it, and no device lab but whether it is offered: their screens load them", () => {
    const loadedBy = loadedAtStart();
    // The client's own reading of its files is complete: it reached the shell and Home.
    expect(loadedBy.has("./app-shell.tsx")).toBe(true);
    expect(loadedBy.has("./home.tsx")).toBe(true);
    const unwanted = [...loadedBy.keys()].filter(
      (path) =>
        path === "@orpheum/player" ||
        path === "hls.js" ||
        (/^\.\/lab[-.]/u.test(path) && path !== "./lab-offered.ts"),
    );
    expect(unwanted.map((path) => chainTo(loadedBy, path))).toEqual([]);
  });
});

describe("what the first page loads before it draws (the efficiency audit of 2026-10-28, finding 10)", () => {
  it("is no admin's or System screen, nor the lab's or the gallery's words: they load with their screens", () => {
    const loadedBy = loadedAtStart();
    expect(loadedBy.has("./later-screens.tsx")).toBe(true);
    const later = [
      "./identify.tsx",
      "./jobs.tsx",
      "./system.tsx",
      "./update-page.tsx",
      "./needs-attention.tsx",
      "./users.tsx",
      "./user.tsx",
      "./libraries.tsx",
      "./library-manage.tsx",
      "./create-library.tsx",
      "./files.tsx",
      "./settings.tsx",
      "./locales/en-lab.ts",
      "./locales/en-system.ts",
      "./locales/en-libraries.ts",
      "./locales/en-people.ts",
      "./locales/en-gallery.ts",
      // The owner, 2026-11-13: the settings pages' words load with them.
      "./locales/en-settings.ts",
      "./locales/en-setting-labels.ts",
      // The architecture audit of 2026-11-10, Startup 3: no start draws these first.
      "./about.tsx",
      "./account.tsx",
      "./credits.tsx",
      "./library-browse.tsx",
      "./old-addresses.tsx",
      "./register.tsx",
      "./sign-in.tsx",
      "./this-device.tsx",
    ];
    expect(
      later.filter((path) => loadedBy.has(path)).map((path) => chainTo(loadedBy, path)),
    ).toEqual([]);
  });
});

describe("what the shell's entry and the update's check load (the architecture audit of 2026-11-10, Startup 5)", () => {
  it("is no discovery's start and no SDK: a discovery's page's meta is read by discovery-page.ts alone", () => {
    const reached = ["./main.tsx", "./app-update.ts", "./client-reaches.ts"].flatMap((entry) => {
      const loadedBy = loadedAtStart(entry);
      return ["./discovery-start.ts", ...(entry === "./client-reaches.ts" ? [] : ["@orpheum/sdk"])]
        .filter((path) => loadedBy.has(path))
        .map((path) => chainTo(loadedBy, path));
    });
    expect(loadedAtStart("./main.tsx").has("./discovery-page.ts")).toBe(true);
    expect(reached).toEqual([]);
  });
});

describe("what the client loads before it draws (the architecture audit of 2026-11-10, Startup 6)", () => {
  it("is no WebRTC path: a tab that asked for it loads it in the asking's branch", () => {
    const loadedBy = loadedAtStart();
    expect(loadedBy.has("./transport-wanted.ts")).toBe(true);
    expect(
      loadedBy.has("./transport-choice.ts") ? chainTo(loadedBy, "./transport-choice.ts") : "",
    ).toBe("");
    expect(sources["./client-main.tsx"]).toContain('import("./transport-choice.ts")');
  });
});

// The test-regime audit of 2026-10-31 (R4): test-dom.tsx imported later-screens.tsx for its reset,
// which made every test that mounts anything "related" to every screen, half the suite rerun by
// revert-red for one changed screen. A helper reaches a screen's code only as the test itself does
// (test-walk.tsx mounts the app, so it does).
describe("what the tests' shared helpers load", () => {
  it("is no module of lazily loaded screens: a test that mounts a screen imports it itself", () => {
    const helpers = Object.keys(sources).filter(
      (path) => /^\.\/test-[^/]+\.tsx?$/u.test(path) && path !== "./test-walk.tsx",
    );
    expect(helpers).toContain("./test-dom.tsx");
    const reached = helpers.flatMap((helper) => {
      const loadedBy = loadedAtStart(helper);
      return ["./later-screens.tsx", "./tray-dialog-code.tsx", "./together-dialog-code.tsx"]
        .filter((path) => loadedBy.has(path))
        .map((path) => chainTo(loadedBy, path));
    });
    expect(reached).toEqual([]);
  });
});

// The web client's architecture audit of 2026-11-10, Playback 2: a film's page and a series' page
// imported the player's entry for what they read of the sound, and Up next's and the play state's
// code for a type and a list, so each waited on hls.js, 584 KB, before it drew. They read the sound
// from the player's heard-volume.ts, which the build keeps a chunk of its own, and the player loads
// as Play nears (warm-play.ts, watching.ts).
const heardVolume: Record<string, string> = import.meta.glob(
  "../../../packages/player/src/heard-volume.ts",
  { query: "?raw", import: "default", eager: true },
);

/** The names a source imports at load from the player's entry, types left out. */
function playerNamesOf(source: string): string[] {
  const imports = source.matchAll(/^import\s+\{([^}]*)\}\s+from\s+"@orpheum\/player";/gmsu);
  return [...imports].flatMap((each) =>
    (each[1] ?? "")
      .split(",")
      .map((name) => name.trim())
      .filter((name) => name !== "" && !name.startsWith("type ")),
  );
}

describe("what a film's and a series' page load before they draw", () => {
  const heardVolumeSource = Object.values(heardVolume)[0] ?? "";

  it("is of the player only what its heard-volume.ts holds, which imports nothing", () => {
    expect(heardVolumeSource).toContain("export function isWebAudioLate(");
    expect(heardVolumeSource).not.toMatch(/^import\s/mu);
    for (const entry of ["./title.tsx", "./series.tsx"]) {
      const loadedBy = loadedAtStart(entry);
      expect(loadedBy.has("./watching.ts")).toBe(true);
      const outside = [...loadedBy.keys()]
        .filter((path) => path.startsWith("./"))
        .flatMap((path) =>
          playerNamesOf(sources[path] ?? "")
            .filter((name) => !heardVolumeSource.includes(`export function ${name}(`))
            .map((name) => `${name}: ${chainTo(loadedBy, path)}`),
        );
      expect(outside).toEqual([]);
      const playing = ["./up-next.tsx", "./play-state-video-events.ts", "./moving-down.ts"];
      expect(
        playing.filter((path) => loadedBy.has(path)).map((path) => chainTo(loadedBy, path)),
      ).toEqual([]);
    }
  });

  it("finds the player's heard-volume.ts a chunk of its own in the build", async () => {
    const { default: config } = await import("../vite.config.ts");
    const resolved = await Promise.resolve(
      config({ command: "build", mode: "production", isSsrBuild: false, isPreview: false }),
    );
    const output = resolved.build?.rolldownOptions?.output;
    const splitting = Array.isArray(output) ? undefined : output?.codeSplitting;
    const groups = typeof splitting === "object" ? (splitting.groups ?? []) : [];
    const path = "/repo/web/packages/player/src/heard-volume.ts";
    expect(
      groups.filter((group) => group.test instanceof RegExp && group.test.test(path)),
    ).toHaveLength(1);
  });
});

describe("what the loader loads before it draws (web architecture audit, 2026-11-10, startup 2)", () => {
  it("is neither origin's role: each is loaded when the page is found to be at its origin", () => {
    const loadedBy = loadedAtStart("./loader-main.tsx");
    expect(loadedBy.has("./part-load.ts")).toBe(true);
    const roles = ["./loader-at-loader-origin.tsx", "./loader-at-client-origin.tsx"];
    expect(
      roles.filter((path) => loadedBy.has(path)).map((path) => chainTo(loadedBy, path)),
    ).toEqual([]);
    expect(sources["./loader-main.tsx"]).toContain('import("./loader-at-loader-origin.tsx")');
    expect(sources["./loader-main.tsx"]).toContain('import("./loader-at-client-origin.tsx")');
  });
});

// The audit's Startup 5 and 6 moved what the shell and every tab read out of the discovery's start
// and the WebRTC path, which load later; one left defined there too would be a second way to read
// the same thing (AGENTS.md rule 11), and the next change would mend one of them.
describe("what the shell reads of the page and the address, moved out of what loads later", () => {
  it("is defined in one module each", () => {
    const moved: Record<string, string> = {
      discoveryClientApiOf: "./discovery-page.ts",
      discoveryHasRealms: "./discovery-page.ts",
      transportParameter: "./transport-wanted.ts",
      pathName: "./transport-wanted.ts",
      wantsWebrtc: "./transport-wanted.ts",
    };
    const definedIn = Object.keys(moved).map((name) => {
      const defining = new RegExp(`^export (?:async )?(?:function|const) ${name}\\b`, "mu");
      return [
        name,
        Object.entries(sources)
          .filter(([path, source]) => path.startsWith("./") && defining.test(source))
          .map(([path]) => path),
      ];
    });
    expect(Object.fromEntries(definedIn)).toEqual(
      Object.fromEntries(Object.entries(moved).map(([name, path]) => [name, [path]])),
    );
  });
});
