// SPDX-License-Identifier: MPL-2.0

import type { TogetherEndReason } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { secretFromHash } from "./accounts.ts";
import { browserStorage } from "./browser-storage.ts";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { NoLink } from "./together-join.tsx";
import type { SessionsAsked } from "./together-session-end.ts";
import { forgetMembership, readMembership } from "./together-storage.ts";
import { ViewerPage } from "./together-viewer-tap.tsx";

// A Watch Together viewer's page when it is in no session: no link and no session to be in, a link
// pasted into the tab, and the session over.

/**
 * No link in the address and no session here: signed in, the server is asked for their session
 * first, and without one they are taken home (useGoneFromUnder); else there is nothing to join.
 */
export function NoSessionHere(props: {
  readonly isSignedIn: boolean;
  readonly asked: SessionsAsked | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { asked } = props;
  if (!props.isSignedIn) {
    return <NoLink />;
  }
  if (asked === undefined) {
    return <p>{t("common.loading")}</p>;
  }
  return (
    <>
      <p role="alert">{problemText(t, asked.failure)}</p>
      <button type="button" onClick={asked.askAgain}>
        {t("together.askAgain")}
      </button>
    </>
  );
}

/**
 * A link pasted into a tab already here changes only what follows #, and the page does not load
 * again: the new link is joined in place of whatever this browser was in, unless it is the same.
 */
export function usePastedLinks(setSecret: (secret: string) => void, again: () => void): void {
  const { navigation } = useServices();
  useEffect(
    () =>
      navigation.subscribe(() => {
        const pasted = secretFromHash(navigation.fragment());
        if (pasted !== undefined) {
          if (readMembership(browserStorage(), "viewer")?.link !== pasted) {
            forgetMembership(browserStorage(), "viewer");
          }
          setSecret(pasted);
          again();
        }
      }),
    [navigation, setSecret, again],
  );
}

/**
 * Why the session is over for this viewer, and that the page has nothing more to show; or, sent away
 * because the host's connection could not carry everyone, a way back in (D286 §5).
 */
export function SessionOver(props: {
  readonly reason: TogetherEndReason;
  readonly onAgain: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ViewerPage className="together-viewer" inShellClassName="together-viewer--in-shell">
      <output>{t(`together.over.${props.reason}`)}</output>
      {props.reason === "uploadFull" ? (
        <button type="button" onClick={props.onAgain}>
          {t("together.over.joinAgain")}
        </button>
      ) : null}
    </ViewerPage>
  );
}
