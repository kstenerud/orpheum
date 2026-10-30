// SPDX-License-Identifier: MPL-2.0

import { useCallback, useState, type ReactNode } from "react";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { useMembership } from "./together-membership.ts";
import { GroupFilmPage } from "./together-viewer.tsx";
import { AsksWhatPlays, type GroupPlays } from "./together-what-plays.tsx";

/**
 * Whose a Play page's film is (D384 §2, D372 §9): for a signed-in guest in a Watch Together session
 * that plays it, the group's, followed on this page with the session around it (GroupFilmPage); for
 * anyone else, or a film the group does not play, the person's own (own), a guest still a member
 * while they watch it. Decided once, as the session first says what it plays (AsksWhatPlays); a
 * session that cannot be reached leaves the film the person's own. A session found over ends as
 * every guest's session does (useSessionEnd): home, told why, the membership forgotten, the film never
 * played as their own: a page loaded again after the host ended it played the group's film on its
 * own (the owner's S23, staging, 2026-10-30), as did the page of a device that hosts it too, the
 * person's own, once the session ended under it.
 */
export function WhoseFilm(props: { readonly fileId: number; readonly own: ReactNode }): ReactNode {
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const viewing = useMembership(browserStorage, "viewer");
  const [whose, setWhose] = useState<Whose | undefined>(undefined);
  const { fileId } = props;
  const answered = useCallback(
    (plays: GroupPlays) => {
      setWhose(whoseOf(plays, fileId));
    },
    [fileId],
  );
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
  return <AsksWhatPlays membership={viewing} onAnswered={answered} />;
}

/** Whose a Play page's film is, as its session says: or that the session is over. */
type Whose = "group" | "own" | "over";

/** Whose a Play page's film fileId is, given what the guest's session plays. */
function whoseOf(plays: GroupPlays, fileId: number): Whose {
  if (plays.kind === "over") {
    return "over";
  }
  return plays.kind === "film" && plays.fileId === fileId ? "group" : "own";
}
