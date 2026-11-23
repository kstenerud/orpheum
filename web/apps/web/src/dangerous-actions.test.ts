// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

/** Every source file of the app, as text, tests left out, by its path. */
// Untyped at the call: vitest keeps no cached transform of a file holding "import.meta.glob(", whose
// matches change as files come and go (2026-10-26; glob-cache.test.ts).
const sources: Record<string, string> = import.meta.glob(
  ["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}"],
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
);

/**
 * Every dangerous action of the web client (ui.md rule 8, the owner, 2026-11-22: "All dangerous
 * actions bring up a confirmation dialog, and the confirmation defaults to not doing it"): what
 * deletes, removes, revokes, ends, discards, resets, signs out, stops a running job, or otherwise
 * cannot be undone or loses someone's work, by the file that draws it and its confirm's action. Each
 * acts only from the confirm (confirm-button.tsx), whose focus starts on Cancel; confirmed-actions
 * .test.tsx and the parts' own tests press them. A new confirm joins this list, so the audit stays
 * whole.
 */
const dangerousActions: readonly (readonly [file: string, confirmKey: string])[] = [
  ["./app-shell.tsx", "nav.signOut"],
  ["./invitations-section.tsx", "people.withdrawConfirm"],
  ["./jobs.tsx", "jobs.cancelConfirm"],
  ["./library-manage.tsx", "libraries.approveRemovalsConfirm"],
  ["./library-manage.tsx", "libraries.deleteConfirm"],
  ["./library-manage.tsx", "libraries.diskGoneConfirm"],
  ["./library-manage.tsx", "libraries.retireRootConfirm"],
  ["./loader-at-loader-origin.tsx", "loader.households.forgetConfirm"],
  ["./node-unconfined.tsx", "system.unconfined.allowConfirm"],
  ["./rescan-action.tsx", "libraries.rescanConfirm"],
  ["./roles-section.tsx", "roles.deleteConfirm"],
  ["./settings-keys.tsx", "settings.removeConfirm"],
  ["./settings-unread.tsx", "settings.unread.confirm"],
  ["./system-discovery.tsx", "system.discovery.leaveConfirm"],
  ["./system-network.tsx", "system.network.restartConfirm"],
  ["./system-problem-reports.tsx", "system.problemReports.deleteConfirm"],
  ["./system-problem-reports.tsx", "system.problemReports.soundOffset.applyConfirm"],
  ["./system-waiting.tsx", "system.waitingRemoveConfirm"],
  ["./system.tsx", "system.retireNodeConfirm"],
  ["./title.tsx", "title.retireFileConfirm"],
  ["./together-people.tsx", "watchTogether.ejectConfirm"],
  ["./together-people.tsx", "watchTogether.withdrawConfirm"],
  ["./user.tsx", "people.newLinkConfirm"],
  ["./user.tsx", "person.cancelResetLinkConfirm"],
  ["./user.tsx", "person.newResetLinkConfirm"],
  ["./user.tsx", "person.removeConfirm"],
  ["./user.tsx", "person.renewResetLinkConfirm"],
  ["./user.tsx", "person.revokeOwnLinkConfirm"],
  ["./watch-together.tsx", "watchTogether.endConfirm"],
  ["./your-devices.tsx", "devices.removeConfirm"],
  ["./your-devices.tsx", "devices.signOutElsewhereConfirm"],
];

/** The gallery's confirms, which only show the part. */
const galleryFiles = new Set(["./ui-gallery.tsx", "./ui-gallery-overlays.tsx"]);

/** Two confirms in order: by file, then by their action's words. */
function byFileThenLabel(
  [file, label]: readonly [string, string],
  [otherFile, otherLabel]: readonly [string, string],
): number {
  const byFile = file.localeCompare(otherFile);
  return byFile === 0 ? label.localeCompare(otherLabel) : byFile;
}

/** Each confirm the sources hold, as [file, its action's words' key]. */
function confirmsInSources(): (readonly [string, string])[] {
  const found: (readonly [string, string])[] = [];
  for (const [file, text] of Object.entries(sources)) {
    if (galleryFiles.has(file)) {
      continue;
    }
    for (const match of text.matchAll(/confirmLabel=\{t\("([^"]+)"/g)) {
      found.push([file, match[1] ?? ""]);
    }
  }
  return found.sort(byFileThenLabel);
}

describe("every dangerous action asks first, in the one confirm (ui.md rule 8, D538 §4)", () => {
  it("draws each dangerous action listed, and no other, through the confirm", () => {
    expect(confirmsInSources()).toEqual([...dangerousActions].sort(byFileThenLabel));
  });

  it("draws the irreversible red only in the confirm, so a red action is always one that asks", () => {
    const red = Object.entries(sources)
      .filter(([, text]) => /"irreversible(-outline|-words)?"/.test(text))
      .map(([file]) => file);
    expect(red).toEqual(["./confirm-button.tsx"]);
  });

  it("names Cancel as a confirm's safe choice, no Keep it left", () => {
    const confirm = sources["./confirm-button.tsx"] ?? "";
    expect(confirm).toContain('t("dialog.cancel")');
    expect(confirm).toContain("isConfirm");
    expect(Object.values(sources).some((text) => text.includes("common.keepIt"))).toBe(false);
  });
});
