// SPDX-License-Identifier: MPL-2.0

import { useState } from "react";
import { useTranslation } from "react-i18next";

import type { BannerNotice } from "./banner.tsx";
import { browserStorage } from "./browser-storage.ts";
import { Link } from "./link.tsx";
import { isSnoozed, snooze, useUpdate } from "./update.tsx";

/**
 * The banner above every page while the server's version has a security fix pending, for its admins
 * alone (security-updates.md Part B), a warning Banner (D538 §1): what the fix is and Update now, the
 * way to the How to update page, whose steps give the command that installs it. It cannot be
 * dismissed while the fix is pending; Remind me tomorrow hides it, that version's, in this browser
 * for a day, kept in its storage, which a browser that refuses storage keeps for the page's life
 * alone. A newer release with no security fix is no banner: the settings page says it. Everyone else
 * sees nothing, and nothing is asked of the server for them.
 */
export function useSecurityUpdateNotice(isAdmin: boolean): BannerNotice | undefined {
  const { t } = useTranslation();
  const update = useUpdate(isAdmin);
  const [snoozedFix, setSnoozedFix] = useState<string | undefined>(undefined);
  // The page's time when drawn first: a snooze that ends while the page stays open shows again at
  // its next load.
  const [shownAtMs] = useState(() => Date.now());
  const fix = update.state === "loaded" ? update.value?.security : undefined;
  if (
    !isAdmin ||
    update.state !== "loaded" ||
    fix === undefined ||
    snoozedFix === fix.fixedIn ||
    isSnoozed(browserStorage(), fix.fixedIn, shownAtMs)
  ) {
    return undefined;
  }
  return {
    key: "security update",
    tone: "warning",
    text: t("update.banner", { version: fix.fixedIn, summary: fix.summary.replace(/\.$/u, "") }),
    actions: <Link to={{ screen: "update" }}>{t("update.updateNow")}</Link>,
    putAway: {
      kind: "remind tomorrow",
      onRemind: () => {
        snooze(browserStorage(), fix.fixedIn, Date.now());
        setSnoozedFix(fix.fixedIn);
      },
    },
  };
}
