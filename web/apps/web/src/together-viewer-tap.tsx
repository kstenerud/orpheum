// SPDX-License-Identifier: MPL-2.0

import {
  readyForSoundAtTap,
  type BrowserVoiceDevices,
  type RosterMember,
  type TogetherConnection,
} from "@orpheum/sdk";
import { use, useCallback, useEffect, useMemo, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { InShellContext } from "./app-shell.tsx";
import { useServices } from "./services.tsx";

/** The one video every film of the session plays in, and whether the page has had a tap yet. */
export interface KeptVideo {
  readonly video: HTMLVideoElement;
  readonly hasHadATap: boolean;
  /** From a tap the page's listener may not hear as a click, as the Join's submitting: the same. */
  readonly tapped: () => void;
}

/**
 * The video every film of the session plays in, which each tap anywhere on the page, made before its
 * film or between films, the Join's among them, lets play with sound (readyForSoundAtTap), as the
 * same tap readies the voices (playWaitingAtTaps): one tap gives a guest both, whichever starts first
 * (the owner, 2026-10-13). A video made per film needed a tap of its own on an iPhone.
 */
export function useKeptVideo(hasHadATapAlready = false): KeptVideo {
  const [video] = useState(() => document.createElement("video"));
  const [hasHadATap, setHasHadATap] = useState(hasHadATapAlready);
  const tapped = useCallback(() => {
    readyForSoundAtTap(video);
    setHasHadATap(true);
  }, [video]);
  useEffect(() => {
    document.addEventListener("click", tapped, true);
    return () => {
      document.removeEventListener("click", tapped, true);
    };
  }, [tapped]);
  return { video, hasHadATap, tapped };
}

/**
 * Whether the page has had a press since it loaded (the browser's sticky activation), which lets a
 * film play with sound: a page reached by one, as a signed-in guest's Play page from the tray's
 * dialog, asks for no tap to continue.
 */
export function hasBeenPressed(): boolean {
  const activation: unknown = Reflect.get(navigator, "userActivation");
  return (
    typeof activation === "object" &&
    activation !== null &&
    Reflect.get(activation, "hasBeenActive") === true
  );
}

/**
 * The guest's voice devices, the page's from its opening to its closing, so the Join's tap readies
 * them as it does the kept video, and they last from one membership to the next.
 */
export function useViewerVoiceDevices(): BrowserVoiceDevices {
  const { voiceDevices } = useServices();
  const devices = useMemo(() => voiceDevices(), [voiceDevices]);
  useEffect(
    () => () => {
      devices.close();
    },
    [devices],
  );
  return devices;
}

/**
 * The page of a guest back in their session who has not tapped it since it opened, as after a
 * reload: who invited them and Tap to continue, whose tap is the one that lets the film and the
 * voices play with sound (the owner, 2026-10-13: "put an opaque overlay on top of it that just says
 * 'You've been invited by X to watch together. Tap to continue'"), and nothing of the session
 * until then: "as if it really were a separate page". A guest who joined has tapped Join, and never
 * sees it.
 */
export function TapToContinue(props: {
  readonly roster: readonly RosterMember[];
  /** The session's connection once connected, by which the page says how it came to be shown. */
  readonly connection: Pick<TogetherConnection, "note"> | undefined;
  readonly hasJoinedHere: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const host = props.roster.find((member) => member.isHost)?.name;
  useToldContinueAsked(props.connection, props.hasJoinedHere);
  return (
    <ViewerPage className="together-viewer__continue">
      <p>
        {host === undefined ? t("together.invitedBySomeone") : t("together.invitedBy", { host })}
      </p>
      {/* Its tap is the page's (useKeptVideo, playWaitingAtTaps): the button only asks for one. */}
      <button type="button">{t("together.tapToContinue")}</button>
    </ViewerPage>
  );
}

/**
 * A Watch Together guest's page: the whole page, its own main landmark, for a guest by link alone,
 * who sees none of the app (D246 §3); a screen in the shell, under its banner, for a signed-in guest,
 * who keeps the tray, the menu and Your account (D372 §9; the owner, 2026-10-29: a guest's page had no
 * banner, so Show debug tools, or a cast from it, needed a second tab).
 */
export function ViewerPage(props: {
  readonly className: string;
  readonly children: ReactNode;
}): JSX.Element {
  const isInShell = use(InShellContext);
  return isInShell ? (
    <div className={`${props.className} ${props.className}--in-shell`}>{props.children}</div>
  ) : (
    <main className={props.className}>{props.children}</main>
  );
}

/** Whether the page is a screen in the shell, whose banner already shows the product's logo. */
export function useIsInShell(): boolean {
  return use(InShellContext);
}

/**
 * Tells the server, once connected, that the page asks for a tap to continue: how the browser came
 * to show it, how long since it loaded, and whether it joined itself. An iPhone that joined by its
 * keyboard's Go was asked straight after, and nothing said whether its page had loaded again (the
 * owner's test, 2026-10-13).
 */
function useToldContinueAsked(
  connection: Pick<TogetherConnection, "note"> | undefined,
  hasJoinedHere: boolean,
): void {
  useEffect(() => {
    if (connection === undefined) {
      return;
    }
    const entry = performance.getEntriesByType("navigation").at(0);
    const type = entry !== undefined && "type" in entry ? entry.type : "navigate";
    connection.note({
      kind: "continueAsked",
      why: isNavigationType(type) ? type : "navigate",
      page: `loadedMsAgo=${String(Math.round(performance.now()))} joinedHere=${String(hasJoinedHere)}`,
    });
  }, [connection, hasJoinedHere]);
}

const navigationTypes = ["navigate", "reload", "back_forward", "prerender"] as const;

/** Whether type is a way a browser shows a page that the server knows. */
function isNavigationType(type: unknown): type is (typeof navigationTypes)[number] {
  return navigationTypes.some((known) => known === type);
}
