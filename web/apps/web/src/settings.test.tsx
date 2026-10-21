// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TFunction } from "i18next";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import {
  AboutButton,
  isFound,
  parseSettingText,
  SettingsView,
  settingText,
  startText,
} from "./server-settings.tsx";
import { SecretsView } from "./settings.tsx";
import { click, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("settings", () => {
  it("shows whether a secret is set and its last characters, never its value, and when it must be entered again", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SecretsView
          search=""
          problems={new Map([["tmdb_api_key", "Refused."]])}
          onSave={ignore}
          onRemove={ignore}
          secrets={[
            {
              name: "tmdb_api_key",
              description: "TMDB's API key",
              isSet: true,
              lastChars: "a1b2",
              needsReentry: false,
            },
            {
              name: "opensubtitles_password",
              description: "OpenSubtitles",
              isSet: false,
              needsReentry: true,
            },
          ]}
        />,
      ),
    );
    expect(markup).toContain("Set, ending in a1b2");
    expect(markup).toContain("Not set");
    expect(markup).toContain("Must be entered again");
    expect(markup).toContain('type="password"');
    expect(markup).toContain('role="alert">Refused.</p>');
    // Only a secret that is set has a bin to press.
    expect(markup).toMatch(/<button[^>]*aria-label="Remove tmdb_api_key"/u);
    expect(markup).not.toMatch(/<button[^>]*disabled=""[^>]*aria-label="Remove tmdb_api_key"/u);
    expect(markup).toMatch(
      /<button[^>]*disabled=""[^>]*aria-label="Remove opensubtitles_password"/u,
    );
    // The tooltip says what the bin does, or why it cannot: the row names the secret.
    expect(markup).toContain('data-tip="Remove it"');
    expect(markup).toContain('data-tip="Not set, so nothing to remove"');
  });

  it("lists the important settings one to a row by section, each with what it is for and its starting value back", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SettingsView
          isAdvanced={false}
          search=""
          problems={new Map()}
          resetting={new Set()}
          onSave={ignore}
          onReset={ignore}
          settings={[
            {
              key: "tmdb_refresh_after_days",
              kind: "integer",
              description: "days",
              value: 90,
              startingValue: 180,
              isModified: true,
              isImportant: true,
              section: "metadata",
              minimum: 0,
              maximum: 36500,
              setAt: 1,
            },
            {
              key: "should_check_for_updates",
              kind: "bool",
              description: "updates",
              value: true,
              startingValue: null,
              isModified: true,
              isImportant: true,
              section: "general",
              setAt: 1,
            },
            {
              key: "should_scan_on_node_hello",
              kind: "bool",
              description: "scan on hello",
              value: true,
              startingValue: true,
              isModified: false,
              isImportant: false,
              section: "libraries",
              setAt: 1,
            },
          ]}
        />,
      ),
    );
    expect(markup).toContain('type="number"');
    expect(markup).toContain('value="90"');
    expect(markup).toContain('aria-labelledby="setting-should_check_for_updates" checked=""');
    expect(markup).toContain("Return should_check_for_updates to its starting value, unanswered");
    expect(markup).toContain(
      'aria-label="Return tmdb_refresh_after_days to its starting value, 180"',
    );
    // General before Metadata, the server's order; the setting that is not important waits for Advanced.
    expect(markup.indexOf(">General<")).toBeLessThan(markup.indexOf(">Metadata and artwork<"));
    expect(markup).not.toContain("should_scan_on_node_hello");
    // The bin's tooltip names the starting value in words, or says the setting is already there.
    expect(markup).toContain('data-tip="Return to default value (180)"');
    expect(markup).toContain('data-tip="Return to default value (unanswered)"');
    expect(markup).toContain('data-tip="What it is for"');
    expect(parseSettingText("integer", "30")).toBe(30);
    expect(parseSettingText("bool", "false")).toBe(false);
    expect(settingText(null)).toBe("");
  });

  it("opens what a setting is for as a dialog: the short words, then more only where there is more", async () => {
    mounted = await mount(
      withServices(<AboutButton name="dns_provider_token" description="Short." />),
    );
    await click(
      mounted.container.querySelector('button[aria-label="What dns_provider_token is for"]'),
    );
    const said = [...mounted.container.querySelectorAll("dialog p")].map(
      (paragraph) => paragraph.textContent,
    );
    expect(said[0]).toBe("Short.");
    expect(said[1]).toContain("To be reached from outside the home over HTTPS");
    expect(said).toHaveLength(2);
    mounted.unmount();
    mounted = await mount(
      withServices(<AboutButton name="opensubtitles_username" description="Short." />),
    );
    await click(mounted.container.querySelector("button"));
    expect(mounted.container.querySelectorAll("dialog p")).toHaveLength(1);
  });

  it("says what a setting is for in the catalogue's words, the server's where it has none", async () => {
    const setting = {
      key: "tmdb_refresh_after_days",
      kind: "integer" as const,
      description: "days, for developers",
      value: 90,
      startingValue: 180,
      isModified: true,
      isImportant: true,
      section: "metadata" as const,
      setAt: 1,
    };
    const shown = async (key: string): Promise<string | null | undefined> => {
      mounted?.unmount();
      mounted = await mount(
        withServices(
          <SettingsView
            isAdvanced={false}
            search=""
            problems={new Map()}
            resetting={new Set()}
            onSave={ignore}
            onReset={ignore}
            settings={[{ ...setting, key }]}
          />,
        ),
      );
      await click(mounted.container.querySelector(`button[aria-label="What ${key} is for"]`));
      return mounted.container.querySelector("dialog p")?.textContent;
    };
    expect(await shown("tmdb_refresh_after_days")).toBe(
      "How long a title's TMDB data is kept before it is fetched again. 0 never fetches again.",
    );
    expect(await shown("a_setting_the_catalogue_lacks")).toBe("days, for developers");
  });

  it("names a starting value in words: on or off, nothing, or unanswered", () => {
    const t = ((key: string): string => key.replace("settings.", "")) as unknown as TFunction;
    expect(startText(t, true)).toBe("on");
    expect(startText(t, false)).toBe("off");
    expect(startText(t, "")).toBe("nothing");
    expect(startText(t, null)).toBe("unanswered");
    expect(startText(t, 90)).toBe("90");
  });

  it("finds a setting by every word searched for, in its key or its description", () => {
    expect(isFound("drift together", "together_drift_left_alone_ms", "How far it may drift")).toBe(
      true,
    );
    expect(isFound("SEEK", "together_drift_left_alone_ms", "How far it may drift")).toBe(false);
    expect(isFound("  ", "any_key", "")).toBe(true);
  });
});
