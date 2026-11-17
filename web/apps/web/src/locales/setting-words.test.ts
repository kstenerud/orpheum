// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { enSettingLabels } from "./en-setting-labels.ts";
import { enSettingsCatalogue } from "./en-settings.ts";

/** The server's settings' keys, camelCase, as its registry's Go source declares them (`Key = "x"`). */
const serverSettingKeys = Object.entries(
  import.meta.glob("../../../../../internal/core/settings/*.go", {
    query: "?raw",
    import: "default",
    eager: true,
  }),
).flatMap(([path, source]) =>
  path.endsWith("_test.go")
    ? []
    : [...source.matchAll(/\bKey\s+=\s+"(\w+)"/gu)].flatMap((match) =>
        match[1] === undefined ? [] : [match[1]],
      ),
);

describe("the settings' words", () => {
  // The converse of the Go tests that hold a label and a description here for every setting the
  // server has (internal/core/settings/catalogue_test.go): a setting the server no longer has
  // keeps none, words no field shows.
  it("label and describe no setting the server does not have", () => {
    expect(serverSettingKeys).toContain("unavailableRootRescanSeconds");
    expect(serverSettingKeys.length).toBeGreaterThan(100);
    const stale = [
      ...Object.keys(enSettingLabels.labels).map((key) => `the label of ${key}`),
      ...Object.keys(enSettingsCatalogue.descriptions).map((key) => `the description of ${key}`),
    ].filter((named) => !serverSettingKeys.includes(named.split(" ").at(-1) ?? ""));
    expect(stale).toEqual([]);
  });

  // And as the Go tests hold it, from the web's side: every setting the server has is labelled and
  // described here, so its field never shows its bare key.
  it("label and describe every setting the server has", () => {
    const labels: Record<string, string> = enSettingLabels.labels;
    const descriptions: Record<string, string> = enSettingsCatalogue.descriptions;
    const unworded = serverSettingKeys.flatMap((key) => [
      ...(labels[key] === undefined ? [`the label of ${key}`] : []),
      ...(descriptions[key] === undefined ? [`the description of ${key}`] : []),
    ]);
    expect(unworded).toEqual([]);
  });

  // D509 §3: the alert's setting says, in its details, what is kept and what the banner says when.
  it("say in the alert's details what a failure of unknown cause keeps and when the banner shows", () => {
    const details: Record<string, string> = enSettingsCatalogue.details;
    expect(details["unknownFailuresAlertPerHour"]).toMatch(
      /kept with everything it said.*banner/su,
    );
  });
});
