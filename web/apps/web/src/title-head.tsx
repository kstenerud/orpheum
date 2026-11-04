// SPDX-License-Identifier: MPL-2.0

import "./title-head.css";

import { useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { WideArt } from "./poster.tsx";

/**
 * The first screen of a film's, an episode's and a series' page, one template (ui-audit-2026-11-04.md
 * R8): the backdrop behind it, not a band above it; the art; then, in reading order, the line above
 * the name (an episode's series and season), the name, one line of facts, the status notes, the
 * actions row, its main action first and the page's arrival, so a remote's first OK plays (R1, R3),
 * and what it is about, clamped with More. What else the page has follows it. The actions row is a
 * focus container, a row as a poster row is: Right at its end goes nowhere (Q3), where it fell back
 * off its line to whatever lay to the right further down the page (a series' season's Mark as
 * watched, half a screen below). A title with nothing to play has no main action, and arrives on its
 * "…" instead, so an OK opens a menu and never changes anything (D418 §7): not on Mark as watched,
 * the row's first then. Where it has no "…" either, the head is no arrival.
 */
export function TitleHead(props: {
  readonly backdropSha256: string | undefined;
  /** The poster, or an episode's still. */
  readonly art: ReactNode;
  /** The line above the name: an episode's series and season, each a link up. */
  readonly above?: ReactNode;
  readonly name: string;
  /** The facts, each a span: the year, the length, the sound. */
  readonly facts: ReactNode;
  readonly notes?: ReactNode;
  /** The actions row: Play or Resume first, where there is one, then the rest. */
  readonly actions: ReactNode;
  /** Whether the actions row starts with something that plays the title: the page's arrival. */
  readonly hasMainAction: boolean;
  /** The "…" of what is rarely used, the actions row's last. */
  readonly more: ReactNode;
  readonly tagline?: string | undefined;
  readonly overview?: string | undefined;
}): JSX.Element {
  return (
    <div className="title-head">
      <WideArt
        sourceSha256={props.backdropSha256}
        size="landscape-l"
        className="title-head__backdrop"
      />
      <div className="title-head__body">
        {props.art}
        <div className="title-head__words">
          {props.above}
          <h1>{props.name}</h1>
          <p className="title-head__facts">{props.facts}</p>
          {props.notes}
          <div
            className="page-actions title-head__actions"
            data-focus-container=""
            {...(props.hasMainAction ? arrivalMark : {})}
          >
            {props.actions}
            <span className="title-head__overflow" {...(props.hasMainAction ? {} : arrivalMark)}>
              {props.more}
            </span>
          </div>
          {props.tagline === undefined ? null : (
            <p className="title-head__tagline">{props.tagline}</p>
          )}
          {props.overview === undefined ? null : <Synopsis text={props.overview} />}
        </div>
      </div>
    </div>
  );
}

/**
 * What a title is about, clamped to a few lines with More, which shows the rest, and Less: offered
 * only where the text runs past the clamp, measured as laid out (ui-audit-2026-11-04.md §3).
 */
export function Synopsis(props: { readonly text: string }): JSX.Element {
  const { t } = useTranslation();
  const ref = useRef<HTMLParagraphElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isClamped, setIsClamped] = useState(false);
  // Measured as laid out, again as its width changes; its first measure comes as it is observed.
  useEffect(() => {
    const text = ref.current;
    if (text === null || isOpen || typeof ResizeObserver === "undefined") {
      return undefined;
    }
    const resizes = new ResizeObserver(() => {
      setIsClamped(text.scrollHeight > text.clientHeight + 1);
    });
    resizes.observe(text);
    return () => {
      resizes.disconnect();
    };
  }, [props.text, isOpen]);
  return (
    <div className="title-head__synopsis">
      <p ref={ref} id="title-synopsis" className={isOpen ? undefined : "title-head__clamped"}>
        {props.text}
      </p>
      {isClamped || isOpen ? (
        <button
          type="button"
          className="title-head__more"
          aria-expanded={isOpen}
          aria-controls="title-synopsis"
          onClick={() => {
            setIsOpen(!isOpen);
          }}
        >
          {isOpen ? t("title.synopsisLess") : t("title.synopsisMore")}
        </button>
      ) : null}
    </div>
  );
}
