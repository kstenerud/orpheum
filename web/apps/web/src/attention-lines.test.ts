// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { attentionLine } from "./attention-lines.ts";
import type { AttentionItem } from "./attention-pages.ts";
import { i18n } from "./i18n.ts";

const gib = 2 ** 30;

/** A machine's disk too full to prepare videos, as its last status said it (D543). */
function diskFull(prepared?: NonNullable<AttentionItem["prepared"]>): AttentionItem {
  const item: AttentionItem = { kind: "diskFull", isProblem: true, since: 5000, nodeName: "attic" };
  return prepared === undefined ? item : { ...item, prepared };
}

describe("a machine's disk too full to prepare videos (D543)", () => {
  it("says the free space against the floor it keeps, or how full against its limit, on System", () => {
    const t = i18n.t.bind(i18n);
    const below = attentionLine(
      t,
      diskFull({
        usedBytes: gib,
        budgetBytes: 200 * gib,
        diskUsedPercent: 90,
        diskUsedLimitPercent: 95,
        diskAvailableBytes: 2 * gib,
        diskMinFreeBytes: 5 * gib,
      }),
      "en",
    );
    expect(below?.text).toBe(
      "attic's disk is too full to prepare videos: it has 2 GiB free and keeps at least 5 GiB free.",
    );
    expect(below?.to.screen).toBe("system");
    const past = attentionLine(
      t,
      diskFull({
        usedBytes: gib,
        budgetBytes: 200 * gib,
        diskUsedPercent: 97,
        diskUsedLimitPercent: 95,
        diskAvailableBytes: 50 * gib,
        diskMinFreeBytes: 5 * gib,
      }),
      "en",
    );
    expect(past?.text).toBe(
      "attic's disk is too full to prepare videos: it is 97% full, its limit 95%.",
    );
    expect(attentionLine(t, diskFull(), "en")?.text).toBe(
      "attic's disk is too full to prepare videos: it has not said how full it is.",
    );
  });
});
