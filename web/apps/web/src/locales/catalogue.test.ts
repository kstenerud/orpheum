// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { camelKey } from "../catalogue-key.ts";
import { sectionOrder } from "../server-settings.tsx";
import { settingsWords } from "../settings-words.ts";
import { en } from "./en.ts";
import { enGallery } from "./en-gallery.ts";
import { enLab } from "./en-lab.ts";
import { enLibraries } from "./en-libraries.ts";
import { enPeople } from "./en-people.ts";
import { enSystem } from "./en-system.ts";

/** Every source file of the app, as text, tests left out. */
// Untyped at the call: vitest keeps no cached transform of a file holding "import.meta.glob(", whose
// matches change as files come and go; one given its type at the call escaped that, and a test read
// a stale list of sources (2026-10-26; glob-cache.test.ts).
const sources: Record<string, string> = import.meta.glob(
  ["../**/*.{ts,tsx}", "!../**/*.test.{ts,tsx}"],
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
);

/** The keys a source file asks the translation layer for by name, as t("a.b"). */
function keysAskedFor(source: string): string[] {
  return [...source.matchAll(/\bt\(\s*"([^"]+)"/gu)].flatMap((match) =>
    match[1] === undefined ? [] : [match[1]],
  );
}

/** The words a module adds as it loads, never with the first page, by the import that adds them. */
const laterWords: Record<string, object> = {
  "./lab-words.ts": enLab,
  "./gallery-words.ts": enGallery,
  "./system-words.ts": enSystem,
  "./libraries-words.ts": enLibraries,
  "./people-words.ts": enPeople,
  "./settings-words.ts": settingsWords,
};

/** The catalogue a source file's keys are read from: the first page's, and the words it brings. */
function catalogueOf(source: string): object {
  const brought = Object.entries(laterWords).filter(([path]) =>
    source.includes(`import "${path}";`),
  );
  return brought.reduce<object>((catalogue, [, words]) => withWords(catalogue, words), en);
}

/** A catalogue with words added, merged at every depth as i18n.ts's addWords merges them: the
 * settings pages' words join `settings.`, which the first page's catalogue has too. */
function withWords(catalogue: object, words: object): object {
  const merged = new Map<string, unknown>(Object.entries(catalogue));
  for (const [key, value] of Object.entries(words)) {
    const held = merged.get(key);
    merged.set(
      key,
      typeof held === "object" && held !== null && typeof value === "object" && value !== null
        ? withWords(held, value)
        : value,
    );
  }
  return Object.fromEntries(merged);
}

/** Whether the catalogue has text at a dotted key. */
function hasText(catalogue: unknown, key: string): boolean {
  let node = catalogue;
  for (const part of key.split(".")) {
    if (typeof node !== "object" || node === null) {
      return false;
    }
    node = new Map<string, unknown>(Object.entries(node)).get(part);
  }
  return typeof node === "string";
}

/** The API document, as text: the one list of the codes the server refuses with (cut-a §5.2). */
const apiDocuments = Object.values(
  import.meta.glob("../../../../../api/openapi.json", {
    query: "?raw",
    import: "default",
    eager: true,
  }),
);

/** A field of a parsed JSON value, or undefined where there is none. */
function field(value: unknown, name: string): unknown {
  return typeof value === "object" && value !== null
    ? new Map<string, unknown>(Object.entries(value)).get(name)
    : undefined;
}

/** A schema's enumerated string values at a property. */
function enumAt(schema: unknown, property: string): string[] {
  const values = field(field(field(schema, "properties"), property), "enum");
  return Array.isArray(values)
    ? values.filter((value): value is string => typeof value === "string")
    : [];
}

/**
 * Every error code the API document declares, and for each code whose detail enumerates a
 * reason, those reasons, read from ProblemBody's forms.
 */
function problemCodesAndReasons(document: string): { code: string; reasons: string[] }[] {
  const schemas = field(field(JSON.parse(document), "components"), "schemas");
  const forms = field(field(schemas, "ProblemBody"), "oneOf");
  if (!Array.isArray(forms)) {
    return [];
  }
  return forms.flatMap((form: unknown) => {
    const reference = field(field(field(form, "properties"), "detail"), "$ref");
    const detail =
      typeof reference === "string" ? field(schemas, reference.split("/").at(-1) ?? "") : undefined;
    const reasons = enumAt(detail, "reason");
    return enumAt(form, "code").map((code) => ({ code, reasons }));
  });
}

/** Where a code's reasons have their words, by the code in camelCase. */
const reasonKeys: Readonly<Partial<Record<string, string>>> = {
  authRequired: "problem.signedOut",
  providerError: "problem.providerReason",
  mediaUnreachable: "problem.mediaReason",
};

/**
 * A code's own words, problem.<code> in camelCase, except where problemText words it by its
 * reason: a session's end with no reason it knows says there is no session.
 */
const codeKeys: Readonly<Partial<Record<string, string>>> = {
  authRequired: "problem.signedOut.noSession",
};

describe("the English catalogue", () => {
  it("has words for every error code and every reason the API document declares", () => {
    expect(apiDocuments).toHaveLength(1);
    const declared = problemCodesAndReasons(apiDocuments[0] ?? "");
    expect(declared.map(({ code }) => code)).toContain("notFound");
    expect(declared.find(({ code }) => code === "providerError")?.reasons).toContain("noKey");
    const missing = declared.flatMap(({ code, reasons }) => [
      ...(hasText(en, codeKeys[camelKey(code)] ?? `problem.${camelKey(code)}`) ? [] : [code]),
      ...reasons
        .map(
          (reason) =>
            `${reasonKeys[camelKey(code)] ?? `problem.${camelKey(code)}`}.${camelKey(reason)}`,
        )
        .filter((key) => !hasText(en, key)),
    ]);
    expect(missing).toEqual([]);
  });

  it("finds the keys a file asks for", () => {
    expect(keysAskedFor('t("a.b"); x.t("c"); at("d"); t(\n  "e.f", {})')).toEqual([
      "a.b",
      "c",
      "e.f",
    ]);
  });

  it("has text for every key the app asks for by name, so none shows as its key (D51)", () => {
    const missing = Object.entries(sources).flatMap(([file, source]) =>
      keysAskedFor(source)
        .filter((key) => !hasText(catalogueOf(source), key))
        .map((key) => `${file}: ${key}`),
    );
    expect(Object.keys(sources).length).toBeGreaterThan(20);
    expect(missing).toEqual([]);
  });

  // A key need not be asked for as t("a.b"): a list of labels, a Trans's i18nKey or a key built at
  // the call names it as a quoted "a.b" too, and shows as itself where its words never arrived.
  it("finds every file naming a key of words that load later importing what adds them", () => {
    const missing = Object.entries(sources).flatMap(([path, source]) =>
      Object.entries(laterWords).flatMap(([words, catalogue]) => {
        const names = Object.keys(catalogue).join("|");
        const naming = new RegExp(`["'\`](?:${names})\\.[A-Za-z]`, "u");
        return path.endsWith(words.slice(1)) ||
          !naming.test(source) ||
          source.includes(`import "${words}";`)
          ? []
          : [`${path} names ${naming.exec(source)?.[0] ?? ""}… without importing ${words}`];
      }),
    );
    expect(missing).toEqual([]);
  });

  it("names every section of the settings page, listed in the order the API document declares them", () => {
    const schemas = field(field(JSON.parse(apiDocuments[0] ?? ""), "components"), "schemas");
    const section = field(field(field(schemas, "SettingBody"), "properties"), "section");
    expect(field(section, "enum")).toEqual(sectionOrder);
    expect(
      sectionOrder.filter(
        (each) => !hasText(withWords(en, settingsWords), `settings.sections.${camelKey(each)}`),
      ),
    ).toEqual([]);
  });
});
