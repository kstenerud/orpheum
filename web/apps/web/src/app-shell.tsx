// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import { useEffect, useRef, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import logo from "./assets/orpheum-logo.svg";
import { Link } from "./link.tsx";
import { productName } from "./product-name.ts";
import type { Route } from "./router.ts";

/** A place the menu leads to, and whether this person may go there. */
interface Destination {
  readonly route: Route;
  /** The key of the destination's name in the catalogue, under nav. */
  readonly labelKey:
    | "films"
    | "series"
    | "search"
    | "watchTogether"
    | "identify"
    | "libraries"
    | "people"
    | "settings"
    | "jobs"
    | "system"
    | "account"
    | "about";
  /** Who it shows to: everyone, an admin, or a person whose role may host Watch Together. */
  readonly audience: "everyone" | "admin" | "host";
  /**
   * Where in the banner it is: the places for watching, always in view; the admin's area, under
   * Manage; or the person's own, under their name (ui-design-practice-2026-10-05.md §5.1 item 5).
   */
  readonly group: "watching" | "manage" | "own";
}

const destinations: readonly Destination[] = [
  { route: { screen: "films" }, labelKey: "films", audience: "everyone", group: "watching" },
  { route: { screen: "series" }, labelKey: "series", audience: "everyone", group: "watching" },
  {
    route: { screen: "search", query: "" },
    labelKey: "search",
    audience: "everyone",
    group: "watching",
  },
  {
    route: { screen: "watch-together" },
    labelKey: "watchTogether",
    audience: "host",
    group: "watching",
  },
  { route: { screen: "identify" }, labelKey: "identify", audience: "admin", group: "manage" },
  { route: { screen: "libraries" }, labelKey: "libraries", audience: "admin", group: "manage" },
  { route: { screen: "people" }, labelKey: "people", audience: "admin", group: "manage" },
  { route: { screen: "settings" }, labelKey: "settings", audience: "admin", group: "manage" },
  { route: { screen: "jobs" }, labelKey: "jobs", audience: "admin", group: "manage" },
  { route: { screen: "system" }, labelKey: "system", audience: "admin", group: "manage" },
  { route: { screen: "account" }, labelKey: "account", audience: "everyone", group: "own" },
  { route: { screen: "about" }, labelKey: "about", audience: "everyone", group: "own" },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The places for watching are always in view; the admin's
 * destinations are under Manage, shown to an admin only; the person's account, About and Sign out
 * are under their name. Watch Together shows to a person whose role may host. The server refuses
 * a destination to anyone else whatever the menu shows.
 */
export function AppShell(props: {
  readonly isAdmin: boolean;
  /** Whether the person's role may host Watch Together, which shows them its screen. */
  readonly canHostWatchTogether: boolean;
  /**
   * Whether this tab hosts a Watch Together session now: Watch Together is marked as on, in view
   * on every screen, since while it is on what the host plays plays for everyone.
   */
  readonly isHostingTogether?: boolean;
  readonly currentScreen: Route["screen"];
  readonly onSignOut: () => void;
  /** Who is signed in, and on which device, once known: said in the banner. */
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = destinations.filter(
    (destination) =>
      destination.audience === "everyone" ||
      (destination.audience === "admin" && props.isAdmin) ||
      (destination.audience === "host" && props.canHostWatchTogether),
  );
  const inGroup = (group: Destination["group"]): readonly Destination[] =>
    shown.filter((destination) => destination.group === group);
  const links = (group: Destination["group"]): JSX.Element[] =>
    inGroup(group).map((destination) => (
      <Place
        key={destination.labelKey}
        destination={destination}
        isCurrent={destination.route.screen === props.currentScreen}
        isOn={destination.labelKey === "watchTogether" && props.isHostingTogether === true}
      />
    ));
  const isIn = (group: Destination["group"]): boolean =>
    inGroup(group).some((destination) => destination.route.screen === props.currentScreen);
  return (
    <div className="app-shell">
      <header className="app-shell__banner">
        <p className="app-shell__title">
          <img className="app-shell__logo" src={logo} alt={productName} />
        </p>
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <ul className="app-shell__watching">{links("watching")}</ul>
          {inGroup("manage").length === 0 ? null : (
            <BannerMenu label={t("nav.manage")} isCurrent={isIn("manage")}>
              {links("manage")}
            </BannerMenu>
          )}
          <OwnMenu
            signedInAs={props.signedInAs}
            isCurrent={isIn("own")}
            onSignOut={props.onSignOut}
          >
            {links("own")}
          </OwnMenu>
        </nav>
      </header>
      <main className="app-shell__main">{props.children}</main>
    </div>
  );
}

/** One place in the menu, marked when it is the current screen, and when it is on. */
function Place(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
  /** Whether it is on now: Watch Together, while this tab hosts a session. */
  readonly isOn: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { destination } = props;
  return (
    <li aria-current={props.isCurrent ? "page" : undefined}>
      <Link to={destination.route}>
        {t(`nav.${destination.labelKey}`)}
        {props.isOn ? <span className="app-shell__on"> {t("nav.togetherOn")}</span> : null}
      </Link>
    </li>
  );
}

/** The person's own menu, under their name: who is signed in where, their places, and Sign out. */
function OwnMenu(props: {
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  readonly isCurrent: boolean;
  readonly onSignOut: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const { signedInAs } = props;
  return (
    <BannerMenu label={signedInAs?.name ?? t("nav.you")} isCurrent={props.isCurrent} modifier="own">
      {signedInAs === undefined ? null : (
        <li className="app-shell__who">
          {signedInAs.deviceName === undefined
            ? t("nav.signedInAs", { name: signedInAs.name })
            : t("nav.signedInAsOn", { name: signedInAs.name, device: signedInAs.deviceName })}
        </li>
      )}
      {props.children}
      <li>
        <button type="button" onClick={props.onSignOut}>
          {t("nav.signOut")}
        </button>
      </li>
    </BannerMenu>
  );
}

/**
 * Whether event closes the open menu: a press outside it, Escape, or a click on one of its
 * places. A press on its own name is the menu's to toggle.
 */
function closesMenu(event: Event, menu: HTMLDetailsElement): boolean {
  const target = event.target instanceof Element ? event.target : null;
  const isInside = target !== null && menu.contains(target);
  if (event instanceof KeyboardEvent) {
    return event.key === "Escape";
  }
  if (event.type === "pointerdown") {
    return !isInside;
  }
  return isInside && target.closest("a") !== null;
}

/**
 * A menu in the banner: its name, which opens a list below it. It is the page's own disclosure,
 * so it works without script and by keyboard; it closes when one of its places is chosen, when
 * the person presses or taps outside it, and on Escape.
 */
function BannerMenu(props: {
  readonly label: string;
  /** Whether the current screen is one of its places: its name is then marked as where one is. */
  readonly isCurrent: boolean;
  readonly modifier?: "own";
  readonly children: ReactNode;
}): JSX.Element {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const close = (event: Event): void => {
      const menu = ref.current;
      if (menu?.open === true && closesMenu(event, menu)) {
        menu.open = false;
      }
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("click", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", close);
    };
  }, []);
  const classes = ["app-shell__menu"];
  if (props.modifier !== undefined) {
    classes.push(`app-shell__menu--${props.modifier}`);
  }
  if (props.isCurrent) {
    classes.push("app-shell__menu--current");
  }
  return (
    <details ref={ref} className={classes.join(" ")}>
      <summary>{props.label}</summary>
      <ul>{props.children}</ul>
    </details>
  );
}
