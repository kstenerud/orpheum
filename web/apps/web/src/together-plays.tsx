// SPDX-License-Identifier: MPL-2.0

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCast } from "./cast.ts";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { useServices } from "./services.tsx";
import type { SessionDetails } from "./together-people.tsx";
import type { useTogether } from "./use-together.ts";

/**
 * What plays for the group, in Watch Together's dialog (D364 §15, D384 §2): the group's film, the way
 * back to its Play page, the one page that plays it and holds its controls, and Stop, which ends it
 * for everyone, as the host playing another film does; leaving the film does not (D246 §5's stop on
 * leaving withdrawn). While this device casts too, Stop casting with it (D364 §11, §13). The dialog
 * plays nothing: it is a panel of the session, whose connection it holds (session).
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
        <FilmPlaying
          fileId={fileId}
          details={details}
          connection={session.connection}
          status={session.snapshot?.status}
        />
      )}
      <StopCasting />
    </section>
  );
}

/** The group's film: its name, the way back to it, and Stop for everyone. */
function FilmPlaying(props: {
  readonly fileId: number;
  readonly details: SessionDetails;
  readonly connection: ReturnType<typeof useTogether>["connection"];
  /** The connection's status as last read; Stop waits while it is not "connected". */
  readonly status: TogetherSnapshot["status"] | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { connection, details, status } = props;
  const isUp = status === "connected";
  return (
    <>
      <h2 id="together-plays-heading">{details.titleName ?? t("cast.aFilm")}</h2>
      <div className="together-plays__buttons">
        {/* The page's one primary action, styled as the title page's Play: the host and guests come
            back to watching here by it (the owner missed it as a plain link, 2026-10-29). */}
        <Link to={{ screen: "play", fileId: props.fileId }} className="title__play">
          {t("watchTogether.backToTheFilm")}
        </Link>
        {/* A Stop pressed while the connection is down would be dropped unsaid, so it waits. */}
        <button
          type="button"
          disabled={!isUp}
          onClick={() => {
            connection?.command({ command: "stop" });
          }}
        >
          {t("watchTogether.stopTheFilm")}
        </button>
        {status === "connecting" || status === "reconnecting" ? (
          <output>{t(`together.${status}`)}</output>
        ) : null}
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
