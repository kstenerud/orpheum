// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { sourcesHere } from "./test-sources.ts";

/**
 * What interface rules 1 and 14 (D538) keep out of the client's sources, each a pattern of the
 * source: a native drop-down, which a television's remote cannot work (DropDown, drop-down.tsx); a
 * number field, which steps on a wheel and a remote's Up and Down (Stepper, stepper.tsx); a
 * placeholder, gone when typing starts (Field's hint); an error paragraph by hand (Field's error, the
 * StatusNote); and a submit disabled by what its form holds, which says nothing of what is missing.
 */
const banned = {
  nativeSelect: /<select\b/gu,
  numberField: /type="number"/gu,
  placeholder: /\bplaceholder=/gu,
  alertParagraph: /<p\s+role="alert"/gu,
  disabledSubmit: /<button[^>]*type="submit"[^>]*disabled=\{/gu,
} as const;

type Banned = keyof typeof banned;

/**
 * The sites there were when the rule came (the owner, 2026-11-22), by file and how many: each is
 * moved onto its part by batch 2 of the revamp and taken off this list, which then shrinks. A new
 * site, or one more in a file listed, fails; a site gone and still listed fails too, so the list
 * says what is left. A search box's placeholder (listing-tools.tsx, server-settings.tsx) is the one the
 * rule keeps.
 */
const knownSites: Readonly<Record<Banned, Readonly<Record<string, number>>>> = {
  nativeSelect: {
    // A person's role: chosen in the pop-up chooser, its Saved note did not survive the page read
    // again after it (user.tsx's saved); left a select until that is understood.
    "user.tsx": 1,
  },
  numberField: {
    "candidate-chooser.tsx": 1,
    "create-library.tsx": 1,
    "library-manage.tsx": 1,
    "system-work.tsx": 1,
  },
  placeholder: {
    "candidate-chooser.tsx": 1,
    "device-activation.tsx": 2,
    "lab-field.tsx": 1,
    "lab-throughput.tsx": 1,
    "listing-tools.tsx": 1,
    "people-dialogs.tsx": 1,
    "server-settings.tsx": 1,
    "together-people.tsx": 1,
  },
  alertParagraph: {
    // The player's menus, which another part of the revamp is moving onto the parts.
    "audio-tracks.tsx": 1,
    // The player's notes under the video: play.tsx is at its 800-line cap and the player's other
    // agent is in it; the owner decides where its parts go.
    "play.tsx": 2,
    // The player's menus, as above.
    "sound.tsx": 1,
    "subtitles.tsx": 1,
  },
  disabledSubmit: {
    "create-library.tsx": 1,
    "device-activation.tsx": 1,
    "discovery-gate.tsx": 1,
    "problem-report.tsx": 1,
    "system-address-at-home.tsx": 1,
    "system-discovery-connect.tsx": 1,
    "system-discovery.tsx": 2,
    "system-network.tsx": 1,
  },
};

/** How many times pattern stands in each source that has it, by file. */
function sitesOf(
  sources: readonly (readonly [string, string])[],
  pattern: RegExp,
): Record<string, number> {
  const sites: Record<string, number> = {};
  for (const [name, text] of sources) {
    const count = [...text.matchAll(pattern)].length;
    if (count > 0) {
      sites[name] = count;
    }
  }
  return sites;
}

describe("a form is made of the parts, never of what they replace (D538, interface rules 1 and 14)", () => {
  const sources = sourcesHere(".tsx");

  it.each(Object.keys(banned) as Banned[])(
    "has %s only where it was when the rule came",
    (kind) => {
      expect(sources.map(([name]) => name)).toContain("stepper.tsx");
      expect(sitesOf(sources, banned[kind])).toEqual(knownSites[kind]);
    },
  );

  it("finds each, so the checks above can fail", () => {
    const made = [
      [
        "a.tsx",
        '<select value={a}><p role="alert">{b}</p><input type="number" placeholder="c" />' +
          '<button type="submit" disabled={name === ""}>Add</button></select>',
      ],
    ] as const;
    for (const kind of Object.keys(banned) as Banned[]) {
      expect(sitesOf(made, banned[kind]), kind).toEqual({ "a.tsx": 1 });
    }
  });
});
