// SPDX-License-Identifier: MPL-2.0

/**
 * ORPHEUM_E2E_ONLY, a JSON list of tests as Playwright's grep sees them ("chromium shell.spec.ts the
 * logo is…", browser-map.ts's record of each), or of whole specs by file ("shell.spec.ts", every test
 * in it under every browser): only those run. Revert-red sets it to the tests a revert can reach, and
 * check-pr to those a change can (scripts/browser_map.py, scripts/impact.py); unset, every test runs.
 */
export function onlyTheseTests(listed: string | undefined): { grep?: RegExp[] } {
  if (listed === undefined || listed === "") {
    return {};
  }
  const tests: unknown = JSON.parse(listed);
  if (!Array.isArray(tests) || !tests.every((one): one is string => typeof one === "string")) {
    throw new Error(`ORPHEUM_E2E_ONLY is ${listed}; want a JSON list of tests`);
  }
  const escape = (text: string): string => text.replaceAll(/[$()*+.?[\\\]^{|}]/gu, String.raw`\$&`);
  return {
    grep: tests.map((one) =>
      isSpecFile(one)
        ? new RegExp(`^\\S+ ${escape(one)} `, "u")
        : new RegExp(`^${escape(one)}$`, "u"),
    ),
  };
}

/** Whether an entry names a whole spec, a file of tests, rather than one test. */
function isSpecFile(entry: string): boolean {
  return /^[\w-]+\.spec\.ts$/u.test(entry);
}

/**
 * The tag of a test whose claim is a time (press to play within 8 s, voice under 300 ms): it runs in
 * the browser tier's second pass, alone on one worker and the machine, after the rest ran side by
 * side (the owner, 2026-10-18). A test slow only by how it is written is fixed, not tagged.
 */
export const timingTag = "@timing";

/** A tag as Playwright's grep sees it, among a test's others at its title's end. */
const timingTagged = /(?:^|\s)@timing(?:\s|$)/u;

/**
 * The tests of one pass of the browser tier, ORPHEUM_E2E_ONLY's narrowing kept: the first, every
 * test but those tagged timingTag; the second, only those.
 */
export function testsOfThisPass(
  listed: string | undefined,
  isTimingPass: boolean,
): { grep?: RegExp[]; grepInvert?: RegExp[] } {
  const only = onlyTheseTests(listed).grep;
  if (!isTimingPass) {
    return { ...(only === undefined ? {} : { grep: only }), grepInvert: [timingTagged] };
  }
  // Each listed pattern, anchored at its start as onlyTheseTests makes it, and the tag besides.
  return {
    grep: (only ?? [/^/u]).map(
      (pattern) =>
        new RegExp(`^(?=.*${timingTagged.source})${pattern.source.slice(1)}`, pattern.flags),
    ),
  };
}
