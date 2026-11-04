// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog } from "./modal-dialog.tsx";
import { dismissNotice, useNotice, type Notice } from "./notice.ts";
import { useRoute } from "./services.tsx";

/**
 * The tab's notice (notice.ts), a pop-up over the screen (the owner, 2026-10-29: the notice that a
 * Watch Together session ended "is a modal popup", not the note at the top of the page): the shared
 * modal dialog, its words and an OK a remote's arrows reach, closed too by a press outside it, Escape
 * and Back. It goes too once the screen it was first shown on changes.
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
  const close = (): void => {
    dismissNotice(notice);
  };
  return (
    <ModalDialog role="alertdialog" label={notice.text} onClose={close}>
      <p>{notice.text}</p>
      <button type="button" onClick={close}>
        {t("notice.ok")}
      </button>
    </ModalDialog>
  );
}
