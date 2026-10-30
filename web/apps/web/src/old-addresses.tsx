// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

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
 * An address of Watch Together's page, which D384 §2 made a dialog, as a history entry may hold one:
 * once the session this browser is in says what it plays, the Play page of the group's film in its
 * place, the one page that plays it; with nothing playing, no session or one ended, home, Watch
 * Together's dialog open over it. A signed-in guest's own page (/together) goes elsewhere: to the
 * group's film, or a page waiting for it (ToTheGroupsFilm), never the dialog.
 */
export function TogetherLeft(): null {
  const { navigation } = useServices();
  const hosting = useMembership(tabStorage, "host");
  const viewing = useMembership(browserStorage, "viewer");
  const membership = hosting ?? viewing;
  const { snapshot } = useTogether({
    credential: membership?.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  const fileId = snapshot?.status === "connected" ? (snapshot.group?.fileId ?? 0) : undefined;
  const isEnded = snapshot?.status === "ended";
  const hasNone = membership === undefined;
  useEffect(() => {
    if (fileId !== undefined && fileId !== 0) {
      navigation.replace({ screen: "play", fileId });
    } else if (hasNone || fileId === 0 || isEnded) {
      navigation.replace({ screen: "home" });
      openTrayDialog("together");
    }
  }, [fileId, isEnded, hasNone, navigation]);
  return null;
}
