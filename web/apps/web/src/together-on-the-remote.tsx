// SPDX-License-Identifier: MPL-2.0

import "./together-on-the-remote.css";
import "./cast-words.ts";

import type { TogetherConnection, TogetherSnapshot } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { useLive } from "./live.tsx";
import { SessionRemote } from "./session-remote.tsx";
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
 * Watch Together on a guest's page while it casts the group's film, the phone showing no film
 * (D440 §5): as over the film (§2, §3, §6), the slider row, each other person's in the session; Talk
 * beside it; and Watch Together's one button, the session's panel moved into the dialog it opens,
 * the guest's Leave; then what lies behind the remote (TogetherBehindTheRemote). The cast's own
 * remote draws the overlay's 🔈, Talk and Watch Together's sign instead (D444), and the part behind.
 */
export function TogetherOnTheRemote(): JSX.Element {
  const voice = useSessionVoice();
  return (
    <div className="together-on-the-remote">
      <RemoteVolumeRow voice={voice} />
      <div className="together-on-the-remote__buttons">
        <SessionTalk isOverVideo={false} />
        <TogetherButton isOverVideo={false} onOpenChange={noOverlay} />
      </div>
      <TogetherBehindTheRemote />
    </div>
  );
}

/**
 * Whether this page casts the group's film (useIsCastingTheGroupsFilm), by the tray's cast or a guest
 * by link's (guestCast, D518), and, where it is known where to, the stage it shows (CastingStage).
 */
export function useCastingTheGroupsFilm(
  snapshot: TogetherSnapshot | undefined,
  guestCast: { readonly castingTo: string | undefined; readonly stop: () => void } | undefined,
): {
  readonly isCasting: boolean;
  readonly stage: { readonly castingTo: string; readonly onStop?: () => void } | undefined;
} {
  const { sending } = useLive();
  const guests = guestCast?.castingTo === undefined ? undefined : guestCast;
  const isCasting = useIsCastingTheGroupsFilm(snapshot, guests !== undefined);
  const castingTo = guests?.castingTo ?? sending?.receiverName;
  if (!isCasting || castingTo === undefined) {
    return { isCasting, stage: undefined };
  }
  return {
    isCasting,
    stage: guests === undefined ? { castingTo } : { castingTo, onStop: guests.stop },
  };
}

/**
 * A guest's stage while their device casts the group's film (together-viewer.tsx's ViewingArea): the
 * device it plays on, the cast's controls, Watch Together's among them, the phone the control centre
 * (D440 §5), and, where onStop is given, Stop casting: a guest by link's, whose cast is not the
 * tray's (D518).
 */
export function CastingStage(props: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly connection: TogetherConnection | undefined;
  readonly castingTo: string;
  readonly onStop?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="together-viewer__viewing" aria-label={t("together.stage")}>
      <p>{t("cast.playingOn", { name: props.castingTo })}</p>
      {props.onStop === undefined ? null : (
        <button type="button" onClick={props.onStop}>
          {t("cast.stop")}
        </button>
      )}
      <SessionRemote snapshot={props.snapshot} connection={props.connection} durationMs={0} />
      <TogetherOnTheRemote />
    </section>
  );
}

/**
 * What of Watch Together lies behind a remote: the voices' tap, where they wait for one, on the
 * page, and, for a host, the session's panel. The voice is the page's, the host's tab-wide
 * (HostVoiceProvider) or the guest's page's (together-viewer.tsx), so it plays on as the page turns
 * from the player to the remote and back: nothing here holds it. The host's player, whose panel
 * Watch Together's button moves into its dialog (the host's people, grants and End), is not drawn
 * while casting, so the host's panel is drawn here (hostedSnapshot, the session this tab hosts as
 * the remote reads it), on the page unseen; the guest's page draws its own, and hides it on its page
 * while casting. With no session, the button's dialog starts one from the remote.
 */
export function TogetherBehindTheRemote(props: {
  readonly hostedSnapshot?: TogetherSnapshot | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const voice = useSessionVoice();
  // The host's session voice is the tab's (HostVoiceProvider), the one this page holds.
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  return (
    <>
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
    </>
  );
}

/**
 * Whether this device casts while its session plays a film: the device it casts to plays the film
 * in its place (together-viewer.tsx's ViewingArea), and the page is the cast's controls. A guest by
 * link casts with no cast made (isTheGuestsPartCast): a Chromecast takes their part (D518).
 */
export function useIsCastingTheGroupsFilm(
  snapshot: TogetherSnapshot | undefined,
  isTheGuestsPartCast = false,
): boolean {
  const { sending } = useLive();
  const isCasting = isTheGuestsPartCast || sending?.isAccepted === true;
  return isCasting && (snapshot?.group?.fileId ?? 0) !== 0;
}
