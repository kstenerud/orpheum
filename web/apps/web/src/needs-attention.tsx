// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { Link } from "./link.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";

/**
 * Everything that needs the admin, in one place, which the status tray's warning sign leads to
 * (D280). A placeholder until it is designed: it counts what needs them and leads to the pages that
 * have each, the System page, the Libraries and Needs a match.
 */
export function NeedsAttention(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [counts] = useResource("needs-attention", async () =>
    dataOrThrow(await api.GET("/needs-attention")),
  );
  return (
    <section aria-labelledby="needs-attention-heading">
      <h1 id="needs-attention-heading">{t("needsAttention.heading")}</h1>
      <p>{t("needsAttention.placeholder")}</p>
      <Loaded
        resource={counts}
        render={(value) => {
          const lines = [
            { count: value.rootCount, key: "needsAttention.offlineFolders" },
            { count: value.diskCount, key: "needsAttention.missingDisks" },
            { count: value.waitingRootCount, key: "needsAttention.waitingFolders" },
            { count: value.unreadCount, key: "needsAttention.unreadPaths" },
            { count: value.matchCount, key: "needsAttention.waitingMatches" },
          ].filter((line) => line.count > 0);
          const readWrongly = value.seriesReadWrongly ?? [];
          const withoutSound = value.filmsWithoutSound ?? [];
          return lines.length + readWrongly.length + withoutSound.length === 0 ? (
            <p>{t("needsAttention.nothing")}</p>
          ) : (
            <ul {...arrivalMark}>
              {lines.map((line) => (
                <li key={line.key}>
                  <WarningIcon /> {t(line.key, { count: line.count })}
                </li>
              ))}
              {readWrongly.map((series) => (
                <li key={`series-${String(series.seriesId)}`}>
                  <WarningIcon />{" "}
                  <Link to={{ screen: "series-page", collectionId: series.seriesId }}>
                    {series.name}
                  </Link>
                  {t("needsAttention.seriesReadWrongly", { count: series.unplacedCount })}
                </li>
              ))}
              {withoutSound.map((film) => (
                <li key={`film-${String(film.titleId)}`}>
                  <WarningIcon />{" "}
                  <Link to={{ screen: "title", itemId: film.titleId }}>{film.name}</Link>
                  {t("needsAttention.filmWithoutSound")}
                </li>
              ))}
            </ul>
          );
        }}
      />
      <p>
        <Link to={{ screen: "system" }}>{t("needsAttention.toSystem")}</Link> ·{" "}
        <Link to={{ screen: "libraries" }}>{t("needsAttention.toLibraries")}</Link> ·{" "}
        <Link to={{ screen: "identify" }}>{t("needsAttention.toIdentify")}</Link>
      </p>
    </section>
  );
}
