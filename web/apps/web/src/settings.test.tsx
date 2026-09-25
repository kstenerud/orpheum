// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { parsed, SecretsView, SettingsView, shown } from "./settings.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("settings", () => {
  it("shows whether a secret is set and its hint, never its value, and when it must be entered again", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SecretsView
          notes={new Map([["tmdb_api_key", "Saved."]])}
          onSave={ignore}
          onRemove={ignore}
          secrets={[
            {
              name: "tmdb_api_key",
              description: "TMDB's API key",
              isSet: true,
              hint: "a1b2",
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
    expect(markup).toContain("<output>Saved.</output>");
    expect(markup.match(/>Remove</gu)).toHaveLength(1);
  });

  it("edits each setting by its kind, and offers the starting value back when it differs", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SettingsView
          onSave={ignore}
          onReset={ignore}
          settings={[
            {
              key: "tmdb_refresh_after_d",
              kind: "integer",
              describe: "days",
              value: 90,
              start: 180,
              isModified: true,
              isExposed: false,
              minimum: 0,
              maximum: 36500,
              setAt: 1,
            },
            {
              key: "scan_on_node_hello",
              kind: "bool",
              describe: "scan on hello",
              value: true,
              start: true,
              isModified: false,
              isExposed: false,
              setAt: 1,
            },
          ]}
        />,
      ),
    );
    expect(markup).toContain('type="number"');
    expect(markup).toContain('value="90"');
    expect(markup).toContain("Changed; the starting value is 180");
    expect(markup).toContain("Reset to the starting value");
    expect(markup).toContain('type="checkbox" checked=""');
    expect(parsed("integer", "30")).toBe(30);
    expect(parsed("bool", "false")).toBe(false);
    expect(shown(null)).toBe("");
  });
});
