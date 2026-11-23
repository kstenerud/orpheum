// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { i18n } from "./i18n.ts";

import type { TFunction } from "i18next";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import {
  isFound,
  isStepped,
  parseSettingText,
  SectionSettingsView,
  settingDescription,
  settingLabel,
  settingText,
  startText,
  type Setting,
} from "./server-settings.tsx";
import { KeysView } from "./settings-keys.tsx";
import { advancedShownKey, foundBySearch, Settings } from "./settings.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  enter,
  isSwitchOn,
  mount,
  servedBy,
  settleAnswers,
  settleUntil,
  submit,
  switchNamed,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  localStorage.clear();
});

/** A setting as the server sends it, important and at its starting value unless changed. */
function setting(changed: Partial<Setting> & Pick<Setting, "key">): Setting {
  return {
    kind: "integer",
    description: "a server description, never shown while the catalogue has one",
    value: 1,
    startingValue: 1,
    isModified: false,
    isImportant: true,
    section: "together",
    setAt: 1,
    ...changed,
  };
}

/** The catalogue's words, as the page reads them. */
function wordsOf(): TFunction {
  return i18n.t.bind(i18n);
}

/** A setting's row in container, by its key. */
function rowOf(container: HTMLElement, key: string): HTMLElement {
  const row = container.querySelector<HTMLElement>(`#setting-${key}`);
  if (row === null) {
    throw new Error(`no row for ${key}`);
  }
  return row;
}

/**
 * Opens a setting's What it is for dialog by its key and returns what it says the setting is for:
 * the catalogue's short words, then its longer ones where it has any, without the lines that follow
 * them, its name in the configuration and its default.
 */
async function paragraphsFor(key: string): Promise<readonly (string | null)[]> {
  mounted?.unmount();
  mounted = await mount(
    withServices(
      <SectionSettingsView
        section="together"
        isAdvanced={false}
        problems={new Map()}
        onSave={ignore}
        onReset={ignore}
        settings={[setting({ key })]}
      />,
    ),
  );
  await click(
    buttonNamed(mounted.container, `What ${settingLabel(wordsOf(), "labels", key)} is for`),
  );
  return [...mounted.container.querySelectorAll("dialog .setting-info p")]
    .map((p) => p.textContent)
    .slice(0, -2);
}

describe("settings' saving (D538 §9)", () => {
  it("saves nothing typed and left unsaved as the page closes or the person moves away", async () => {
    const saved: string[] = [];
    const record = (key: string): void => {
      saved.push(key);
    };
    mounted = await mount(
      withServices(
        <>
          <SectionSettingsView
            section="diagnostics"
            isAdvanced
            problems={new Map()}
            onSave={record}
            onReset={ignore}
            settings={[
              setting({
                key: "logLevel",
                kind: "string",
                value: "info",
                startingValue: "info",
                section: "diagnostics",
              }),
            ]}
          />
          <KeysView
            problems={new Map()}
            onSave={record}
            onRemove={ignore}
            secrets={[
              { name: "tmdbApiKey", description: "key", isSet: false, needsReentry: false },
            ]}
          />
        </>,
      ),
    );
    const fields = mounted.container.querySelectorAll("input");
    expect(fields).toHaveLength(2);
    await enter(fields[0], "debug");
    await enter(fields[1], "a new key");
    window.dispatchEvent(new Event("pagehide"));
    mounted.unmount();
    mounted = undefined;
    expect(saved).toEqual([]);
  });
});

describe("a setting's control (D538 §1, the owner, 2026-11-22)", () => {
  it("saves a switch as it changes, and says a question not yet answered is one", async () => {
    const saved: [string, unknown][] = [];
    mounted = await mount(
      withServices(
        <SectionSettingsView
          section="general"
          isAdvanced={false}
          problems={new Map()}
          onSave={(key, value) => {
            saved.push([key, value]);
          }}
          onReset={ignore}
          settings={[
            setting({
              key: "shouldCheckForUpdates",
              kind: "bool",
              value: null,
              startingValue: null,
              section: "general",
            }),
          ]}
        />,
      ),
    );
    const shown = rowOf(mounted.container, "shouldCheckForUpdates").querySelector(
      '[role="switch"]',
    );
    expect(isSwitchOn(shown)).toBe(false);
    expect(rowOf(mounted.container, "shouldCheckForUpdates").textContent).toContain("unanswered");
    await click(shown);
    expect(saved).toEqual([["shouldCheckForUpdates", true]]);
  });

  it("steps a narrow whole number, saving each step, and types a wide one, its range its hint", async () => {
    const saved: [string, unknown][] = [];
    mounted = await mount(
      withServices(
        <SectionSettingsView
          section="general"
          isAdvanced={false}
          problems={new Map()}
          onSave={(key, value) => {
            saved.push([key, value]);
          }}
          onReset={ignore}
          settings={[
            setting({
              key: "backupEveryHours",
              value: 24,
              startingValue: 24,
              minimum: 1,
              maximum: 168,
              section: "general",
            }),
            setting({
              key: "tmdbRefreshAfterDays",
              value: 90,
              startingValue: 180,
              isModified: true,
              minimum: 0,
              maximum: 36500,
              section: "general",
            }),
          ]}
        />,
      ),
    );
    const { container } = mounted;
    const backup = rowOf(container, "backupEveryHours");
    expect(backup.querySelector(".stepper")).not.toBeNull();
    await click(buttonNamed(backup, "Back up every (hours): one more"));
    expect(saved).toEqual([["backupEveryHours", 25]]);
    const refresh = rowOf(container, "tmdbRefreshAfterDays");
    expect(refresh.querySelector(".stepper")).toBeNull();
    const field = refresh.querySelector("input");
    expect(field?.type).toBe("number");
    expect(refresh.textContent).toContain("From 0 to 36500");
    await enter(field, "30");
    expect(saved).toHaveLength(1);
    await submit(refresh.querySelector("form"));
    expect(saved).toEqual([
      ["backupEveryHours", 25],
      ["tmdbRefreshAfterDays", 30],
    ]);
  });

  it("saves typing on leaving the field, once, and not again for nothing changed", async () => {
    const saved: [string, unknown][] = [];
    mounted = await mount(
      withServices(
        <SectionSettingsView
          section="diagnostics"
          isAdvanced={false}
          problems={new Map()}
          onSave={(key, value) => {
            saved.push([key, value]);
          }}
          onReset={ignore}
          settings={[
            setting({
              key: "logLevel",
              kind: "string",
              value: "info",
              startingValue: "info",
              section: "diagnostics",
            }),
          ]}
        />,
      ),
    );
    const field = rowOf(mounted.container, "logLevel").querySelector("input");
    await enter(field, "debug");
    field?.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    field?.dispatchEvent(new FocusEvent("blur"));
    await settleAnswers();
    field?.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    await settleAnswers();
    expect(saved).toEqual([["logLevel", "debug"]]);
  });

  it("puts a changed setting back by its row's Use the default, asked first, What it is for naming the default", async () => {
    const reset: string[] = [];
    mounted = await mount(
      withServices(
        <SectionSettingsView
          section="metadata"
          isAdvanced={false}
          problems={new Map()}
          onSave={ignore}
          onReset={(key) => {
            reset.push(key);
          }}
          settings={[
            setting({
              key: "tmdbRefreshAfterDays",
              value: 90,
              startingValue: 180,
              isModified: true,
              minimum: 0,
              maximum: 36500,
              section: "metadata",
            }),
            setting({ key: "togetherIdleSessionEndHours", section: "metadata" }),
          ]}
        />,
      ),
    );
    const { container } = mounted;
    await click(buttonNamed(container, "What Refresh details from TMDB after (days) is for"));
    const dialog = container.querySelector("dialog");
    expect(dialog?.querySelector("h2, h1")?.textContent).toBe(
      "Refresh details from TMDB after (days)",
    );
    expect(dialog?.textContent).toContain("tmdbRefreshAfterDays");
    expect(dialog?.textContent).toContain("The default is 180.");
    // What it is for only tells (the owner, 2026-11-23, revamp question 16): Done alone.
    expect(
      [...(dialog?.querySelectorAll(".modal-dialog__foot button") ?? [])].map((b) => b.textContent),
    ).toEqual(["Done"]);
    await click(buttonWithText(dialog instanceof HTMLElement ? dialog : container, "Done"));
    // The row's own Use the default asks, then puts it back.
    await click(
      buttonNamed(container, "Use the default for Refresh details from TMDB after (days)"),
    );
    expect(reset).toEqual([]);
    await click(
      buttonWithText(
        container.querySelector<HTMLElement>("dialog") ?? container,
        "Use the default",
      ),
    );
    expect(reset).toEqual(["tmdbRefreshAfterDays"]);
    expect(container.querySelector("dialog")).toBeNull();
    // A setting at its default has nothing to put back: no button beside it.
    expect(
      buttonNamed(
        container,
        "Use the default for End an idle Watch Together session after (hours)",
      ),
    ).toBeUndefined();
    expect(container.querySelectorAll('button[aria-label^="Use the default for"]')).toHaveLength(1);
  });

  it("is a Stepper only for a whole number whose bounds are no more than 200 apart", () => {
    expect(isStepped(setting({ key: "a", minimum: 1, maximum: 168 }))).toBe(true);
    expect(isStepped(setting({ key: "a", minimum: 0, maximum: 200 }))).toBe(true);
    expect(isStepped(setting({ key: "a", minimum: 0, maximum: 201 }))).toBe(false);
    expect(isStepped(setting({ key: "a", minimum: 1 }))).toBe(false);
    expect(isStepped(setting({ key: "a", kind: "string", minimum: 1, maximum: 2 }))).toBe(false);
  });
});

describe("a section's page (pages-spec-3)", () => {
  it("lists the important settings, the rest behind Show advanced settings, remembered in this browser", async () => {
    const services = servedBy({
      "GET /config": [
        200,
        {
          settings: [
            setting({
              key: "shouldCheckForUpdates",
              kind: "bool",
              value: true,
              section: "general",
            }),
            setting({
              key: "shouldScanOnNodeHello",
              kind: "bool",
              value: true,
              isImportant: false,
              section: "general",
            }),
            setting({ key: "logLevel", kind: "string", value: "info", section: "diagnostics" }),
          ],
        },
      ],
    });
    mounted = await mount(withServices(<Settings page="general" />, services));
    const first = mounted;
    await settleUntil(
      () => first.container.querySelector("#setting-shouldCheckForUpdates") !== null,
    );
    expect(first.container.querySelector("h1")?.textContent).toBe("General");
    expect(first.container.querySelector("#setting-shouldScanOnNodeHello")).toBeNull();
    // Another section's settings are on its own page.
    expect(first.container.querySelector("#setting-logLevel")).toBeNull();
    await click(switchNamed(first.container, "Show advanced settings"));
    expect(first.container.querySelector("#setting-shouldScanOnNodeHello")).not.toBeNull();
    expect(localStorage.getItem(advancedShownKey)).toBe("true");
    first.unmount();
    mounted = await mount(withServices(<Settings page="diagnostics" />, services));
    expect(isSwitchOn(switchNamed(mounted.container, "Show advanced settings"))).toBe(true);
  });

  it("shows an advanced setting the address names, for focus to arrive at", async () => {
    const services = servedBy({
      "GET /config": [
        200,
        {
          settings: [
            setting({
              key: "shouldScanOnNodeHello",
              kind: "bool",
              value: true,
              isImportant: false,
              section: "libraries",
            }),
          ],
        },
      ],
    });
    mounted = await mount(
      withServices(<Settings page="libraries" setting="shouldScanOnNodeHello" />, services),
    );
    const shown = mounted;
    await settleUntil(
      () => shown.container.querySelector("#setting-shouldScanOnNodeHello") !== null,
    );
    expect(
      shown.container
        .querySelector("#setting-shouldScanOnNodeHello")
        ?.getAttribute("data-focus-start"),
    ).toBe("arrival");
  });

  it("leads Remote access to Network for the address in the home, which it does not list", async () => {
    const services = servedBy({
      "GET /config": [
        200,
        {
          settings: [
            setting({
              key: "addressAtHome",
              kind: "string",
              value: "",
              section: "remoteAccess",
            }),
            setting({
              key: "shouldMapPorts",
              kind: "bool",
              value: true,
              section: "remoteAccess",
            }),
          ],
        },
      ],
    });
    mounted = await mount(withServices(<Settings page="remoteAccess" />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.querySelector("#setting-shouldMapPorts") !== null);
    expect(shown.container.querySelector('a[href="/network"]')?.textContent).toBe(
      "The home address and ports are set on Network",
    );
    expect(shown.container.querySelector("#setting-addressAtHome")).toBeNull();
  });

  // ui-audit-2026-11-04.md §3, Settings; the owner, 2026-11-04, Q5: every setting saves at once.
  it("says Saved once a change saves", async () => {
    const logLevel = setting({
      key: "logLevel",
      kind: "string",
      value: "info",
      startingValue: "info",
      section: "diagnostics",
    });
    const services = servedBy({
      "GET /config": [200, { settings: [logLevel] }],
      "PUT /config": [200, { settings: [{ ...logLevel, value: "debug", isModified: true }] }],
    });
    mounted = await mount(withServices(<Settings page="diagnostics" />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.querySelector("#setting-logLevel") !== null);
    const row = rowOf(shown.container, "logLevel");
    const field = row.querySelector("input");
    expect([...(field?.labels ?? [])].map((label) => label.textContent)).toEqual(["Log level"]);
    expect(row.querySelector("button[type=submit]")).toBeNull();
    expect(row.textContent).not.toContain("Saved");
    await enter(field, "debug");
    await submit(row.querySelector("form"));
    await settleAnswers();
    expect(row.querySelector("output.status-line--success")?.textContent).toBe("Saved");
  });
});

describe("Settings' index (pages-spec-3)", () => {
  it("lists Keys and passwords with its keys' states, then each section with its line, under a search landmark", async () => {
    const services = servedBy({
      "GET /config": [200, { settings: [] }],
      "GET /needs-attention": [200, { items: [] }],
      "GET /config/unused": [200, { unusedSettings: [] }],
      "GET /secrets": [
        200,
        {
          secrets: [
            {
              name: "tmdbApiKey",
              description: "k",
              isSet: true,
              lastChars: "a1b2",
              needsReentry: false,
            },
            { name: "opensubtitlesPassword", description: "p", isSet: false, needsReentry: false },
          ],
        },
      ],
    });
    mounted = await mount(withServices(<Settings />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes("TMDB API key set"));
    expect(
      shown.container.querySelector("search.settings__filters input[type=search]"),
    ).not.toBeNull();
    const rows = [...shown.container.querySelectorAll(".settings-index__row")];
    expect(rows[0]?.getAttribute("href")).toBe("/settings/keys");
    expect(rows[0]?.textContent).toContain("TMDB API key set · OpenSubtitles password not set");
    expect(rows[1]?.querySelector(".settings-index__name")?.textContent).toBe("General");
    expect(rows[1]?.querySelector(".settings-index__summary")?.textContent).not.toBe("");
    // Updates and How to update are the Update page's now (pages-spec-3).
    expect(shown.container.querySelector('a[href="/update"]')).toBeNull();
  });

  it("leads each key's Needs you line to Keys and passwords, at the key a line names (D538 §11)", async () => {
    const services = servedBy({
      "GET /config": [200, { settings: [] }],
      "GET /secrets": [200, { secrets: [] }],
      "GET /config/unused": [200, { unusedSettings: [] }],
      "GET /needs-attention": [
        200,
        {
          pages: {
            settings: {
              isProblem: true,
              itemCount: 3,
              items: [
                { kind: "keyRefused", isProblem: true, name: "TMDB" },
                { kind: "secretNeedsReentry", isProblem: true, name: "tmdbApiKey" },
                { kind: "keyMissing", isProblem: false, name: "OpenSubtitles" },
              ],
            },
          },
        },
      ],
    });
    mounted = await mount(withServices(<Settings />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.querySelector(".attention-list a") !== null);
    const lines = [...shown.container.querySelectorAll(".attention-list a")].map((link) => [
      link.textContent,
      link.getAttribute("href"),
    ]);
    expect(lines).toEqual([
      ["TMDB refused its key or login.", "/settings/keys"],
      ["TMDB API key must be entered again.", "/settings/keys?setting=tmdbApiKey"],
      ["OpenSubtitles has no key or login set, so the server can't use it.", "/settings/keys"],
    ]);
  });

  it("finds settings and keys by every word, advanced ones too, each leading to its row, the address in the home to Network", () => {
    const t = wordsOf();
    const settings = [
      setting({ key: "logLevel", kind: "string", value: "info", section: "diagnostics" }),
      setting({ key: "backupEveryHours", isImportant: false, section: "general" }),
      setting({ key: "addressAtHome", kind: "string", value: "", section: "remoteAccess" }),
    ];
    const secrets = [{ name: "tmdbApiKey", description: "k", isSet: false, needsReentry: false }];
    expect(foundBySearch(t, "back up", { settings, secrets }).map((found) => found.to)).toEqual([
      { screen: "settings", page: "general", setting: "backupEveryHours" },
    ]);
    expect(foundBySearch(t, "TMDB", { settings, secrets })[0]?.to).toEqual({
      screen: "settings",
      page: "keys",
      setting: "tmdbApiKey",
    });
    expect(foundBySearch(t, "address in the home", { settings, secrets })).toEqual([
      {
        key: "addressAtHome",
        label: "Address in the home",
        pageName: "Network",
        to: { screen: "network" },
      },
    ]);
    expect(foundBySearch(t, "", { settings, secrets }).map((found) => found.key)).toEqual([
      "tmdbApiKey",
      "backupEveryHours",
      "addressAtHome",
      "logLevel",
    ]);
  });

  it("finds a setting by every word searched for, in any of its texts", () => {
    expect(isFound("drift together", ["togetherDriftLeftAloneMs", "How far it may drift"])).toBe(
      true,
    );
    expect(isFound("SEEK", ["togetherDriftLeftAloneMs", "How far it may drift"])).toBe(false);
    expect(isFound("  ", ["any_key", ""])).toBe(true);
  });
});

describe("Keys and passwords (pages-spec-3)", () => {
  it("shows whether a key is set and an API key's last four, never its value, and when it must be entered again", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <KeysView
          problems={new Map([["tmdbApiKey", "Refused."]])}
          onSave={ignore}
          onRemove={ignore}
          secrets={[
            {
              name: "tmdbApiKey",
              description: "TMDB's API key",
              isSet: true,
              lastChars: "a1b2",
              needsReentry: false,
            },
            {
              name: "opensubtitlesPassword",
              description: "OpenSubtitles",
              isSet: false,
              needsReentry: true,
            },
          ]}
        />,
      ),
    );
    expect(markup).toContain("Set, ending in a1b2");
    // Whether a key is set is part of its row's heading, never a placeholder, and its field is empty.
    expect(markup).not.toContain("placeholder=");
    expect(markup).not.toMatch(/value="[^"]/u);
    expect(markup).toContain('<span class="settings__secret-state">Set, ending in a1b2</span>');
    expect(markup).toContain("Not set");
    expect(markup).toContain("Must be entered again");
    expect(markup).toContain('type="password"');
    expect(markup).toContain('role="alert">');
    expect(markup).toContain('<span class="status-line__words">Refused.</span>');
    // Only a key that is set has Remove… to press.
    expect(markup.match(/>Remove…</gu)).toHaveLength(1);
  });

  it("asks before removing a key, and removes it only on Remove it", async () => {
    const removed: string[] = [];
    mounted = await mount(
      withServices(
        <KeysView
          problems={new Map()}
          onSave={ignore}
          onRemove={(name) => {
            removed.push(name);
          }}
          secrets={[
            {
              name: "tmdbApiKey",
              description: "k",
              isSet: true,
              lastChars: "a1b2",
              needsReentry: false,
            },
          ]}
        />,
      ),
    );
    const { container } = mounted;
    await click(buttonNamed(container, "Remove…"));
    expect(container.querySelector("dialog")?.textContent).toContain("Remove the TMDB API key?");
    expect(removed).toEqual([]);
    await click(buttonNamed(container, "Remove it"));
    expect(removed).toEqual(["tmdbApiKey"]);
  });

  it("saves a key on Enter and empties its field, saving nothing for an empty one", async () => {
    const saved: [string, string][] = [];
    mounted = await mount(
      withServices(
        <KeysView
          problems={new Map()}
          onSave={(name, value) => {
            saved.push([name, value]);
          }}
          onRemove={ignore}
          secrets={[{ name: "tmdbApiKey", description: "k", isSet: false, needsReentry: false }]}
        />,
      ),
    );
    const form = mounted.container.querySelector("form");
    await submit(form);
    expect(saved).toEqual([]);
    await enter(form?.querySelector("input"), "a new key");
    await submit(form);
    expect(saved).toEqual([["tmdbApiKey", "a new key"]]);
    expect(form?.querySelector("input")?.value).toBe("");
  });
});

describe("settings' words", () => {
  it("names a starting value in words: on or off, nothing, or unanswered", () => {
    const t = ((key: string): string => key.replace("settings.", "")) as unknown as TFunction;
    expect(startText(t, true)).toBe("on");
    expect(startText(t, false)).toBe("off");
    expect(startText(t, "")).toBe("nothing");
    expect(startText(t, null)).toBe("unanswered");
    expect(startText(t, 90)).toBe("90");
    expect(parseSettingText("integer", "30")).toBe(30);
    expect(parseSettingText("bool", "false")).toBe(false);
    expect(settingText(null)).toBe("");
  });

  it("says what a setting is for in the catalogue's words, the server's where it has none", async () => {
    expect(await paragraphsFor("tmdbRefreshAfterDays")).toEqual([
      "How long a title's TMDB data is kept before it is fetched again. 0 never fetches again.",
    ]);
    expect(await paragraphsFor("a_setting_the_catalogue_lacks")).toEqual([
      "a server description, never shown while the catalogue has one",
    ]);
  });

  it("says a Watch Together invitation is short-lived and an idle session ends (D346, D339)", async () => {
    expect(await paragraphsFor("togetherLinkLifetimeSeconds")).toEqual([
      "How long a Watch Together link lets its person in.",
      "A Watch Together invitation is short-lived: a link not used within this long of being made lets no one in. Whoever already joined by it stays.",
    ]);
    expect(await paragraphsFor("togetherIdleSessionEndHours")).toEqual([
      "How long a Watch Together session with nothing playing and no one doing anything lasts before it ends.",
      "A host may forget to end their session. Once nothing has played in it and no one has done anything in it for this long, it ends as if the host had ended it, and its links stop working.",
    ]);
  });

  it("says what the address at home is and where it came from, in the person's words (D452 §3)", () => {
    const t = wordsOf();
    expect(settingDescription(t, setting({ key: "addressAtHome" }))).toBe(
      "The server's address in your home, which devices there reach it at directly. It is learned from a device at home that opens the server by its address, or set on the System page; empty until then.",
    );
    expect(settingLabel(t, "labels", "addressAtHome")).toBe("Address in the home");
    expect(settingDescription(t, setting({ key: "addressAtHomeLearnedFrom" }))).toBe(
      "The device the address in your home was learned from. Empty when an admin set it.",
    );
    expect(settingLabel(t, "labels", "addressAtHomeLearnedFrom")).toBe(
      "Address in the home learned from",
    );
  });

  it("says a film is converted, not prepared, now the whole-film prepare is retired (D344), its pieces' length a floor its keyframes decide (D414 §5)", async () => {
    expect(await paragraphsFor("hlsSegmentLengthSeconds")).toEqual([
      "The shortest piece a converted film is cut into; its keyframes decide where each piece ends. Changing it cuts films again when they are next played.",
      "A converted film is sent in short pieces. A piece can only start where the film has a keyframe, so this is the shortest a piece is made, not its length: most of a film usually plays from pieces of 6 to 7 seconds, and pieces of 10 seconds or more are common. Shorter pieces let the dialogue boost and other sound changes start sooner; longer ones make fewer requests. Changing it cuts each film again the next time it's played.",
    ]);
    const playLadder = await paragraphsFor("playLadderWaitMs");
    expect(playLadder[1]).toBe(
      "A film is cut into pieces where its own keyframes fall once they are read. Until then, playback waits up to this long, then plays the original where the device can, or cuts the film at even times instead.",
    );
    expect(await paragraphsFor("nodeProgressEverySeconds")).toEqual([
      "How often, at most, a media node reports how far a read-through in progress is.",
    ]);
  });

  it("says the server's and its media machines' log level can be changed at once (D347, D525)", async () => {
    expect(await paragraphsFor("logLevel")).toEqual([
      "How much the server and each of its media machines write to their logs, changed at once, a media machine's within half a minute: debug, info, warn or error. file keeps the level each one's config file names.",
    ]);
  });

  it("says what a key is for, with more where there is more, and no default", async () => {
    mounted = await mount(
      withServices(
        <KeysView
          problems={new Map()}
          onSave={ignore}
          onRemove={ignore}
          secrets={[
            { name: "dnsProviderToken", description: "Short.", isSet: false, needsReentry: false },
          ]}
        />,
      ),
    );
    await click(buttonNamed(mounted.container, "What DNS provider token is for"));
    const said = [...mounted.container.querySelectorAll("dialog .setting-info p")].map(
      (paragraph) => paragraph.textContent,
    );
    expect(said[1]).toContain("To be reached from outside the home over HTTPS");
    expect(said.at(-1)).toContain("dnsProviderToken");
    expect(mounted.container.querySelector("dialog")?.textContent).not.toContain("The default is");
  });
});
