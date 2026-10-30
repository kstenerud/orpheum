// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

import { useToken } from "./services.tsx";
import { useSessionEnd } from "./together-session-end.ts";
import type { Membership } from "./together-storage.ts";
import { useTogether } from "./use-together.ts";

/** What a guest's session plays, as it says: a film, nothing, or that the session is over. */
export type GroupPlays =
  | { readonly kind: "film"; readonly fileId: number }
  | { readonly kind: "nothing" }
  | { readonly kind: "over" };

/**
 * How long a guest's page waits for their session to say what it plays before it goes on without the
 * answer, in milliseconds: a round trip once the session's socket is open; a session that has not
 * said by then holds up no film. A Play page then plays its film as the person's own (WhoseFilm); a
 * guest's way in waits for the group's film on a page of its own (ToTheGroupsFilm). A justified
 * constant.
 */
export const groupKnownWithinMs = 3000;

/**
 * Asks a signed-in guest's session what it plays, by a panel of it, which plays nothing and goes once
 * told; one that has not said within groupKnownWithinMs is taken to play nothing; a session over,
 * ended or no longer there, ends for the guest (useSessionEnd): home, told why.
 */
export function AsksWhatPlays(props: {
  readonly membership: Membership;
  readonly onAnswered: (plays: GroupPlays) => void;
}): null {
  const { snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  const { over } = useSessionEnd(true, undefined);
  const fileId = snapshot?.status === "connected" ? (snapshot.group?.fileId ?? 0) : undefined;
  const endReason = snapshot?.status === "ended" ? (snapshot.endReason ?? "gone") : undefined;
  const { onAnswered } = props;
  useEffect(() => {
    if (fileId !== undefined) {
      onAnswered(fileId === 0 ? { kind: "nothing" } : { kind: "film", fileId });
    }
  }, [fileId, onAnswered]);
  useEffect(() => {
    if (endReason !== undefined) {
      onAnswered({ kind: "over" });
      over.end(endReason);
    }
  }, [endReason, over, onAnswered]);
  useEffect(() => {
    const timer = setTimeout(() => {
      onAnswered({ kind: "nothing" });
    }, groupKnownWithinMs);
    return () => {
      clearTimeout(timer);
    };
  }, [onAnswered]);
  return null;
}
