// SPDX-License-Identifier: MPL-2.0

import "./together-over-the-film.css";

import type { TogetherSnapshot } from "@orpheum/sdk";
import {
  use,
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { IconButton } from "./icon-button.tsx";
import { useLive } from "./live.tsx";
import { LaterScreen } from "./screen-load.tsx";
import { signMark, TogetherIcon } from "./sign-mark.tsx";
import { TogetherDialog } from "./together-dialog-code.tsx";
import { CanStartTogetherContext } from "./together-can-start.ts";
import { useMembership } from "./together-membership.ts";
import { useHostedElsewhere } from "./together-mine.ts";
import { PanelInTheDialogContext } from "./together-panel-place.tsx";

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
 * while that is open. Its children are drawn once, into a box of their own, which is what moves, and
 * are told whether it is in the dialog (PanelInTheDialogContext). Where isHomeHidden, as on the
 * cast's remote, whose Watch Together button holds the rest (D440 §5, §6), it shows only in the
 * dialog: on its page it stays drawn, unseen, so what it holds goes on.
 */
export function TogetherPanel(props: {
  readonly children: ReactNode;
  readonly isHomeHidden?: boolean | undefined;
}): JSX.Element {
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
    <div ref={homeRef} className="together-panel" hidden={props.isHomeHidden === true}>
      {createPortal(
        <PanelInTheDialogContext value={target !== null}>{props.children}</PanelInTheDialogContext>,
        box,
      )}
    </div>
  );
}

/**
 * Watch Together's sign over the film, in the player's top row. While a session's panel is drawn it
 * is blue, as the tray's is while its session is on, and opens Watch Together's dialog, the one the
 * tray's sign opens, over the film, which plays on, with the panel moved in under its controls
 * (D433): the host's controls with End, or the guest's Leave. With no session it shows, white, to one
 * who may start one (CanStartTogetherContext), and opens the same dialog, whose start is the tray's,
 * so a session starts from full screen; the new session's panel is moved in as it is drawn, and the
 * dialog is then the host's (D440 §6). A host whose session is on another of their devices is told
 * so, and the dialog offers to move it here (D442 §2). A member of a session whose panel is not drawn
 * here is offered nothing, as before. Ending or leaving the session there closes it, as the tray's does. While it is
 * open, onOpenChange holds the overlay shown and gives the dialog its presses, as the menus do.
 */
export function TogetherSignOverTheFilm(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  return <TogetherButton isOverVideo onOpenChange={props.onOpenChange} />;
}

/**
 * Watch Together's one button (D440 §6), over the film (TogetherSignOverTheFilm) or on the cast's
 * remote (together-on-the-remote.tsx), as TogetherSignOverTheFilm says: the session's panel moved
 * into the dialog it opens, or, with no session, the start to one who may host.
 */
export function TogetherButton(props: {
  readonly isOverVideo: boolean;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const isOn = useSyncExternalStore(subscribe, hasPanel, hasPanel);
  const canStart = use(CanStartTogetherContext);
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  // A device receiving a cast plays the cast's session, not one of its own (PlaysAlong, D88).
  const isReceiving = useLive().receiving !== undefined;
  // The session the person hosts on another device: said to be on, there, and offered to move here
  // (D442 §2), never to start another, which would end it.
  const showsElsewhere =
    useHostedElsewhere() !== undefined && !isHosting && !isViewing && !isReceiving;
  const showsStart = canStart && !isHosting && !isViewing && !isReceiving && !showsElsewhere;
  const [isOpen, setIsOpen] = useState(false);
  const { onOpenChange } = props;
  const openOrClose = useCallback(
    (isNowOpen: boolean): void => {
      if (!isNowOpen) {
        dialogBody = null;
        tellChanged();
      }
      setIsOpen(isNowOpen);
      onOpenChange(isNowOpen);
    },
    [onOpenChange],
  );
  useEffect(
    () => () => {
      dialogBody = null;
      tellChanged();
    },
    [],
  );
  useClosedAsThePanelGoes(isOpen, openOrClose);
  // Open, it stays while the session it started is had back and its panel drawn.
  if (!isOn && !showsStart && !showsElsewhere && !isOpen) {
    return null;
  }
  return (
    <span className={props.isOverVideo ? "player-overlay__together" : "together-button"}>
      <IconButton
        label={t(buttonLabel(isOn, showsElsewhere))}
        icon={<TogetherIcon />}
        mark={signMark(isOn)}
        isOverVideo={props.isOverVideo}
        onClick={() => {
          openOrClose(true);
        }}
      />
      {isOpen ? (
        <TogetherDialogOverTheFilm
          onClose={() => {
            openOrClose(false);
          }}
        />
      ) : null}
    </span>
  );
}

/** What the button says: the session on here, on another device of the host's, or none. */
function buttonLabel(isOn: boolean, isElsewhere: boolean): string {
  if (isOn) {
    return "status.togetherOn";
  }
  return isElsewhere ? "status.togetherElsewhere" : "status.together";
}

/**
 * Closes the dialog over the film as the session's panel goes while it is open, its session left or
 * ended (D433 §3): the dialog goes with the sign, and the overlay is told so. Opened with no session
 * to start one (D440 §6), it waits: only a panel drawn while it is open going closes it.
 */
function useClosedAsThePanelGoes(isOpen: boolean, openOrClose: (isNowOpen: boolean) => void): void {
  useEffect(() => {
    if (!isOpen) {
      return undefined;
    }
    let isClosed = false;
    let wasPanelDrawn = hasPanel();
    return subscribe(() => {
      const isPanelDrawn = hasPanel();
      if (!isClosed && wasPanelDrawn && !isPanelDrawn) {
        isClosed = true;
        openOrClose(false);
      }
      wasPanelDrawn = isPanelDrawn;
    });
  }, [isOpen, openOrClose]);
}

/**
 * Watch Together's dialog, the tray's (TogetherDialog, its code loaded as the tray's is), opened over
 * the film with a place under its controls the session's panel is moved into (D433 §2).
 */
function TogetherDialogOverTheFilm(props: { readonly onClose: () => void }): JSX.Element {
  return (
    <LaterScreen resetKey="together-dialog">
      <TogetherDialog
        onClose={props.onClose}
        panelPlace={
          <div
            ref={(body) => {
              if (body !== null && dialogBody !== body) {
                dialogBody = body;
                tellChanged();
              }
            }}
          />
        }
      />
    </LaterScreen>
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
