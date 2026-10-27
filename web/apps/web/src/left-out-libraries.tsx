// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** A library, as the server sends it. */
type LibraryBody = components["schemas"]["LibraryBody"];

/** A person's own choices, as the server sends them. */
type MyPreferencesBody = components["schemas"]["MyPreferencesBody"];

/**
 * The person's own choice of which libraries Films and Series gather (D349 §8, the owner:
 * *"The user can exclude a library from that aggregated view"*): every `movies` or `shows`
 * library the person reaches is in those pages, until they leave it out here; a library
 * left out still has its own place on the Libraries page. Kept on their account
 * (`leftOutLibraryIds`, `GET`/`PUT /me/preferences`); nothing shown where they reach no such
 * library, since there is then nothing to choose between.
 */
export function LeftOutLibrariesSetting(): JSX.Element | null {
  const { t } = useTranslation();
  return (
    <LeaveOutSetting
      field="leftOutLibraryIds"
      headingId="left-out-libraries-heading"
      heading={t("account.librariesHeading")}
      text={t("account.librariesText")}
      isOffered={(library) => library.kind !== "other"}
    />
  );
}

/**
 * The person's own choice of which libraries fill their Home (D366 §5, the owner: *"whether a
 * library's items show up on the home page … It's a choice each person makes"*): every library they
 * reach is on it, until they leave it out here; one left out adds nothing to any row of Home and
 * stays in Films, Series and the Libraries page. Kept on their account (`leftOutOfHomeLibraryIds`).
 */
export function LeftOutOfHomeSetting(): JSX.Element | null {
  const { t } = useTranslation();
  return (
    <LeaveOutSetting
      field="leftOutOfHomeLibraryIds"
      headingId="left-out-of-home-heading"
      heading={t("account.homeLibrariesHeading")}
      text={t("account.homeLibrariesText")}
      isOffered={() => true}
    />
  );
}

/**
 * One choice of libraries to leave out of a place: a checkbox for each library offered, checked
 * while it is in, kept in the person's own field of `/me/preferences`.
 */
function LeaveOutSetting(props: {
  readonly field: "leftOutLibraryIds" | "leftOutOfHomeLibraryIds";
  readonly headingId: string;
  readonly heading: string;
  readonly text: string;
  readonly isOffered: (library: LibraryBody) => boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries] = useResource(
    "account-libraries",
    async () => dataOrThrow(await api.GET("/libraries")).libraries ?? [],
  );
  const [preferences, reload] = useResource(`my-preferences-${props.field}`, async () =>
    dataOrThrow(await api.GET("/me/preferences")),
  );
  const [problem, setProblem] = useState<string | undefined>(undefined);
  if (libraries.state !== "loaded" || preferences.state !== "loaded") {
    return null;
  }
  const offered = libraries.value.filter(props.isOffered);
  if (offered.length === 0) {
    return null;
  }
  const leftOut = new Set(leftOutOf(preferences.value, props.field));
  const setIncluded = (libraryId: number, isIncluded: boolean): void => {
    const next = new Set(leftOut);
    if (isIncluded) {
      next.delete(libraryId);
    } else {
      next.add(libraryId);
    }
    setProblem(undefined);
    api
      .PUT("/me/preferences", { body: { [props.field]: [...next] } })
      .then(throwIfRefused)
      .then(reload, (cause: unknown) => {
        setProblem(problemText(t, cause));
      });
  };
  return (
    <section aria-labelledby={props.headingId}>
      <h2 id={props.headingId}>{props.heading}</h2>
      <p>{props.text}</p>
      <ul>
        {offered.map((library) => (
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

/** The libraries a person's choices leave out of the place field names. */
function leftOutOf(
  preferences: MyPreferencesBody,
  field: "leftOutLibraryIds" | "leftOutOfHomeLibraryIds",
): readonly number[] {
  return (
    (field === "leftOutLibraryIds"
      ? preferences.leftOutLibraryIds
      : preferences.leftOutOfHomeLibraryIds) ?? []
  );
}

/** One library's checkbox: checked while it is in. */
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
