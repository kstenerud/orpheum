// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Banners, type BannerNotice } from "./banner.tsx";
import { ConfirmButton, ConfirmDialog } from "./confirm-button.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { ListBox, ListOption, ListSubmenu } from "./list-box.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { PopoverButton } from "./popover.tsx";
import { StatusNote } from "./status-note.tsx";

/**
 * The gallery's dialogs, menus, banners and status notes (D538): a Dialog that opens another in its
 * place, a confirm, a menu whose picker screen takes its place, the banners and the status notes.
 */
export function OverlayParts(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-overlays">
      <h2 id="gallery-overlays">{t("gallery.overlays")}</h2>
      <GalleryDialog />
      <GalleryConfirm />
      <PopoverButton
        label={t("gallery.menu")}
        opener={t("gallery.menu")}
        openerClassName="ui-gallery__opener"
        openerNaming={{}}
        isFromItsTop
      >
        <ListBox isFromItsTop>
          <ListOption onChoose={() => undefined}>{t("gallery.choiceOne")}</ListOption>
          <ListSubmenu name={t("gallery.picker")} value={t("gallery.pickerValue")}>
            {["gallery.choiceOne", "gallery.choiceTwo", "gallery.choiceThree"].map((key, at) => (
              <ListOption key={key} isCurrent={at === 0} onChoose={() => undefined}>
                {t(key)}
              </ListOption>
            ))}
          </ListSubmenu>
        </ListBox>
      </PopoverButton>
      <GalleryBanners />
      <h3>{t("gallery.statusNotes")}</h3>
      <StatusNote
        told={{
          kind: "empty",
          text: t("gallery.statusEmpty"),
          detail: t("gallery.statusEmptyDetail"),
          next: { label: t("gallery.statusEmptyNext"), onPress: () => undefined },
        }}
      />
      <StatusNote
        told={{ kind: "problem", text: t("gallery.statusProblem"), onTryAgain: () => undefined }}
      />
      <StatusNote told={{ kind: "loading", text: t("gallery.statusLoading") }} />
      <StatusNote told={{ kind: "success", text: t("gallery.statusSuccess") }} />
    </section>
  );
}

/** The gallery dialog's foot: Cancel, then its main action, where it starts (D538 §4). */
function GalleryDialogFoot(props: { readonly onLeave: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <button type="button" onClick={props.onLeave}>
        {t("dialog.cancel")}
      </button>
      <button type="button" {...arrivalMark} onClick={props.onLeave}>
        {t("gallery.dialogSave")}
      </button>
    </>
  );
}

/** A dialog of parts that opens another in its place (D538 §5). */
function GalleryDialog(): JSX.Element {
  const { t } = useTranslation();
  const [depth, setDepth] = useState(0);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setDepth(1);
        }}
      >
        {t("gallery.dialog")}
      </button>
      {depth === 0 ? null : (
        <ModalDialog
          label={t("gallery.dialogHeading")}
          heading={t("gallery.dialogHeading")}
          onClose={() => {
            setDepth(0);
          }}
          footer={
            <GalleryDialogFoot
              onLeave={() => {
                setDepth(0);
              }}
            />
          }
        >
          <p>{t("gallery.dialogBody")}</p>
          <GalleryField />
          <p>
            <button
              type="button"
              onClick={() => {
                setDepth(2);
              }}
            >
              {t("gallery.dialogDeeper")}
            </button>
          </p>
          {depth === 2 ? (
            <ConfirmDialog
              question={t("gallery.removeHeading")}
              consequence={t("gallery.removeConsequence")}
              confirmLabel={t("gallery.removeYes")}
              onConfirm={() => undefined}
              onClose={() => {
                setDepth(1);
              }}
            />
          ) : null}
        </ModalDialog>
      )}
    </>
  );
}

/** A field in the gallery's dialog, where focus arrives (D538 §4). */
function GalleryField(): JSX.Element {
  const { t } = useTranslation();
  return (
    <label>
      {t("gallery.dialogField")} <input type="text" />
    </label>
  );
}

/**
 * The two confirms (ui.md rule 8): one before what cannot be undone, in the irreversible red, and one
 * before what is disruptive but harmless, plain; each starts on Cancel (D538 §4).
 */
function GalleryConfirm(): JSX.Element {
  const { t } = useTranslation();
  return (
    <div>
      <ConfirmButton
        label={t("gallery.remove")}
        question={t("gallery.removeHeading")}
        consequence={t("gallery.removeConsequence")}
        confirmLabel={t("gallery.removeYes")}
        onConfirm={() => undefined}
      />{" "}
      <ConfirmButton
        label={t("gallery.restart")}
        question={t("gallery.restartQuestion")}
        consequence={t("gallery.restartConsequence")}
        confirmLabel={t("gallery.restartYes")}
        isHarmless
        onConfirm={() => undefined}
      />
    </div>
  );
}

/** Three notices, two shown, the warning first; dismissing the information shows the third. */
function GalleryBanners(): JSX.Element {
  const { t } = useTranslation();
  const [isDismissed, setIsDismissed] = useState(false);
  const notices: BannerNotice[] = [
    {
      key: "information",
      tone: "information",
      text: t("gallery.bannerInformation"),
      actions: <a href="#gallery-overlays">{t("gallery.bannerLink")}</a>,
      putAway: {
        kind: "dismiss",
        onDismiss: () => {
          setIsDismissed(true);
        },
      },
    },
    {
      key: "warning",
      tone: "warning",
      text: t("gallery.bannerWarning"),
      putAway: { kind: "none" },
    },
    {
      key: "waiting",
      tone: "information",
      text: t("gallery.bannerWaiting"),
      putAway: { kind: "remind tomorrow", onRemind: () => undefined },
    },
  ];
  return (
    <>
      <h3>{t("gallery.banners")}</h3>
      <Banners notices={notices.filter((notice) => !isDismissed || notice.key !== "information")} />
    </>
  );
}
