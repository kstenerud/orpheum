// SPDX-License-Identifier: MPL-2.0

import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCast } from "./cast.ts";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { useServices } from "./services.tsx";
import { SessionRemote } from "./session-remote.tsx";
import type { SessionDetails } from "./together-people.tsx";
import type { useTogether } from "./use-together.ts";

/**
 * What plays for the group, on the host's session page, with its controls (D364 §15): the remote of
 * the group's film (SessionRemote, the cast's own), the way back to the film, and Stop, which ends it
 * for everyone, as the host playing another film does; leaving the film does not (D246 §5's stop on
 * leaving withdrawn). While this device casts too, the page holds the cast's controls as well, and
 * Stop casting (D364 §11, §13). The host's device here plays nothing: it is a panel of the session,
 * whose connection the page holds (session).
 */
export function TogetherPlays(props: {
  readonly session: ReturnType<typeof useTogether>;
  readonly details: SessionDetails;
  /** Reads the session's details again: what plays changed from what they say. */
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { details, onChanged, session } = props;
  const fileId = session.snapshot?.group?.fileId ?? details.fileId ?? 0;
  const isNamedElse = fileId !== 0 && fileId !== (details.fileId ?? 0);
  useEffect(() => {
    if (isNamedElse) {
      onChanged(); // its name and length, which the details carry
    }
  }, [isNamedElse, onChanged]);
  return (
    <section className="together-plays" aria-labelledby="together-plays-heading">
      {fileId === 0 ? (
        <h2 id="together-plays-heading">{t("watchTogether.nothingPlays")}</h2>
      ) : (
        <FilmPlaying fileId={fileId} details={details} {...session} />
      )}
      <StopCasting />
    </section>
  );
}

/** The group's film: its name, its remote, the way back to it, and Stop for everyone. */
function FilmPlaying(props: {
  readonly fileId: number;
  readonly details: SessionDetails;
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
}): JSX.Element {
  const { t } = useTranslation();
  const { connection, details } = props;
  return (
    <>
      <h2 id="together-plays-heading">{details.titleName ?? t("cast.aFilm")}</h2>
      <SessionRemote
        snapshot={props.snapshot}
        connection={connection}
        durationMs={details.durationMs ?? 0}
      />
      <div className="together-plays__buttons">
        {/* The page's one primary action, styled as the title page's Play: the host and guests come
            back to watching here by it (the owner missed it as a plain link, 2026-10-29). */}
        <Link to={{ screen: "play", fileId: props.fileId }} className="title__play">
          {t("watchTogether.backToTheFilm")}
        </Link>
        <button
          type="button"
          onClick={() => {
            connection?.command({ command: "stop" });
          }}
        >
          {t("watchTogether.stopTheFilm")}
        </button>
      </div>
    </>
  );
}

/** Stop casting, while this device casts too: the cast's controls are this page's (D364 §11). */
function StopCasting(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { sending } = useLive();
  if (sending === undefined) {
    return null;
  }
  return (
    <p>
      {t("watchTogether.castingTo", { name: sending.receiverName })}{" "}
      <button
        type="button"
        onClick={() => {
          endCast(api, sending.castId, "stopped").catch(() => undefined); // the tray says it ended
        }}
      >
        {t("cast.stop")}
      </button>
    </p>
  );
}
