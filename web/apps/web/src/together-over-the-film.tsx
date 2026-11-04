// SPDX-License-Identifier: MPL-2.0

import "./together-over-the-film.css";

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, useRef, useState, useSyncExternalStore, type JSX, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { IconButton } from "./icon-button.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { signMark, TogetherIcon } from "./sign-mark.tsx";

/**
 * Watch Together over the playing film (D423): the film fills the window over the page (D418 §1), so
 * the session's panel, drawn on the page under it, would be out of reach while it plays. A panel that
 * says so (TogetherPanel) is moved, never drawn again, into the dialog the sign over the film opens
 * (TogetherSignOverTheFilm), so its voice and its state go on as they were; closed, it goes back to
 * its page. One panel at a time, in this tab: the host's or the guest's.
 */
let panels = 0;
let dialogBody: HTMLElement | null = null;
const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Whether a session's panel is drawn now, which the sign over the film shows for. */
function hasPanel(): boolean {
  return panels > 0;
}

function currentDialogBody(): HTMLElement | null {
  return dialogBody;
}

/**
 * A session's panel: drawn where it stands, and moved into the dialog the sign over the film opens
 * while that is open. Its children are drawn once, into a box of their own, which is what moves.
 */
export function TogetherPanel(props: { readonly children: ReactNode }): JSX.Element {
  const [box] = useState(() => document.createElement("div"));
  const homeRef = useRef<HTMLDivElement>(null);
  const target = useSyncExternalStore(subscribe, currentDialogBody, currentDialogBody);
  useEffect(() => {
    panels += 1;
    tellChanged();
    return () => {
      panels -= 1;
      tellChanged();
    };
  }, []);
  useEffect(() => {
    (target ?? homeRef.current)?.append(box);
  }, [target, box]);
  return (
    <div ref={homeRef} className="together-panel">
      {createPortal(props.children, box)}
    </div>
  );
}

/**
 * Watch Together's sign over the film, in the player's top row, shown while a session's panel is
 * drawn, blue as the tray's is while its session is on; it opens the panel in a dialog over the film,
 * which plays on. While it is open, onOpenChange holds the overlay shown and gives the dialog its
 * presses, as the menus do.
 */
export function TogetherSignOverTheFilm(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const isShown = useSyncExternalStore(subscribe, hasPanel, hasPanel);
  const [isOpen, setIsOpen] = useState(false);
  const { onOpenChange } = props;
  const openOrClose = (isNowOpen: boolean): void => {
    setIsOpen(isNowOpen);
    onOpenChange(isNowOpen);
  };
  useEffect(
    () => () => {
      dialogBody = null;
      tellChanged();
    },
    [],
  );
  if (!isShown) {
    return null;
  }
  return (
    <span className="player-overlay__together">
      <IconButton
        label={t("status.togetherOn")}
        icon={<TogetherIcon />}
        mark={signMark(true)}
        isOverVideo
        onClick={() => {
          openOrClose(true);
        }}
      />
      {isOpen ? (
        <ModalDialog
          label={t("status.together")}
          onClose={() => {
            dialogBody = null;
            tellChanged();
            openOrClose(false);
          }}
        >
          <div
            ref={(body) => {
              if (body !== null && dialogBody !== body) {
                dialogBody = body;
                tellChanged();
              }
            }}
          />
        </ModalDialog>
      ) : null}
    </span>
  );
}

/**
 * node drawn over the film, inside its stage, so it shows while the film fills the window or the
 * screen; where video has no stage yet, where it stands.
 */
export function onTheFilmsStage(video: HTMLVideoElement | null, node: JSX.Element): JSX.Element {
  const stage = video?.closest(".film-player__stage");
  return stage instanceof HTMLElement ? createPortal(node, stage) : node;
}

/** How long a note of someone joining or leaving shows over the film, in ms. */
export const comingAndGoingShownMs = 5000;

/**
 * The names of those in the session with a device in it now; undefined while this page is not
 * connected, its roster then not the session's.
 */
function presentNames(snapshot: TogetherSnapshot | undefined): readonly string[] | undefined {
  return snapshot?.status === "connected"
    ? snapshot.roster.filter((member) => member.devices.length > 0).map((member) => member.name)
    : undefined;
}

/** The names a roster's key holds (ComingAndGoing). */
function namesOf(key: string): readonly string[] {
  return key.split("\u0000").filter((name) => name !== "");
}

/**
 * A short note over the film as someone joins the session or leaves it (D423), so a person watching
 * learns it without opening the panel; who was there as this page came is no news.
 */
export function ComingAndGoing(props: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly video: HTMLVideoElement | null;
}): JSX.Element | null {
  const { t } = useTranslation();
  const names = presentNames(props.snapshot);
  const key = names?.join("\u0000");
  // Who was there as the page came is no news: the first connected roster is only the measure, and
  // who was there last stays it across a drop. Measured as the roster changes, during the render.
  const [measured, setMeasured] = useState<string | undefined>(undefined);
  const [note, setNote] = useState<{ readonly text: string } | undefined>(undefined);
  if (key !== undefined && key !== measured) {
    setMeasured(key);
    if (measured !== undefined) {
      const was = namesOf(measured);
      const now = namesOf(key);
      const lines = [
        ...now.filter((name) => !was.includes(name)).map((name) => t("together.cameIn", { name })),
        ...was.filter((name) => !now.includes(name)).map((name) => t("together.wentOut", { name })),
      ];
      if (lines.length > 0) {
        setNote({ text: lines.join(" · ") });
      }
    }
  }
  useEffect(() => {
    if (note === undefined) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setNote(undefined);
    }, comingAndGoingShownMs);
    return () => {
      clearTimeout(timer);
    };
  }, [note]);
  if (note === undefined) {
    return null;
  }
  return onTheFilmsStage(props.video, <output className="together-coming">{note.text}</output>);
}
