// SPDX-License-Identifier: MPL-2.0

import type { TogetherEndReason } from "@orpheum/sdk";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { homeTold } from "./notice.ts";
import { useServices } from "./services.tsx";
import { haveBackMySessions, noteHostedElsewhere } from "./together-mine.ts";
import {
  forgetEndedHere,
  forgetMembership,
  isEndedHere,
  noteEndedHere,
  readMembership,
} from "./together-storage.ts";

/** How a guest's session ends for their page, and how it starts again (useSessionEnd). */
export interface SessionEndWays {
  readonly end: (reason: TogetherEndReason) => void;
  /** A signed-in guest's session gone without a word to this page: home, told so. */
  readonly goneFromUnder: () => void;
  readonly again: () => void;
}

/**
 * Why the session is over for this viewer, if it is, and how it ends and starts again: ended, the
 * membership is forgotten, but for a viewer sent away for the host's upload, who is still a member
 * and may come back (D286 §5). A signed-in guest has the app to go back to: ended, they are taken
 * home, told why there, as a page that said only "You can close this page" stopped their app in
 * its tracks (the owner, 2026-10-29); a guest by link alone has nowhere else, and is told here, and
 * again on the page loaded again, unless a link is in its address.
 */
export function useSessionEnd(
  isSignedIn: boolean,
  secret: string | undefined,
): { readonly endReason: TogetherEndReason | undefined; readonly over: SessionEndWays } {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [endReason, setEndReason] = useState<TogetherEndReason | undefined>(() =>
    !isSignedIn && secret === undefined && isEndedHere(tabStorage()) ? "gone" : undefined,
  );
  const over = useMemo(() => {
    return {
      end: (reason: TogetherEndReason) => {
        if (reason === "uploadFull") {
          setEndReason(reason);
          return;
        }
        forgetMembership(browserStorage(), "viewer");
        if (isSignedIn) {
          homeTold(navigation, t(`together.overNotice.${reason}`));
          return;
        }
        noteEndedHere(tabStorage());
        setEndReason(reason);
      },
      goneFromUnder: () => {
        homeTold(navigation, t("together.overNotice.notThere"));
      },
      again: () => {
        forgetEndedHere(tabStorage());
        setEndReason(undefined);
      },
    };
  }, [isSignedIn, navigation, t]);
  return { endReason, over };
}

/** The sessions this tab's host is ending itself, whose end goes home with no word (useHostsSessionOver). */
const endingHere = new Set<number>();

/**
 * Notes that this tab's host ends sessionId itself, as it asks the server to, or, isEnding false, that
 * the asking failed: its end then takes this tab home with no word, the host having done it.
 */
export function noteEndingHere(sessionId: number, isEnding: boolean): void {
  if (isEnding) {
    endingHere.add(sessionId);
  } else {
    endingHere.delete(sessionId);
  }
}

/**
 * How the session this tab hosts ends for it, whatever ended it and whichever of the tab's
 * connections heard it first (D389 §1, the owner: "when a watch together session ends, ALL
 * associated devices go back home (including the host)"): its membership forgotten, and the page
 * home, its film played on alone no more; told why there, unless this tab ended it (noteEndingHere).
 * Moved to another device of theirs, it is noted as hosted there, so Watch Together offers to move
 * it back rather than to start another, which would end it (D442 §2). Once: a session that is no
 * longer this tab's to host is left as it is.
 */
export function useHostsSessionOver(): (sessionId: number, reason: TogetherEndReason) => void {
  const { t } = useTranslation();
  const { navigation } = useServices();
  return useCallback(
    (sessionId: number, reason: TogetherEndReason) => {
      if (readMembership(tabStorage(), "host")?.sessionId !== sessionId) {
        return;
      }
      forgetMembership(tabStorage(), "host");
      if (reason === "movedAway") {
        noteHostedElsewhere({ sessionId, deviceName: undefined });
      }
      if (endingHere.delete(sessionId)) {
        navigation.replace({ screen: "home" });
        return;
      }
      homeTold(navigation, t(hostsOverNotice(reason)));
    },
    [navigation, t],
  );
}

/** What the host is told at home of their session's end, which they did not make on this tab. */
function hostsOverNotice(reason: TogetherEndReason): string {
  if (reason === "hostEnded") {
    return "together.overNotice.endedElsewhere"; // on another of their devices, or one started there
  }
  if (reason === "movedAway") {
    return "together.overNotice.movedElsewhere"; // to another of their devices (D442 §2)
  }
  return reason === "idle" ? "together.overNotice.idle" : "together.overNotice.gone";
}

/** What asking the server for the person's sessions came to, where it failed: why, and asking again. */
export interface SessionsAsked {
  readonly failure: unknown;
  readonly askAgain: () => void;
}

/**
 * Has back, signed in with no link in the address, a session joined on another device (D267 §2);
 * then, with no session here, the one they were in is gone, lost with a restart of the server or
 * ended while their page was away, and ends as one that ended does. It said "This address has no
 * invitation in it" (the owner's Mac, staging, 2026-10-29). Only an answer says the session is gone:
 * a failed ask is said, with a way to ask again, as it took the guest home saying the session was no
 * longer there while it went on (principles §28). What the ask came to, where it failed.
 */
export function useGoneFromUnder(page: {
  readonly isSignedIn: boolean;
  readonly secret: string | undefined;
  readonly hasMembership: boolean;
  readonly over: SessionEndWays;
}): SessionsAsked | undefined {
  const { api, navigation } = useServices();
  const shouldAsk = page.isSignedIn && page.secret === undefined;
  const [hasAsked, setHasAsked] = useState(false);
  const [failure, setFailure] = useState<{ readonly cause: unknown } | undefined>(undefined);
  const [asking, setAsking] = useState(0);
  useEffect(() => {
    if (!shouldAsk) {
      return undefined;
    }
    let isCurrent = true;
    haveBackMySessions(api).then(
      () => {
        if (isCurrent) {
          setHasAsked(true);
        }
      },
      (cause: unknown) => {
        if (isCurrent) {
          setFailure({ cause });
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [api, shouldAsk, asking]);
  const isGone = shouldAsk && hasAsked && !page.hasMembership;
  const { over } = page;
  useEffect(() => {
    // Only while this is the page shown: a session that ended has taken its guest home already.
    if (isGone && navigation.route().screen === "together") {
      over.goneFromUnder();
    }
  }, [isGone, navigation, over]);
  return failure === undefined || hasAsked
    ? undefined
    : {
        failure: failure.cause,
        askAgain: () => {
          setFailure(undefined);
          setAsking((count) => count + 1);
        },
      };
}
