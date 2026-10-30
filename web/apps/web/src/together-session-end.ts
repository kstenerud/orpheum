// SPDX-License-Identifier: MPL-2.0

import type { TogetherEndReason } from "@orpheum/sdk";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { homeTold } from "./notice.ts";
import { useServices } from "./services.tsx";
import { haveBackMySessions } from "./together-mine.ts";
import {
  forgetEndedHere,
  forgetMembership,
  isEndedHere,
  noteEndedHere,
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

/**
 * Has back, signed in with no link in the address, a session joined on another device (D267 §2);
 * then, with no session here, the one they were in is gone, lost with a restart of the server or
 * ended while their page was away, and ends as one that ended does. It said "This address has no
 * invitation in it" (the owner's Mac, staging, 2026-10-29).
 */
export function useGoneFromUnder(page: {
  readonly isSignedIn: boolean;
  readonly secret: string | undefined;
  readonly hasMembership: boolean;
  readonly over: SessionEndWays;
}): void {
  const { api, navigation } = useServices();
  const shouldAsk = page.isSignedIn && page.secret === undefined;
  const [hasAsked, setHasAsked] = useState(false);
  useEffect(() => {
    if (!shouldAsk) {
      return undefined;
    }
    let isCurrent = true;
    void haveBackMySessions(api)
      .catch(() => undefined)
      .finally(() => {
        if (isCurrent) {
          setHasAsked(true);
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [api, shouldAsk]);
  const isGone = shouldAsk && hasAsked && !page.hasMembership;
  const { over } = page;
  useEffect(() => {
    // Only while this is the page shown: a session that ended has taken its guest home already.
    if (isGone && navigation.route().screen === "together") {
      over.goneFromUnder();
    }
  }, [isGone, navigation, over]);
}
