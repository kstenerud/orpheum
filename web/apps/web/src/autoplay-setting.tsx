// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useMe } from "./signed-in-me.ts";

/**
 * The person's own Playing settings on their account: whether the next episode plays by itself as
 * one ends, after Up next's countdown (plans/up-next.md), on until they turn it off. It shows the
 * person's choices as who is signed in carries them (GET /me's preferences, D480 §2), reading none
 * of its own; a change is kept on the server at once, who is signed in read again after it, and a
 * refusal said beside it.
 */
export function AutoplaySetting(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { me, readMeAgain } = useMe();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const preferences = me.state === "loaded" ? me.value.preferences : undefined;
  if (preferences === undefined) {
    return null;
  }
  return (
    <section aria-labelledby="playing-heading">
      <h2 id="playing-heading">{t("upNext.settingHeading")}</h2>
      <p>{t("upNext.autoplayText")}</p>
      <label>
        <input
          type="checkbox"
          checked={preferences.isAutoplayOn}
          onChange={(event) => {
            setProblem(undefined);
            const isAutoplayOn = event.target.checked;
            api
              .PUT("/me/preferences", { body: { isAutoplayOn } })
              .then(throwIfRefused)
              .then(readMeAgain, (cause: unknown) => {
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
