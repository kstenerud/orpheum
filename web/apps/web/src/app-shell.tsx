// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { productName } from "./product-name.ts";
import type { Route } from "./router.ts";

/** A place the menu leads to, and whether this person may go there. */
interface Destination {
  readonly route: Route;
  readonly label:
    | "films"
    | "series"
    | "search"
    | "identify"
    | "libraries"
    | "people"
    | "settings"
    | "account"
    | "about";
  readonly isAdminOnly: boolean;
}

const destinations: readonly Destination[] = [
  { route: { screen: "films" }, label: "films", isAdminOnly: false },
  { route: { screen: "series" }, label: "series", isAdminOnly: false },
  { route: { screen: "search", query: "" }, label: "search", isAdminOnly: false },
  { route: { screen: "identify" }, label: "identify", isAdminOnly: true },
  { route: { screen: "libraries" }, label: "libraries", isAdminOnly: true },
  { route: { screen: "people" }, label: "people", isAdminOnly: true },
  { route: { screen: "settings" }, label: "settings", isAdminOnly: true },
  { route: { screen: "account" }, label: "account", isAdminOnly: false },
  { route: { screen: "about" }, label: "about", isAdminOnly: false },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The admin's destinations show to an admin only; the server
 * refuses them to anyone else whatever the menu shows.
 */
export function AppShell(props: {
  readonly isAdmin: boolean;
  readonly current: Route["screen"];
  readonly onSignOut: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="app-shell">
      <header className="app-shell__banner">
        <p className="app-shell__title">{productName}</p>
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <ul>
            {destinations
              .filter((destination) => props.isAdmin || !destination.isAdminOnly)
              .map((destination) => (
                <li
                  key={destination.label}
                  aria-current={destination.route.screen === props.current ? "page" : undefined}
                >
                  <Link to={destination.route}>{t(`nav.${destination.label}`)}</Link>
                </li>
              ))}
          </ul>
          <button type="button" onClick={props.onSignOut}>
            {t("nav.signOut")}
          </button>
        </nav>
      </header>
      <main className="app-shell__main">{props.children}</main>
    </div>
  );
}
