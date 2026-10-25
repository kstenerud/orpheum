// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The person's own Playing settings on their account: whether the next episode plays by itself as
 * one ends, after Up next's countdown (plans/up-next.md), on until they turn it off. A change is kept
 * on the server at once, and a refusal said beside it.
 */
export function AutoplaySetting(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [preferences, reload] = useResource("my-preferences", async () =>
    dataOrThrow(await api.GET("/me/preferences")),
  );
  const [problem, setProblem] = useState<string | undefined>(undefined);
  if (preferences.state !== "loaded") {
    return null;
  }
  return (
    <section aria-labelledby="playing-heading">
      <h2 id="playing-heading">{t("upNext.settingHeading")}</h2>
      <p>{t("upNext.autoplayText")}</p>
      <label>
        <input
          type="checkbox"
          checked={preferences.value.isAutoplayOn}
          onChange={(event) => {
            setProblem(undefined);
            const isAutoplayOn = event.target.checked;
            api
              .PUT("/me/preferences", { body: { isAutoplayOn } })
              .then(throwIfRefused)
              .then(reload, (cause: unknown) => {
                setProblem(problemText(t, cause));
              });
          }}
        />{" "}
        {t("upNext.autoplay")}
      </label>
      {problem === undefined ? null : <output>{problem}</output>}
    </section>
  );
}
