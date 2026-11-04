// SPDX-License-Identifier: MPL-2.0

import "./about.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import tmdbLogo from "./assets/tmdb-logo.svg";
import { productName } from "./product-name.ts";

/** A project we are built on, by its own name, which is never translated. */
interface Acknowledgement {
  readonly name: string;
  readonly url: string;
}

/**
 * The major projects we use, whether or not their licences ask to be named (D213 §7). The
 * licence notices a binary ships with join this page when releases are built.
 */
export const acknowledgements: readonly Acknowledgement[] = [
  { name: "FFmpeg", url: "https://ffmpeg.org/" },
  { name: "libwebp", url: "https://chromium.googlesource.com/webm/libwebp" },
  { name: "SQLite", url: "https://sqlite.org/" },
  { name: "Go", url: "https://go.dev/" },
  { name: "modernc.org/sqlite", url: "https://gitlab.com/cznic/sqlite" },
  { name: "sqlc", url: "https://sqlc.dev/" },
  { name: "Huma", url: "https://huma.rocks/" },
  { name: "React", url: "https://react.dev/" },
  { name: "Vite", url: "https://vite.dev/" },
  { name: "TypeScript", url: "https://www.typescriptlang.org/" },
  { name: "i18next", url: "https://www.i18next.com/" },
  { name: "openapi-typescript", url: "https://openapi-ts.dev/" },
  { name: "The Movie Database (TMDB)", url: "https://www.themoviedb.org/" },
];

/**
 * About: TMDB's attribution — its logo, smaller than ours, and its notice, as its terms (§3) and
 * FAQ ask, in an About section (D213 §7) — and the projects we are built on. Its heading is the
 * page's h1, as every page's is (ui-audit-2026-11-04.md §3, About).
 */
export function About(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="about" aria-labelledby="about-heading">
      <h1 id="about-heading">{t("about.heading")}</h1>
      <h2>{t("about.tmdbHeading")}</h2>
      <img className="about__tmdb-logo" src={tmdbLogo} alt={t("about.tmdbLogo")} />
      <p>{t("about.tmdbNotice")}</p>
      <h2>{t("about.thanksHeading")}</h2>
      <p>{t("about.thanksIntro", { product: productName })}</p>
      <ul className="about__thanks">
        {acknowledgements.map((project) => (
          <li key={project.name}>
            <a href={project.url} rel="noreferrer noopener" target="_blank">
              {project.name}
            </a>
          </li>
        ))}
      </ul>
    </section>
  );
}
