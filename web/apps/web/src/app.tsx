// SPDX-License-Identifier: MPL-2.0

import { setReadyAheadPercent, type Api, type components } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ReloadWhenOutdated } from "./reload-when-outdated.tsx";
import { About } from "./about.tsx";
import { Account } from "./account.tsx";
import { AppShell } from "./app-shell.tsx";
import { tabStorage } from "./browser-storage.ts";
import { Browse } from "./browse.tsx";
import { CreditedPerson } from "./credits.tsx";
import { Home } from "./home.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { InputLayer } from "./input-layer.tsx";
import { LiveProvider } from "./live.tsx";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { castSignOverTheFilm } from "./status-tray.tsx";
import { useEpisodesReachedLetGoOnGoing } from "./episode-reached.ts";
import {
  CastReady,
  CreateLibraryPage,
  Files,
  HowToUpdate,
  Identify,
  Jobs,
  Lab,
  LabExtendPlayer,
  Libraries,
  LibraryPage,
  NeedsAttention,
  Play,
  Series,
  Settings,
  System,
  Title,
  TogetherViewer,
  UiGallery,
  UserPage,
  Users,
} from "./later-screens.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { DebugToolsContext, type DebugToolsChoice } from "./debug-tools.tsx";
import { LibraryBrowse } from "./library-browse.tsx";
import { chipsOf } from "./list-filters.ts";
import {
  BackgroundChangesContext,
  backgroundChangeWaits,
  changingWaitsPoller,
  clientWaits,
  type Poller,
} from "./polling.ts";
import { ApiProblem, dataOrThrow, throwIfRefused } from "./problem.ts";
import { CastingLeft, TogetherLeft } from "./old-addresses.tsx";
import { Register } from "./register.tsx";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";
import { InterestSaidEveryContext } from "./page-interest.ts";
import { usePathMoveCount, useRoute, useServices, useToken } from "./services.tsx";
import { SignIn } from "./sign-in.tsx";
import { ThisDevice } from "./this-device.tsx";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { HostVoiceProvider } from "./together-voice.tsx";
import { Remembered, useRemembered } from "./remembered.ts";
import { LaterScreen } from "./screen-load.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/**
 * The client: sign-in until there is a session, then the shell and the route's screen, every one
 * driven by a remote, a keyboard or a controller through the input layer (input-layer.tsx). A
 * registration invitation is its own page, signed in or not (D218 §2); so is a Watch Together
 * viewer's, who sees only the host's playback, never the app around it (D246 §3).
 */
export function App(): JSX.Element {
  const route = useRoute();
  useEpisodesReachedLetGoOnGoing();
  return (
    <InputLayer>
      <RoutedApp route={route} />
      <ReloadWhenOutdated route={route} />
    </InputLayer>
  );
}

/** The screen route names, signed in or not. */
function RoutedApp(props: { readonly route: ReturnType<typeof useRoute> }): JSX.Element {
  const token = useToken();
  const isLeavingForTheDiscovery = useIsLeavingForTheDiscovery(token);
  if (props.route.screen === "register") {
    return <Register />;
  }
  if (props.route.screen === "together" && token === undefined) {
    // A guest by link alone sees none of the app (D246 §3): a page of its own, no debug tools. A
    // signed-in guest's is a screen in the shell (D372 §9; Screen).
    return (
      <LaterScreen resetKey={props.route.screen}>
        <LiveProvider>
          <TogetherViewer />
        </LiveProvider>
      </LaterScreen>
    );
  }
  if (props.route.screen === "lab-extend-player") {
    // The lab's test pattern on the TV's screen: no shell, and no server, so no sign-in.
    return (
      <LaterScreen resetKey={props.route.screen}>
        <LabExtendPlayer />
      </LaterScreen>
    );
  }
  if (token !== undefined) {
    return <SignedIn />;
  }
  return isLeavingForTheDiscovery ? <LeavingForSignIn /> : <SignIn />;
}

/**
 * Whether this page, under a discovery's loader that leaves for the discovery's sign-in once signed
 * out (Services.willLeaveWhenSignedOut; loader-at-client-origin.tsx's leaveWhenSignedOut), was signed
 * out: the server's sign-in, drawn meanwhile, showed for a while before the discovery's replaced it
 * (the owner's Fire TV, 2026-10-29: "That would be confusing for a user"). A page that opened signed
 * out shows the server's sign-in as before, the loader going nowhere.
 */
function useIsLeavingForTheDiscovery(token: string | undefined): boolean {
  const { realm, willLeaveWhenSignedOut } = useServices();
  const [wasSignedIn, setWasSignedIn] = useState(token !== undefined);
  if (token !== undefined && !wasSignedIn) {
    setWasSignedIn(true);
  }
  return (willLeaveWhenSignedOut ?? realm !== undefined) && wasSignedIn && token === undefined;
}

/** What a page signed out shows while the loader takes it to the discovery's sign-in. */
function LeavingForSignIn(): JSX.Element {
  const { t } = useTranslation();
  return <p>{t("signIn.leaving")}</p>;
}

/**
 * The signed-in client. A session the server no longer knows is signed out by the watch every
 * request passes (signed-out-watch.ts), with the server's reason.
 */
function SignedIn(): JSX.Element {
  const { api, session, realm, switchHousehold } = useServices();
  const route = useRoute();
  // Read again on each path the page moves to: an admin administers only from home (D297 §6), and a
  // page that started on the core's public name and moved to its LAN name showed no admin until
  // reloaded (the owner's Chrome, 2026-10-15).
  const [meNow, readMeAgain] = useResource(`me:${String(usePathMoveCount())}`, async () =>
    dataOrThrow(await api.GET("/me")),
  );
  // While it is read again, what it said last stays: the warning sign and the admin's places went
  // and came back at each move (the owner, 2026-10-17: "disappears for a split-second every now and
  // then").
  // Kept by this signed-in page alone, so another sign-in never starts from it.
  const [rememberedMe] = useState(() => new Remembered<components["schemas"]["UserBody"]>());
  const me = useRemembered("me", meNow, rememberedMe);
  const signOut = (): void => {
    signOutHere(api, session);
  };
  const isAdmin = me.state === "loaded" && me.value.isAdmin;
  const debugTools = useDebugToolsOf(me, readMeAgain);
  const clientSettings = useClientSettings();
  const poller = useServersPoller(clientSettings);
  useReadyAheadOfTheServer(clientSettings);
  // The session this tab hosts, whose voice every screen shares (D281 §5).
  const hostMembership = useMembership(tabStorage, "host");
  useEffect(() => {
    // A session this person is in follows them to this device (D267 §2); one not had back is
    // offered again by the screens that start or join one.
    void haveBackMySessions(api).catch(() => undefined);
  }, [api]);
  return (
    <IsAdminContext value={isAdmin}>
      <DebugToolsContext value={debugTools}>
        <LiveProvider>
          <CastSignOverTheFilmContext value={castSignOverTheFilm}>
            <HostVoiceProvider membership={hostMembership}>
              <BackgroundChangesContext value={poller}>
                <InterestSaidEveryContext value={clientSettings?.interestSaidEverySeconds ?? 15}>
                  <AppShell
                    isAdmin={isAdmin}
                    canHostWatchTogether={me.state === "loaded" && me.value.canHostWatchTogether}
                    signedInAs={
                      me.state === "loaded"
                        ? { name: me.value.name, deviceName: me.value.device?.name }
                        : undefined
                    }
                    currentScreen={route.screen}
                    administersFromHomeAt={administersFromHomeAt(me)}
                    realm={realm}
                    showsDeveloperPages={clientSettings?.shouldShowDeveloperPages === true}
                    onSwitchHousehold={switchHousehold}
                    onSignOut={signOut}
                  >
                    <DeviceTestsOfferedContext value={clientSettings?.shouldOfferDeviceTests}>
                      <Screen route={route} />
                    </DeviceTestsOfferedContext>
                  </AppShell>
                </InterestSaidEveryContext>
              </BackgroundChangesContext>
            </HostVoiceProvider>
          </CastSignOverTheFilmContext>
        </LiveProvider>
      </DebugToolsContext>
    </IsAdminContext>
  );
}

/**
 * The person's own Show debug tools, read with who they are (me), and read again once they change
 * it (readMeAgain).
 */
function useDebugToolsOf(
  me: Resource<components["schemas"]["UserBody"]>,
  readMeAgain: () => void,
): DebugToolsChoice {
  const isShown = me.state === "loaded" && me.value.showsDebugTools;
  return useMemo(() => ({ isShown, onChanged: readMeAgain }), [isShown, readMeAgain]);
}

/**
 * Has every player of the page wait, before it plays, by the server's margin on its next chunk's
 * load, once read (readyAheadOfNextChunkPercent; the SDK's ready-ahead.ts).
 */
function useReadyAheadOfTheServer(
  read: components["schemas"]["ClientSettingsBody"] | undefined,
): void {
  const percent = read?.readyAheadOfNextChunkPercent;
  useEffect(() => {
    if (percent !== undefined) {
      setReadyAheadPercent(percent);
    }
  }, [percent]);
}

/** The server settings every client reads, once read. */
function useClientSettings(): components["schemas"]["ClientSettingsBody"] | undefined {
  const { api } = useServices();
  const [clientSettings] = useResource("client-settings", async () =>
    dataOrThrow(await api.GET("/client-settings")),
  );
  return clientSettings.state === "loaded" ? clientSettings.value : undefined;
}

/**
 * The poller of screens showing what the server changes in the background, on the waits the
 * server's client settings say (D237); D237's starting waits until they are read. A longest wait
 * shorter than the first is the first. One poller for the page's life, which takes the waits read
 * from its next wait on: a new one started every polled screen again, so each read twice at the
 * start (the audit of 2026-10-25, W3).
 */
function useServersPoller(read: components["schemas"]["ClientSettingsBody"] | undefined): Poller {
  const firstSeconds = read?.screenRefreshFirstWaitSeconds;
  const longestSeconds = read?.screenRefreshLongestWaitSeconds;
  const failedSeconds = read?.screenRefreshFailedWaitSeconds;
  const [changing] = useState(() => changingWaitsPoller(backgroundChangeWaits));
  useEffect(() => {
    changing.setWaits(
      firstSeconds === undefined || longestSeconds === undefined || failedSeconds === undefined
        ? backgroundChangeWaits
        : clientWaits({ firstSeconds, longestSeconds, failedSeconds }),
    );
  }, [changing, firstSeconds, longestSeconds, failedSeconds]);
  return changing.poller;
}

/** The screen a route names. */
export function Screen(props: { readonly route: Route }): JSX.Element {
  const { t } = useTranslation();
  return (
    <LaterScreen resetKey={JSON.stringify(props.route)}>
      {viewerScreen(props.route) ?? nonLibraryScreen(props.route) ?? adminScreen(props.route) ?? (
        <p>{t("shell.notFound")}</p>
      )}
    </LaterScreen>
  );
}

/** The screens anyone signed in may see to watch: lists, a title, a series, playing, Libraries. */
function viewerScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "home") {
    return <Home />;
  }
  if (route.screen === "films" || route.screen === "series") {
    const { screen, ...state } = route;
    return <Browse key={screen} kind={screen === "films" ? "film" : "series"} state={state} />;
  }
  if (route.screen === "title") {
    return <Title key={route.itemId} id={route.itemId} />;
  }
  if (route.screen === "series-page") {
    return <Series key={route.collectionId} id={route.collectionId} season={route.season} />;
  }
  if (route.screen === "play") {
    return (
      <Play
        key={route.fileId}
        fileId={route.fileId}
        atMs={route.atMs}
        isPaused={route.isPaused === true}
        shouldPlayHere={route.shouldPlayHere === true}
      />
    );
  }
  if (route.screen === "browse") {
    return (
      <LibraryBrowse
        library={route.library}
        view={route.view}
        folder={route.folder}
        sort={route.sort}
        order={route.order}
        query={route.query ?? ""}
        filters={chipsOf(route)}
      />
    );
  }
  return creditedScreen(route);
}

/** A credited person's page: someone a title credits, and their titles (search-by-more §4.4). */
function creditedScreen(route: Route): JSX.Element | undefined {
  return route.screen === "person" ? (
    <CreditedPerson key={route.personId} id={route.personId} />
  ) : undefined;
}

/**
 * The screens anyone signed in may see beyond the library: their account, Watch Together (the
 * server refuses a session to a role that may not host), the device lab and About.
 */
function nonLibraryScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "account") {
    return <Account />;
  }
  if (route.screen === "this-device") {
    return <ThisDevice />;
  }
  if (route.screen === "watch-together") {
    // Watch Together is a dialog (D384 §2): its old address goes to the group's film, or home.
    return <TogetherLeft />;
  }
  if (route.screen === "together") {
    // A signed-in guest's page, in the shell: the tray, the menu and Your account stay theirs while
    // they are in a session, and they may go anywhere and come back by the tray (D372 §9). Its live
    // connection, the shell's, says whether this device casts the film to another (D371 §4).
    return <TogetherViewer />;
  }
  const casting = castingScreen(route);
  if (casting !== undefined) {
    return casting;
  }
  if (route.screen === "ui-gallery") {
    return <UiGallery />;
  }
  if (route.screen === "lab") {
    return <Lab key={route.probe ?? "menu"} probe={route.probe} />;
  }
  return route.screen === "about" ? <About /> : undefined;
}

/**
 * Casting's screens: a receiver's ready page (D364 §5), and an old address of the casting page, which
 * D384 §2 took away, sent to the film's page.
 */
function castingScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "casting") {
    return <CastingLeft />;
  }
  return route.screen === "cast-ready" ? <CastReady /> : undefined;
}

/** The admin's screens; the server refuses them to anyone else. */
function adminScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "identify") {
    return <Identify />;
  }
  if (route.screen === "settings") {
    return <Settings />;
  }
  if (route.screen === "jobs") {
    return <Jobs />;
  }
  if (route.screen === "system") {
    return <System />;
  }
  if (route.screen === "update") {
    return <HowToUpdate />;
  }
  if (route.screen === "needs-attention") {
    return <NeedsAttention />;
  }
  if (route.screen === "users") {
    return <Users />;
  }
  if (route.screen === "user") {
    return <UserPage key={route.userId} userId={route.userId} />;
  }
  return librariesScreen(route);
}

/**
 * The admin's library screens (D361): the list, one library's page, creating one, and what the
 * scanner saw of one.
 */
function librariesScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "libraries") {
    return <Libraries />;
  }
  if (route.screen === "library") {
    return <LibraryPage key={route.libraryId} libraryId={route.libraryId} />;
  }
  if (route.screen === "new-library") {
    return <CreateLibraryPage />;
  }
  return route.screen === "files" ? (
    <Files key={route.libraryId} libraryId={route.libraryId} />
  ) : undefined;
}

/**
 * Signs this device out: at once here, whatever the server says. When the server could not be
 * told (it refused, or could not be reached), the sign-in page says so, since the session may
 * still work there until it expires; one the server had already ended needs no word.
 */
function signOutHere(api: Api, session: Session): void {
  api
    .DELETE("/session")
    .then(throwIfRefused)
    .then(
      () => {
        session.signOut();
      },
      (cause: unknown) => {
        if (cause instanceof ApiProblem && cause.code === "authRequired") {
          session.signOut();
          return;
        }
        session.signOutBecause("signOutUnheard");
      },
    );
}

/**
 * For an admin who does not administer here (away from home, D297 §6): the server's home address,
 * or "" when it has none ready; undefined for anyone else, or before /me is read.
 */
function administersFromHomeAt(
  me: Resource<components["schemas"]["UserBody"]>,
): string | undefined {
  if (me.state !== "loaded" || me.value.role !== "admin" || me.value.isAdmin) {
    return undefined;
  }
  return me.value.homeAddresses?.[0] ?? "";
}
