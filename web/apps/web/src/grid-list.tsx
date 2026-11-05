// SPDX-License-Identifier: MPL-2.0

import "./grid-list.css";

import type { CSSProperties, JSX, ReactNode } from "react";

import { arrivalMark } from "./focus-navigation.ts";
import { focusContainerAttribute } from "./focus-engine.ts";

/**
 * How a column's text is laid out: "wrap" wraps within the column's share of the width; "truncate"
 * stays on one line, cut with an ellipsis where its share is too narrow; "fit" stays on one line, the
 * column as wide as its widest cell.
 */
export type GridText = "wrap" | "truncate" | "fit";

/** One column of a grid list: how its text is laid out, and what each row draws in it. */
export interface GridColumn<Row> {
  readonly key: string;
  readonly text: GridText;
  readonly cell: (row: Row) => ReactNode;
}

/** What a row's one pressable target is given to draw: its class, and its cells inside it. */
export interface GridTargetDrawn {
  readonly className: string;
  readonly children: ReactNode;
}

/** Each column's track: a share of what is left for wrap and truncate, its widest cell for fit. */
const trackOf: Readonly<Record<GridText, string>> = {
  wrap: "minmax(0, 1fr)",
  truncate: "minmax(0, 1fr)",
  fit: "max-content",
};

/** Each text's cell class, written out whole, so a search for a stylesheet's class finds it. */
const cellClass: Readonly<Record<GridText, string>> = {
  wrap: "grid-list__cell grid-list__cell--wrap",
  truncate: "grid-list__cell grid-list__cell--truncate",
  fit: "grid-list__cell grid-list__cell--fit",
};

/** The list's column template: each column's track, then the aside's where rows have one. */
export function gridTemplate(texts: readonly GridText[], hasAside: boolean): string {
  return [...texts.map((text) => trackOf[text]), ...(hasAside ? ["max-content"] : [])].join(" ");
}

/**
 * A list whose rows share one set of columns (the owner, 2026-11-05: "a grid UI widget for
 * alignment, which supports some columns wrapping and some not"): every row's cells line up under
 * one another, each column's text wraps or stays on one line as the column declares, and each row
 * is one pressable target, the caller's Link or button drawn by target, covering exactly its cells.
 * aside draws what sits beside a row's target and is not part of it (a role's warning), in a column
 * of its own. isAcross flows the rows several to a line, each as wide as the others, for a list of
 * short entries (a title's cast); there a "fit" column's cells must be one size, as each row lays
 * out its own columns. isArrival marks the list as its page's arrival; isFocusContainer as a place
 * the focus engine walks within first.
 */
export function GridList<Row>(props: {
  readonly rows: readonly Row[];
  readonly rowKey: (row: Row, index: number) => string;
  readonly columns: readonly GridColumn<Row>[];
  readonly target: (row: Row, drawn: GridTargetDrawn) => JSX.Element;
  readonly aside?: (row: Row) => ReactNode;
  readonly isAcross?: boolean;
  readonly isArrival?: boolean;
  readonly isFocusContainer?: boolean;
}): JSX.Element {
  const { columns, target, aside } = props;
  return (
    <GridFrame
      texts={columns.map((column) => column.text)}
      hasAside={aside !== undefined}
      isAcross={props.isAcross}
      isArrival={props.isArrival}
      isFocusContainer={props.isFocusContainer}
    >
      {props.rows.map((row, index) => (
        <GridRow key={props.rowKey(row, index)}>
          {target(row, {
            className: "grid-list__target",
            children: columns.map((column) => (
              <GridCell key={column.key} text={column.text}>
                {column.cell(row)}
              </GridCell>
            )),
          })}
          {aside === undefined ? null : <span className="grid-list__aside">{aside(row)}</span>}
        </GridRow>
      ))}
    </GridFrame>
  );
}

/**
 * A grid list's frame, for rows that draw themselves through a component of their own, as one of
 * several controls whose cells keep state (a waiting item's Choose and its chooser): texts declares
 * each column, as GridList's columns do, and each row is a GridRow of a GridCell a column, each
 * holding its own controls, then any GridAfter. GridList draws its rows into it; the options are
 * GridList's.
 */
export function GridFrame(props: {
  readonly texts: readonly GridText[];
  readonly hasAside?: boolean | undefined;
  readonly isAcross?: boolean | undefined;
  readonly isArrival?: boolean | undefined;
  readonly isFocusContainer?: boolean | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const hasAside = props.hasAside === true;
  // The template, as the stylesheet reads it: React passes a custom property through as it is.
  const style: CSSProperties & { readonly "--grid-list-columns": string } = {
    "--grid-list-columns": gridTemplate(props.texts, hasAside),
  };
  const className = [
    "grid-list",
    props.isAcross === true ? "grid-list--across" : "",
    hasAside ? "grid-list--aside" : "",
  ]
    .filter((name) => name !== "")
    .join(" ");
  const marks = {
    ...(props.isArrival === true ? arrivalMark : {}),
    ...(props.isFocusContainer === true ? { [focusContainerAttribute]: "" } : {}),
  };
  return (
    <ul className={className} style={style} {...marks}>
      {props.children}
    </ul>
  );
}

/** One row of a grid list: its cells, in the frame's columns. */
export function GridRow(props: { readonly children: ReactNode }): JSX.Element {
  return <li className="grid-list__row">{props.children}</li>;
}

/** One cell of a grid list's row, its text laid out as its column declares. */
export function GridCell(props: {
  readonly text: GridText;
  readonly children: ReactNode;
}): JSX.Element {
  return <span className={cellClass[props.text]}>{props.children}</span>;
}

/** What opens under a grid list's row, across every column. */
export function GridAfter(props: { readonly children: ReactNode }): JSX.Element {
  return <div className="grid-list__after">{props.children}</div>;
}
