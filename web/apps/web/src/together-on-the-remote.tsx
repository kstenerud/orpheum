// SPDX-License-Identifier: MPL-2.0

import "./together-on-the-remote.css";

import type { TogetherSnapshot } from "@orpheum/sdk";
import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { useLive } from "./live.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherNews } from "./together-news.tsx";
import { TogetherButton, TogetherPanel } from "./together-over-the-film.tsx";
import { SessionTalk, useSessionVoice, VoiceControls } from "./together-voice.tsx";
import { RemoteVolumeRow } from "./volume-row.tsx";

/** Leaves the dialog's opening and closing to the dialog: nothing over the remote waits on it. */
function noOverlay(): void {
  // The cast's remote has no overlay to hold shown, as the film's has.
}

/**
 * Watch Together on the cast's remote, the control centre while casting, the phone showing no film
 * (D440 §5): as over the film (§2, §3, §6), the slider row, the film's slider first (filmSlider, the
 * receiver's own, which the remote draws: until then the row holds the people's alone), then each
 * other person's in the session; Talk beside it; and Watch Together's one button, tuned to the role,
 * the session's panel moved into the dialog it opens: the guest's Leave, the host's people, grants and
 * End. With no session, the button alone, to one who may start one (CanStartTogetherContext), the
 * session then started from the remote. The voice is the page's, the host's tab-wide
 * (HostVoiceProvider) or the guest's page's (together-viewer.tsx), so it plays on as the page turns
 * from the player to the remote and back: nothing here holds it. The host's player, whose panel the
 * button moves, is not drawn while casting, so the host's panel is drawn here (hostedSnapshot, the
 * session this tab hosts as the remote reads it), on the page unseen; the guest's page draws its
 * own, and hides it on its page while casting. The voices' tap, where they wait for one, shows on the
 * page, not in the hidden panel.
 */
export function TogetherOnTheRemote(props: {
  readonly filmSlider?: ReactNode;
  readonly hostedSnapshot?: TogetherSnapshot | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const voice = useSessionVoice();
  // The host's session voice is the tab's (HostVoiceProvider), the one this page holds.
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  return (
    <div className="together-on-the-remote">
      <RemoteVolumeRow filmSlider={props.filmSlider} voice={voice} />
      <div className="together-on-the-remote__buttons">
        <SessionTalk isOverVideo={false} />
        <TogetherButton isOverVideo={false} onOpenChange={noOverlay} />
      </div>
      {voice !== undefined && voice.snapshot.isHearing && voice.tap.isWaiting ? (
        <button type="button" className="together-voice__tap" onClick={voice.tap.play}>
          {t("together.voice.tapToHear")}
        </button>
      ) : null}
      {!isHosting || voice === undefined ? null : (
        <TogetherPanel isHomeHidden>
          <TogetherNews snapshot={props.hostedSnapshot} />
          <VoiceControls {...voice} tap={{ ...voice.tap, isWaiting: false }} />
        </TogetherPanel>
      )}
    </div>
  );
}

/**
 * Whether this device casts while its session plays a film: the device it casts to plays the film
 * in its place (together-viewer.tsx's ViewingArea), and the page is the cast's controls.
 */
export function useIsCastingTheGroupsFilm(snapshot: TogetherSnapshot | undefined): boolean {
  const { sending } = useLive();
  return sending?.isAccepted === true && (snapshot?.group?.fileId ?? 0) !== 0;
}
