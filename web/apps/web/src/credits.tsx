// SPDX-License-Identifier: MPL-2.0

import "./credits.css";
import "./credits-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { GridList, type GridColumn } from "./grid-list.tsx";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow } from "./problem.ts";
import { itemRoute } from "./router.ts";
import { useServices } from "./services.tsx";
import { loadAgain } from "./use-pages.ts";
import { useResource } from "./use-resource.ts";

/** How many of a person's titles one request asks for: the most GET /items gives. */
const creditedPageSize = 500;

/** A title's cast, directors and writers, as the server sends them, each in billing order. */
export type Credits = components["schemas"]["CreditsBody"];
type Credit = components["schemas"]["CreditBody"];
type PersonItem =
  components["schemas"]["TitleEntryBody"] | components["schemas"]["SeriesEntryBody"];
type PersonCredit = components["schemas"]["ItemCreditBody"];

/** A name's initials: its first word's first letter and its last's, as a face's placeholder shows. */
export function initialsOf(name: string): string {
  const words = name.split(/\s+/u).filter((word) => word !== "");
  const ends = words.length > 1 ? [words[0], words.at(-1)] : words;
  return ends
    .map((word) => firstLetterOf(word ?? ""))
    .join("")
    .toLocaleUpperCase();
}

/** A word's first letter as a reader sees one, an accent or a joined emoji with it. */
function firstLetterOf(word: string): string {
  const segments = new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(word);
  return segments.containing(0)?.segment ?? "";
}

/**
 * A person's face's place: their initials in a circle. The server keeps where their picture is at
 * its source (pictureRef) but serves no picture yet, so none is shown; hidden from a screen reader,
 * which hears the name beside it.
 */
export function PersonFace(props: { readonly name: string }): JSX.Element {
  return (
    <span className="person-face" aria-hidden="true">
      {initialsOf(props.name)}
    </span>
  );
}

/**
 * A credit's columns: their face's place, then their name and, in the cast, their part under it,
 * wrapping where a name is long.
 */
const creditColumns: readonly GridColumn<Credit>[] = [
  { key: "face", text: "fit", cell: (credit) => <PersonFace name={credit.name} /> },
  {
    key: "words",
    text: "wrap",
    cell: (credit) => (
      <span className="credit__words">
        <span className="credit__name">{credit.name}</span>
        {credit.character === undefined || credit.character === "" ? null : (
          <span className="credit__character">{credit.character}</span>
        )}
      </span>
    ),
  },
];

/** One group of a title's credits under its heading, each leading to the person's page; none when empty. */
function CreditGroup(props: {
  readonly id: string;
  readonly heading: string;
  readonly credits: readonly Credit[] | null | undefined;
}): JSX.Element | null {
  const credits = props.credits ?? [];
  if (credits.length === 0) {
    return null;
  }
  return (
    <section className="credits__group" aria-labelledby={props.id}>
      <h2 id={props.id}>{props.heading}</h2>
      <GridList
        rows={credits}
        // A person may be credited twice in one group (two parts); their place keeps each apart.
        rowKey={(credit, index) => `${String(credit.personId)}/${String(index)}`}
        columns={creditColumns}
        target={(credit, drawn) => (
          <Link
            to={{ screen: "person", personId: credit.personId }}
            className={`${drawn.className} credit`}
          >
            {drawn.children}
          </Link>
        )}
        isAcross
        isFocusContainer
      />
    </section>
  );
}

/**
 * A title's people (search-by-more §4.4): its cast in billing order, each one's part under their
 * name, then its directors and writers, each one press from their page and its titles. An episode's
 * are its series' (the server's). Nothing when the title credits no one.
 */
export function TitlePeople(props: { readonly credits: Credits | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const { credits } = props;
  if (credits === undefined) {
    return null;
  }
  return (
    <div className="credits">
      <CreditGroup id="credits-cast" heading={t("credits.cast")} credits={credits.cast} />
      <CreditGroup
        id="credits-directors"
        heading={t("credits.directors")}
        credits={credits.directors}
      />
      <CreditGroup id="credits-writers" heading={t("credits.writers")} credits={credits.writers} />
    </div>
  );
}

/** What a person did on a title, in words: "Director", "Writer", or "as" their part in the cast. */
function jobText(t: TFunction, job: string, character: string | undefined): string {
  switch (job) {
    case "director":
      return t("credits.roleDirector");
    case "writer":
      return t("credits.roleWriter");
    default:
      return character === undefined || character === ""
        ? t("credits.roleCast")
        : t("credits.roleAs", { character });
  }
}

/** What a person did on a title, each job in words, cast first as the server orders them. */
function creditText(t: TFunction, credit: PersonCredit | undefined): string {
  return (credit?.jobs ?? []).map((job) => jobText(t, job, credit?.character)).join(", ");
}

/** One title on a person's page: its name, its year, and what they did on it, leading to its page. */
function PersonTitleLink(props: { readonly title: PersonItem }): JSX.Element {
  const { t } = useTranslation();
  const { title } = props;
  const jobs = creditText(t, title.credit);
  return (
    <Link to={itemRoute(title.kind, title.id)} className="credited-person__title">
      <span className="credited-person__title-name">{title.name}</span>
      {title.year === undefined ? null : (
        <span className="credited-person__title-detail">
          {t("browse.year", { year: title.year })}
        </span>
      )}
      {jobs === "" ? null : <span className="credited-person__title-detail">{jobs}</span>}
    </Link>
  );
}

/**
 * A credited person's page (search-by-more §4.4): their name, and the titles they reach that credit
 * them, newest first as the server orders them, each opening its page. One no title the person
 * reaches credits is not found (the server's 404), told as any page's failure is.
 */
export function CreditedPerson(props: { readonly id: number }): JSX.Element {
  const { api } = useServices();
  const [person] = useResource(`person/${String(props.id)}`, async () => {
    const named = dataOrThrow(
      await api.GET("/people/{id}", { params: { path: { id: props.id } } }),
    );
    // Every title crediting them, page after page: the page lists them all (D425 §7).
    const pages = await loadAgain(async (cursor) => {
      const query = {
        person: props.id,
        sort: "released" as const,
        limit: creditedPageSize,
        ...(cursor === "" ? {} : { cursor }),
      };
      const page = dataOrThrow(await api.GET("/items", { params: { query } }));
      return { items: page.items ?? [], nextCursor: page.nextCursor ?? "" };
    }, Number.POSITIVE_INFINITY);
    return { ...named, titles: pages.flatMap((page) => page.items) };
  });
  return (
    <Loaded
      resource={person}
      render={(loaded) => (
        <section className="credited-person" aria-labelledby="credited-person-heading">
          <div className="credited-person__head">
            <PersonFace name={loaded.name} />
            <h1 id="credited-person-heading">{loaded.name}</h1>
          </div>
          <ul className="credited-person__titles" data-focus-container="" {...arrivalMark}>
            {loaded.titles.map((title) => (
              <li key={`${title.kind}/${String(title.id)}`}>
                <PersonTitleLink title={title} />
              </li>
            ))}
          </ul>
        </section>
      )}
    />
  );
}
