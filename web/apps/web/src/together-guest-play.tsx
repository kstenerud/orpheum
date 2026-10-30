// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type ReactNode } from "react";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { useToken } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import { useSessionEnd } from "./together-session-end.ts";
import type { Membership } from "./together-storage.ts";
import { GroupFilmPage } from "./together-viewer.tsx";
import { useTogether } from "./use-together.ts";

/**
 * Whose a Play page's film is (D384 §2, D372 §9): for a signed-in guest in a Watch Together session
 * that plays it, the group's, followed on this page with the session around it (GroupFilmPage); for
 * anyone else, or a film the group does not play, the person's own (own), a guest still a member
 * while they watch it. Decided once, as the session first says what it plays; a session that cannot
 * be reached leaves the film the person's own. A session found over ends as every guest's session
 * does (useSessionEnd): home, told why, the membership forgotten, the film never played as their
 * own: a page loaded again after the host ended it played the group's film on its own (the owner's
 * S23, staging, 2026-10-30), as did the page of a device that hosts it too, the person's own, once
 * the session ended under it.
 */
export function WhoseFilm(props: { readonly fileId: number; readonly own: ReactNode }): ReactNode {
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const viewing = useMembership(browserStorage, "viewer");
  const [whose, setWhose] = useState<Whose | undefined>(undefined);
  if (whose === "over") {
    // Its session ended: the page goes home (useSessionEnd), and plays nothing meanwhile.
    return null;
  }
  if (viewing === undefined || isHosting || whose === "own") {
    return props.own;
  }
  if (whose === "group") {
    return <GroupFilmPage membership={viewing} />;
  }
  return <AsksWhose membership={viewing} fileId={props.fileId} onAnswered={setWhose} />;
}

/** Whose a Play page's film is, as its session says: or that the session is over (AsksWhose). */
type Whose = "group" | "own" | "over";

/**
 * How long a guest's Play page waits for their session to say what it plays before the film plays as
 * their own, in milliseconds: a round trip once the session's socket is open; a session that has not
 * said by then holds up no film. A justified constant.
 */
export const groupKnownWithinMs = 3000;

/**
 * Asks the guest's session what it plays, by a panel of it, which plays nothing and goes once told:
 * the group's where it plays fileId, else the guest's own, as too where it does not say within
 * groupKnownWithinMs; a session over, ended or no longer there, ends for the guest (useSessionEnd).
 */
function AsksWhose(props: {
  readonly membership: Membership;
  readonly fileId: number;
  readonly onAnswered: (whose: Whose) => void;
}): null {
  const { snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  const { over } = useSessionEnd(true, undefined);
  const playing = snapshot?.status === "connected" ? (snapshot.group?.fileId ?? 0) : undefined;
  const endReason = snapshot?.status === "ended" ? (snapshot.endReason ?? "gone") : undefined;
  const { fileId, onAnswered } = props;
  useEffect(() => {
    if (playing !== undefined) {
      onAnswered(playing === fileId ? "group" : "own");
    }
  }, [playing, fileId, onAnswered]);
  useEffect(() => {
    if (endReason !== undefined) {
      onAnswered("over");
      over.end(endReason);
    }
  }, [endReason, over, onAnswered]);
  useEffect(() => {
    const timer = setTimeout(() => {
      onAnswered("own");
    }, groupKnownWithinMs);
    return () => {
      clearTimeout(timer);
    };
  }, [onAnswered]);
  return null;
}
