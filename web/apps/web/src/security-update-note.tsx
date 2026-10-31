// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage } from "./browser-storage.ts";
import { Link } from "./link.tsx";
import { isSnoozed, snooze, useUpdate } from "./update.ts";

/**
 * The banner above every page while the server's version has a security fix pending, for its admins
 * alone (security-updates.md Part B): what the fix is, the command that installs it and the way to
 * the How to update page. It cannot be dismissed while the fix is pending; Snooze for a day hides it
 * in this browser for a day, kept in its storage, which a browser that refuses storage keeps for the
 * page's life alone. A newer release with no security fix is no banner: the settings page says it.
 * Everyone else sees nothing, and nothing is asked of the server for them.
 */
export function SecurityUpdateNote(props: { readonly isAdmin: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  const update = useUpdate(props.isAdmin);
  const [snoozedFix, setSnoozedFix] = useState<string | undefined>(undefined);
  // The page's time when drawn first: a snooze that ends while the page stays open shows again at
  // its next load.
  const [shownAtMs] = useState(() => Date.now());
  const fix = update.state === "loaded" ? update.value?.security : undefined;
  if (
    !props.isAdmin ||
    update.state !== "loaded" ||
    fix === undefined ||
    snoozedFix === fix.fixedIn ||
    isSnoozed(browserStorage(), fix.fixedIn, shownAtMs)
  ) {
    return null;
  }
  return (
    <output className="app-shell__note app-shell__note--security">
      {t("update.banner", { version: fix.fixedIn, summary: fix.summary.replace(/\.$/u, "") })}{" "}
      <Link to={{ screen: "update" }}>{t("update.updateNow")}</Link>
      {update.value?.command === undefined ? null : (
        <span className="app-shell__note-command">
          {t("update.runOnServer")} <code>{update.value.command}</code>
        </span>
      )}
      <button
        type="button"
        onClick={() => {
          snooze(browserStorage(), fix.fixedIn, Date.now());
          setSnoozedFix(fix.fixedIn);
        }}
      >
        {t("update.snooze")}
      </button>
    </output>
  );
}
