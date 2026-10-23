// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import { useEffect, useRef, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import logo from "./assets/orpheum-logo.svg";
import { StatusTray } from "./status-tray.tsx";
import { Link } from "./link.tsx";
import { LocalNetworkNote } from "./local-network-note.tsx";
import { PlainHttpNote } from "./plain-http-note.tsx";
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
    | "thisDevice"
    | "account"
    | "about"
    | "uiGallery";
  /**
   * Who it shows to: everyone, an admin, a person whose role may host Watch Together, or anyone
   * while the admin has the pages for development turned on.
   */
  readonly audience: "everyone" | "admin" | "host" | "developer";
  /**
   * Where in the banner it is: the places for watching, always in view; or, in the one menu at the
   * banner's end, the admin's area under Manage, the pages for development, or the person's own
   * (the owner, 2026-10-11 and 2026-10-17).
   */
  readonly group: "watching" | "manage" | "develop" | "own";
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
  {
    route: { screen: "ui-gallery" },
    labelKey: "uiGallery",
    audience: "developer",
    group: "develop",
  },
  { route: { screen: "this-device" }, labelKey: "thisDevice", audience: "everyone", group: "own" },
  { route: { screen: "account" }, labelKey: "account", audience: "everyone", group: "own" },
  { route: { screen: "about" }, labelKey: "about", audience: "everyone", group: "own" },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The places for watching are always in view; everything else is
 * in one menu behind a menu icon at the banner's end (the owner, 2026-10-11: "consolidate all the
 * extra functionality into a single pull-down menu on the right"): who is signed in, the admin's
 * destinations under Manage, shown to an admin only, the person's own places, and Sign out. Watch Together shows to a person whose role may host. The server refuses
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
  /** The realm the person signed in to the server by, when they did: shown in the banner (D297 §5). */
  readonly realm?: string | undefined;
  /**
   * Whether the menu offers the pages for development, such as the UI gallery: a server setting the
   * admin turns on (the owner, 2026-10-17: "development items behind a setting").
   */
  readonly showsDeveloperPages?: boolean;
  /**
   * Goes to the discovery's list of households, offered in the menu while a discovery's loader runs
   * the page (D308 §8); undefined, and the menu offers none.
   */
  readonly onSwitchHousehold?: (() => void) | undefined;
  /**
   * For an admin who does not administer here, away from home (D297 §6): the server's home address,
   * where they do, or "" when it has none ready; undefined for anyone else. Under Manage the menu
   * then says so, in place of places it cannot offer.
   */
  readonly administersFromHomeAt?: string | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = destinations.filter(
    (destination) =>
      destination.audience === "everyone" ||
      (destination.audience === "admin" && props.isAdmin) ||
      (destination.audience === "host" && props.canHostWatchTogether) ||
      (destination.audience === "developer" && props.showsDeveloperPages === true),
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
          {/* The logo is the way home, to the library's first shelf, as on most sites. */}
          <Link to={{ screen: "films" }} className="app-shell__home">
            <img className="app-shell__logo" src={logo} alt={productName} />
          </Link>
        </p>
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <ul className="app-shell__watching">{links("watching")}</ul>
          <BannerEnd shell={props} isMenuCurrent={isIn("manage") || isIn("develop") || isIn("own")}>
            <MenuSection
              heading={t("nav.manage")}
              places={managePlaces(props.administersFromHomeAt, links("manage"))}
            />
            <MenuSection heading={t("nav.develop")} places={links("develop")} />
            {links("own")}
          </BannerEnd>
        </nav>
      </header>
      <ShellMain>{props.children}</ShellMain>
    </div>
  );
}

/** The screen, under any word the page has about what the browser is doing. */
function ShellMain(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <main className="app-shell__main">
      <PlainHttpNote />
      <LocalNetworkNote />
      {props.children}
    </main>
  );
}

/**
 * The banner's end: the status tray (D280), then the one menu with its places, kept together so
 * the tray sits just before the menu at every width (the owner, 2026-10-11: on a narrow window it
 * had fallen into the row of places for watching).
 */
function BannerEnd(props: {
  readonly shell: Parameters<typeof AppShell>[0];
  readonly isMenuCurrent: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { shell } = props;
  return (
    <div className="app-shell__end">
      {shell.realm === undefined ? null : <RealmMark realm={shell.realm} />}
      <StatusTray isHostingTogether={shell.isHostingTogether === true} isAdmin={shell.isAdmin} />
      <MainMenu
        signedInAs={shell.signedInAs}
        isCurrent={props.isMenuCurrent}
        onSignOut={shell.onSignOut}
        onSwitchHousehold={shell.onSwitchHousehold}
      >
        {props.children}
      </MainMenu>
    </div>
  );
}

/** A titled part of the menu and the divider after it, or nothing when it has no place. */
function MenuSection(props: {
  readonly heading: string;
  readonly places: readonly JSX.Element[];
}): JSX.Element | null {
  if (props.places.length === 0) {
    return null;
  }
  return (
    <>
      <li className="app-shell__section">{props.heading}</li>
      {props.places}
      <li className="app-shell__divider" aria-hidden="true" />
    </>
  );
}

/** Manage's places: the admin's, or for an admin away from home where they administer instead. */
function managePlaces(
  administersFromHomeAt: string | undefined,
  places: JSX.Element[],
): JSX.Element[] {
  return administersFromHomeAt === undefined
    ? places
    : [<AdministerFromHome key="home" address={administersFromHomeAt} />];
}

/**
 * Where an admin away from home administers: at home, at the server's home address when it has one
 * ready (D297 §6, D292 §6).
 */
function AdministerFromHome(props: { readonly address: string }): JSX.Element {
  const { t } = useTranslation();
  return (
    <li className="app-shell__note">
      {props.address === "" ? (
        t("nav.administerFromHome")
      ) : (
        <>
          {t("nav.administerFromHomeAt")} <a href={props.address}>{props.address}</a>
        </>
      )}
    </li>
  );
}

/** The realm the person is signed in to, in view on every screen: which server this is (D297 §5). */
function RealmMark(props: { readonly realm: string }): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="app-shell__realm" title={t("nav.realmTitle", { realm: props.realm })}>
      {props.realm}
    </span>
  );
}

/** Three lines: the menu's mark, its name for a screen reader. */
export function MenuIcon(): JSX.Element {
  return (
    <svg className="app-shell__menu-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d="M3 6h18v2H3zm0 5h18v2H3zm0 5h18v2H3z" />
    </svg>
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

/**
 * The one menu, behind its icon: who is signed in where, the places it is given, and Sign out.
 */
function MainMenu(props: {
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  readonly isCurrent: boolean;
  readonly onSignOut: () => void;
  readonly onSwitchHousehold: (() => void) | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const { signedInAs } = props;
  return (
    <BannerMenu label={t("nav.menu")} isCurrent={props.isCurrent}>
      {signedInAs === undefined ? null : (
        <li className="app-shell__who">
          {signedInAs.deviceName === undefined
            ? t("nav.signedInAs", { name: signedInAs.name })
            : t("nav.signedInAsOn", { name: signedInAs.name, device: signedInAs.deviceName })}
        </li>
      )}
      {props.children}
      {props.onSwitchHousehold === undefined ? null : (
        <li>
          <button type="button" onClick={props.onSwitchHousehold}>
            {t("nav.switchHousehold")}
          </button>
        </li>
      )}
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
 * A menu in the banner: its icon, which opens a list below it. It is the page's own disclosure,
 * so it works without script and by keyboard; it closes when one of its places is chosen, when
 * the person presses or taps outside it, and on Escape.
 */
function BannerMenu(props: {
  /** Its name, for a screen reader and a pointer's hover: the eye sees the menu icon. */
  readonly label: string;
  /** Whether the current screen is one of its places: its icon is then marked as where one is. */
  readonly isCurrent: boolean;
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
  if (props.isCurrent) {
    classes.push("app-shell__menu--current");
  }
  return (
    <details ref={ref} className={classes.join(" ")}>
      <summary aria-label={props.label} title={props.label}>
        <MenuIcon />
      </summary>
      <ul>{props.children}</ul>
    </details>
  );
}
