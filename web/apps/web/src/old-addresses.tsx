// SPDX-License-Identifier: MPL-2.0

import type { TogetherEndReason } from "@orpheum/sdk";
import { useEffect, useRef } from "react";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { useLive } from "./live.tsx";
import { useServices, useToken } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import { openTrayDialog } from "./tray-dialog.ts";
import { useTogether } from "./use-together.ts";

/**
 * An address of the casting page, which D384 §2 took away, as a history entry may still hold one:
 * once the server has said what this device casts, the Play page of the film the cast plays, whose
 * video's area is its remote, in its place, else home. Never a film started here by it (D383 §1).
 */
export function CastingLeft(): null {
  const { navigation } = useServices();
  const { sending, isSendingKnown } = useLive();
  const fileId = sending?.isAccepted === true ? sending.fileId : 0;
  useEffect(() => {
    if (isSendingKnown) {
      navigation.replace(fileId === 0 ? { screen: "home" } : { screen: "play", fileId });
    }
  }, [isSendingKnown, fileId, navigation]);
  return null;
}

/**
 * An address of Watch Together's page, which D384 §2 made a dialog, as a history entry or a signed-in
 * guest's way in may hold one: once the session this browser is in says what it plays, the Play page
 * of the group's film in its place, the one page that plays it; with nothing playing, or no session,
 * home, Watch Together's dialog open over it. A session that turns out ended is told onEnded, where
 * given (a signed-in guest's, whose page says why), else goes home so too.
 */
export function TogetherLeft(props: {
  readonly onEnded?: ((reason: TogetherEndReason) => void) | undefined;
}): null {
  const { navigation } = useServices();
  const hosting = useMembership(tabStorage, "host");
  const viewing = useMembership(browserStorage, "viewer");
  const membership = hosting ?? viewing;
  const { snapshot } = useTogether({
    credential: membership?.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  const endedRef = useRef(props.onEnded);
  useEffect(() => {
    endedRef.current = props.onEnded;
  });
  const fileId = snapshot?.status === "connected" ? (snapshot.group?.fileId ?? 0) : undefined;
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const hasNone = membership === undefined;
  useEffect(() => {
    if (endReason !== undefined && endedRef.current !== undefined) {
      endedRef.current(endReason);
    } else if (fileId !== undefined && fileId !== 0) {
      navigation.replace({ screen: "play", fileId });
    } else if (hasNone || fileId === 0 || endReason !== undefined) {
      navigation.replace({ screen: "home" });
      openTrayDialog("together");
    }
  }, [fileId, endReason, hasNone, navigation]);
  return null;
}
