// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { IconButton } from "./icon-button.tsx";
import { dismissNotice, useNotice, type Notice } from "./notice.ts";
import { CloseIcon } from "./player-icons.tsx";
import { useRoute } from "./services.tsx";

/**
 * The tab's notice (notice.ts) above the screen, with a × that closes it: a button a remote's
 * arrows reach like any other, no held key needed. It goes too once the screen it was first shown
 * on changes.
 */
export function NoticeNote(): JSX.Element | null {
  const { t } = useTranslation();
  const notice = useNotice();
  const screen = JSON.stringify(useRoute());
  const [seen, setSeen] = useState<{ notice: Notice; screen: string } | undefined>(undefined);
  if (notice !== undefined && seen?.notice !== notice) {
    setSeen({ notice, screen });
  }
  const isLeftBehind = notice !== undefined && seen?.notice === notice && seen.screen !== screen;
  useEffect(() => {
    if (isLeftBehind) {
      dismissNotice(notice);
    }
  }, [isLeftBehind, notice]);
  if (notice === undefined || isLeftBehind) {
    return null;
  }
  return (
    <output className="app-shell__note notice-note">
      <span className="notice-note__text">{notice.text}</span>
      <IconButton
        className="notice-note__close"
        label={t("notice.close")}
        icon={<CloseIcon />}
        onClick={() => {
          dismissNotice(notice);
        }}
      />
    </output>
  );
}
