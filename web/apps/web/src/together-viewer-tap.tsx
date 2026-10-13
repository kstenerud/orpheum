// SPDX-License-Identifier: MPL-2.0

import { readyForSoundAtTap, type BrowserVoiceDevices, type RosterMember } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useServices } from "./services.tsx";

/** The one video every film of the session plays in, and whether the page has had a tap yet. */
export interface KeptVideo {
  readonly video: HTMLVideoElement;
  readonly hasHadATap: boolean;
}

/**
 * The video every film of the session plays in, which each tap anywhere on the page, made before its
 * film or between films, the Join's among them, lets play with sound (readyForSoundAtTap), as the
 * same tap readies the voices (playWaitingAtTaps): one tap gives a guest both, whichever starts first
 * (the owner, 2026-10-13). A video made per film needed a tap of its own on an iPhone.
 */
export function useKeptVideo(): KeptVideo {
  const [video] = useState(() => document.createElement("video"));
  const [hasHadATap, setHasHadATap] = useState(false);
  useEffect(() => {
    const tapped = (): void => {
      readyForSoundAtTap(video);
      setHasHadATap(true);
    };
    document.addEventListener("click", tapped, true);
    return () => {
      document.removeEventListener("click", tapped, true);
    };
  }, [video]);
  return { video, hasHadATap };
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
export function TapToContinue(props: { readonly roster: readonly RosterMember[] }): JSX.Element {
  const { t } = useTranslation();
  const host = props.roster.find((member) => member.isHost)?.name;
  return (
    <main className="together-viewer__continue">
      <p>
        {host === undefined ? t("together.invitedBySomeone") : t("together.invitedBy", { host })}
      </p>
      {/* Its tap is the page's (useKeptVideo, playWaitingAtTaps): the button only asks for one. */}
      <button type="button">{t("together.tapToContinue")}</button>
    </main>
  );
}
