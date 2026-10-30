// SPDX-License-Identifier: MPL-2.0

import "./watch-together.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { useServices, useToken } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import type { Membership } from "./together-storage.ts";
import { useTogether } from "./use-together.ts";
import { WatchTogetherControls } from "./watch-together.tsx";

/**
 * Watch Together's dialog, which the tray's Watch Together opens over whatever page is shown and which
 * never moves it (D384 §2; the owner: "Watch together keeps track of the shared video being played
 * and offers a way to get back to it, but ultimately there is one playback page that everything goes
 * through"): for one who may host, starting a session and running it, what plays with the way back
 * to its film's Play page and Stop for everyone, voice and the volumes, the people and Invite, and
 * ending it (watch-together.tsx), which closes the dialog, nothing opening it again (D385's Also); for
 * a signed-in guest, the way back to the group's film.
 */
export function TogetherDialog(props: { readonly onClose: () => void }): JSX.Element {
  const { t } = useTranslation();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const viewing = useMembership(browserStorage, "viewer");
  return (
    <ModalDialog
      label={t("watchTogether.heading")}
      onClose={props.onClose}
      className="together-dialog"
    >
      <section className="watch-together" aria-labelledby="watch-together-heading">
        <h2 id="watch-together-heading">{t("watchTogether.heading")}</h2>
        {viewing !== undefined && !isHosting ? (
          <GuestsSession membership={viewing} />
        ) : (
          <WatchTogetherControls onEnded={props.onClose} />
        )}
      </section>
    </ModalDialog>
  );
}

/**
 * A signed-in guest's session, in Watch Together's dialog: the way back to the group's film, whose
 * Play page is theirs in the session (voice, their volume and Leave under it), or that nothing plays
 * yet. Read by a panel of the session while the dialog is open, which plays nothing.
 */
function GuestsSession(props: { readonly membership: Membership }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  if (snapshot?.status !== "connected") {
    return <p>{t("common.loading")}</p>;
  }
  const fileId = snapshot.group?.fileId ?? 0;
  if (fileId === 0) {
    return <p>{t("watchTogether.nothingPlays")}</p>;
  }
  return (
    <button
      type="button"
      className="title__play"
      onClick={() => {
        navigation.go({ screen: "play", fileId });
      }}
    >
      {t("watchTogether.backToTheFilm")}
    </button>
  );
}
