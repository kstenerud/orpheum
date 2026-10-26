// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** A library, as the server sends it. */
type LibraryBody = components["schemas"]["LibraryBody"];

/**
 * The person's own choice of which libraries Films and Series gather (D349 §8, the owner:
 * *"The user can exclude a library from that aggregated view"*): every `movies`, `shows` or
 * `mixed` library the person reaches is in those pages, until they leave it out here; a library
 * left out still has its own place on the Libraries page. Kept on their account
 * (`leftOutLibraryIds`, `GET`/`PUT /me/preferences`); nothing shown where they reach no such
 * library, since there is then nothing to choose between.
 */
export function LeftOutLibrariesSetting(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries] = useResource(
    "account-libraries",
    async () => dataOrThrow(await api.GET("/libraries")).libraries ?? [],
  );
  const [preferences, reload] = useResource("my-preferences-left-out", async () =>
    dataOrThrow(await api.GET("/me/preferences")),
  );
  const [problem, setProblem] = useState<string | undefined>(undefined);
  if (libraries.state !== "loaded" || preferences.state !== "loaded") {
    return null;
  }
  const aggregated = libraries.value.filter((library) => library.kind !== "other");
  if (aggregated.length === 0) {
    return null;
  }
  const leftOut = new Set(preferences.value.leftOutLibraryIds ?? []);
  const setIncluded = (libraryId: number, isIncluded: boolean): void => {
    const next = new Set(leftOut);
    if (isIncluded) {
      next.delete(libraryId);
    } else {
      next.add(libraryId);
    }
    setProblem(undefined);
    api
      .PUT("/me/preferences", { body: { leftOutLibraryIds: [...next] } })
      .then(throwIfRefused)
      .then(reload, (cause: unknown) => {
        setProblem(problemText(t, cause));
      });
  };
  return (
    <section aria-labelledby="left-out-libraries-heading">
      <h2 id="left-out-libraries-heading">{t("account.librariesHeading")}</h2>
      <p>{t("account.librariesText")}</p>
      <ul>
        {aggregated.map((library) => (
          <LibraryChoice
            key={library.id}
            library={library}
            isIncluded={!leftOut.has(library.id)}
            onChange={setIncluded}
          />
        ))}
      </ul>
      {problem === undefined ? null : <output>{problem}</output>}
    </section>
  );
}

/** One library's checkbox: checked while it is gathered into Films and Series. */
function LibraryChoice(props: {
  readonly library: LibraryBody;
  readonly isIncluded: boolean;
  readonly onChange: (libraryId: number, isIncluded: boolean) => void;
}): JSX.Element {
  const { library } = props;
  return (
    <li>
      <label>
        <input
          type="checkbox"
          checked={props.isIncluded}
          onChange={(event) => {
            props.onChange(library.id, event.target.checked);
          }}
        />
        {library.name}
      </label>
    </li>
  );
}
