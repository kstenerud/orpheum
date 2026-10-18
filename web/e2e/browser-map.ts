// SPDX-License-Identifier: MPL-2.0

/**
 * The browser map's recording (scripts/browser_map.py): every browser test imports `test` from
 * here. When ORPHEUM_BROWSER_MAP_DIR is set, as check-pr's full browser run sets it, every page of
 * every context a test opens reports each tag, class, id and attribute name it ever holds, and a
 * test that passed writes them to the directory. Revert-red then runs a stylesheet's revert only in
 * the tests whose pages held what its changed rules select. Each test also writes every built script
 * its pages, their workers and the service worker loaded, which with what each chunk holds (the
 * build's record, apps/web/src/chunk-modules.ts) says which tests a source file's revert can reach.
 * Unset, nothing is added to any page.
 */

import { createHash } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

import {
  test as base,
  type Browser,
  type BrowserContext,
  type Page,
  type TestInfo,
} from "@playwright/test";

import { startServer } from "./server.ts";

const mapDirectory = process.env["ORPHEUM_BROWSER_MAP_DIR"];

declare global {
  interface Window {
    /** What a page calls with the tokens it has newly seen: the binding each watched context exposes. */
    orpheumBrowserMapSeen?: (tokens: string[]) => Promise<void>;
  }
}

/**
 * Runs in every page before its own scripts: reports each tag, class, id and attribute name any
 * element ever holds, as it appears or changes. A page that never held a rule's tokens was never
 * styled by it. Attribute names and tags are lowercased, as a stylesheet in an HTML page compares
 * them; classes and ids keep their case.
 */
function recordWhatThePageHolds(): void {
  const seen = new Set<string>();
  let fresh: string[] = [];
  const note = (token: string): void => {
    if (!seen.has(token)) {
      seen.add(token);
      fresh.push(token);
    }
  };
  const noteElement = (element: Element): void => {
    note(`tag:${element.localName.toLowerCase()}`);
    for (const name of element.classList) {
      note(`class:${name}`);
    }
    if (element.id !== "") {
      note(`id:${element.id}`);
    }
    for (const attribute of element.attributes) {
      note(`attr:${attribute.name.toLowerCase()}`);
    }
  };
  const noteTree = (node: Node): void => {
    if (node instanceof Element) {
      noteElement(node);
      for (const element of node.querySelectorAll("*")) {
        noteElement(element);
      }
    }
  };
  const report = (): void => {
    if (fresh.length > 0) {
      const batch = fresh;
      fresh = [];
      void window.orpheumBrowserMapSeen?.(batch);
    }
  };
  new MutationObserver((records) => {
    for (const record of records) {
      if (record.type === "attributes") {
        noteTree(record.target);
      }
      for (const added of record.addedNodes) {
        noteTree(added);
      }
    }
    report();
  }).observe(document, { subtree: true, childList: true, attributes: true });
}

/** The string Playwright's grep matches a test by: its project, file, titles and tags (D190). */
function grepTitle(testInfo: TestInfo): string {
  return [testInfo.project.name, ...testInfo.titlePath, ...testInfo.tags].join(" ");
}

/**
 * The tag of a test that reads the stylesheets themselves, not what a page drew: it can notice a
 * change to any rule, whatever its pages held (the dead-rule check, shell.spec.ts).
 */
export const readsStylesheets = "@stylesheets";

/**
 * What a test's pages held, or null when the record cannot be vouched for: a context the test made
 * other than by browser.newContext, which nothing watched, a test that did not run to its end, or
 * one that reads the stylesheets themselves (readsStylesheets). A null record reaches every change
 * (scripts/browser_map.py).
 */
export function record(
  tokens: Set<string>,
  wasEveryContextWatched: boolean,
  testInfo: TestInfo,
): string[] | null {
  return wasEveryContextWatched &&
    testInfo.status === "passed" &&
    !testInfo.tags.includes(readsStylesheets)
    ? [...tokens].toSorted()
    : null;
}

/** A script's file in the build, as the build's record names it ("assets/client-main-….js"). */
export function builtScriptOf(url: string): string | undefined {
  const { pathname } = new URL(url);
  return pathname.endsWith(".js") ? pathname.slice(1) : undefined;
}

/** Notes every built script a context's pages, their workers and its service workers load. */
function noteScripts(context: BrowserContext, scripts: Set<string>): void {
  const note = (url: string): void => {
    const script = builtScriptOf(url);
    if (script !== undefined) {
      scripts.add(script);
    }
  };
  const watchPage = (page: Page): void => {
    page.on("worker", (worker) => {
      note(worker.url());
    });
  };
  context.on("request", (request) => {
    note(request.url());
  });
  context.on("page", watchPage);
  for (const page of context.pages()) {
    watchPage(page);
  }
  // Chromium's alone; Firefox's is seen by its request (revert-red re-runs every test before it
  // calls a revert green, so a script missed here costs time, never a verdict).
  context.on("serviceworker", (worker) => {
    note(worker.url());
  });
}

/** Writes a test's record into directory: what its pages held, and the scripts they loaded. */
function writeRecord(
  directory: string,
  testInfo: TestInfo,
  seen: { readonly held: string[] | null; readonly scripts: ReadonlySet<string> },
): void {
  const { held, scripts } = seen;
  const title = grepTitle(testInfo);
  mkdirSync(directory, { recursive: true });
  const name = createHash("sha256").update(title).digest("hex").slice(0, 16);
  writeFileSync(
    path.join(directory, `${name}.json`),
    JSON.stringify({
      test: title,
      tokens: held,
      // A record that cannot vouch for what its pages held cannot for what they loaded either.
      scripts: held === null ? null : [...scripts].toSorted(),
    }),
  );
}

export const test = base.extend<{ browserMap: undefined }, { server: undefined }>({
  // A server of each worker's own, started before its first test and stopped after its last, so
  // workers run side by side and no test meets another's state (server.ts). The address is the
  // worker's environment's, which serverUrl reads.
  server: [
    // eslint-disable-next-line no-empty-pattern -- Playwright reads a fixture's first parameter for what it needs, which is nothing
    async ({}, use, workerInfo) => {
      const stop = await startServer(
        [],
        true,
        `${process.env["ORPHEUM_E2E_TIMING"] === "1" ? "timed-" : ""}${workerInfo.project.name}-${String(workerInfo.workerIndex)}`,
      );
      try {
        await use(undefined);
      } finally {
        await stop();
      }
    },
    { scope: "worker", auto: true, timeout: 180_000 },
  ],
  browserMap: [
    async ({ browser, context }, use, testInfo) => {
      if (mapDirectory === undefined || mapDirectory === "") {
        await use(undefined);
        return;
      }
      const tokens = new Set<string>();
      const scripts = new Set<string>();
      const watched = new Set<BrowserContext>([context]);
      const watch = async (one: BrowserContext): Promise<void> => {
        noteScripts(one, scripts);
        await one.exposeBinding("orpheumBrowserMapSeen", (_source, reported: string[]) => {
          for (const token of reported) {
            tokens.add(token);
          }
        });
        await one.addInitScript(recordWhatThePageHolds);
      };
      await watch(context);
      const opened: BrowserContext[] = [];
      const noteOpened = (one: BrowserContext): void => {
        opened.push(one);
      };
      browser.on("context", noteOpened);
      // Tests open contexts of their own (a host and a guest): each is watched before the test has
      // it, so its first page is too. Playwright's "context" event cannot hold the test back that way.
      const openContext: Browser["newContext"] = browser.newContext.bind(browser);
      // eslint-disable-next-line no-param-reassign -- wrapping the browser's own method is the point: every context a test opens is watched
      browser.newContext = async (options) => {
        const one = await openContext(options);
        watched.add(one);
        await watch(one);
        return one;
      };
      try {
        await use(undefined);
      } finally {
        // eslint-disable-next-line no-param-reassign -- the wrapped method put back
        browser.newContext = openContext;
        browser.off("context", noteOpened);
      }
      // A failed test stops the run, which then stamps no map; its record is never read.
      if (
        testInfo.status === "failed" ||
        testInfo.status === "timedOut" ||
        testInfo.status === "interrupted"
      ) {
        return;
      }
      const held = record(
        tokens,
        opened.every((one) => watched.has(one)),
        testInfo,
      );
      writeRecord(path.join(mapDirectory, testInfo.project.name), testInfo, { held, scripts });
    },
    { auto: true },
  ],
});
