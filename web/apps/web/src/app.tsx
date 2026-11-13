// SPDX-License-Identifier: MPL-2.0

import { setReadyAheadPercent, type Api, type components } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ReloadWhenOutdated } from "./reload-when-outdated.tsx";
import { AppShell } from "./app-shell.tsx";
import { tabStorage } from "./browser-storage.ts";
import { Browse } from "./browse.tsx";
import { Home } from "./home.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { IsGuestContext } from "./guest-page.ts";
import { InputLayer } from "./input-layer.tsx";
import { LiveProvider } from "./live.tsx";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { castSignOverTheFilm } from "./status-tray.tsx";
import { useEpisodesReachedLetGoOnGoing } from "./episode-reached.ts";
import {
  About,
  Account,
  CastingLeft,
  CastReady,
  CreateLibraryPage,
  CreditedPerson,
  Files,
  HowToUpdate,
  Identify,
  Jobs,
  Lab,
  LabExtendPlayer,
  Libraries,
  LibraryBrowse,
  LibraryPage,
  NeedsAttention,
  Play,
  Register,
  Series,
  Settings,
  SignIn,
  System,
  ThisDevice,
  Title,
  TogetherLeft,
  TogetherViewer,
  UiGallery,
  UserPage,
  Users,
  usePreloadsPlay,
  usePreloadsSignIn,
} from "./later-screens.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { DebugToolsContext, type DebugToolsChoice } from "./debug-tools.tsx";
import { chipsOf } from "./list-filters.ts";
import {
  BackgroundChangesContext,
  backgroundChangeWaits,
  changingWaitsPoller,
  clientWaits,
  type Poller,
} from "./polling.ts";
import { ApiProblem, dataOrThrow, throwIfRefused } from "./problem.ts";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";
import { InterestSaidEveryContext } from "./page-interest.ts";
import { usePathMoveCount, useRoute, useServices, useToken } from "./services.tsx";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { HostVoiceProvider } from "./together-voice.tsx";
import { Remembered, useRemembered } from "./remembered.ts";
import { ShellReadContext, useShellPart, type ShellRead } from "./shell-read.ts";
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
  usePreloadsSignIn(token === undefined);
  if (props.route.screen === "register") {
    return (
      <LaterScreen resetKey={props.route.screen}>
        <Register />
      </LaterScreen>
    );
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
  return isLeavingForTheDiscovery ? (
    <LeavingForSignIn />
  ) : (
    <LaterScreen resetKey={props.route.screen}>
      <SignIn />
    </LaterScreen>
  );
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

/** Who is signed in, a way to read it again, and what the shell's one essential read said. */
interface SignedInRead {
  readonly me: Resource<components["schemas"]["UserBody"]>;
  readonly readMeAgain: () => void;
  readonly shellRead: ShellRead;
}

/**
 * Reads who is signed in, again on each path the page moves to, its first read the shell's one
 * essential read (D476), and keeps what it said last while it is read again.
 */
function useSignedInRead(): SignedInRead {
  const { api, session } = useServices();
  // Read again on each path the page moves to: an admin administers only from home (D297 §6), and a
  // page that started on the core's public name and moved to its LAN name showed no admin until
  // reloaded (the owner's Chrome, 2026-10-15).
  const meKey = `me:${String(usePathMoveCount())}`;
  // The first read is the shell's one essential read, which carries what the shell reads as it starts
  // (D476, principles §30); after a sign-in, its answer stands for that read, and none is made.
  const [firstMeKey] = useState(meKey);
  const [handedUser] = useState(() => session.handedUser());
  const isFirstRead = meKey === firstMeKey;
  const [meNow, readMeAgain] = useResource(
    meKey,
    async () =>
      dataOrThrow(
        isFirstRead
          ? await api.GET("/me", { params: { query: { shell: true } } })
          : await api.GET("/me"),
      ),
    { given: isFirstRead && handedUser !== undefined ? { value: handedUser } : undefined },
  );
  const [shellRead, setShellRead] = useState<ShellRead>(
    handedUser === undefined ? { state: "waiting" } : { state: "read", shell: handedUser.shell },
  );
  if (shellRead.state === "waiting" && meNow.state !== "loading") {
    setShellRead({
      state: "read",
      shell: meNow.state === "loaded" ? meNow.value.shell : undefined,
    });
  }
  // While it is read again, what it said last stays: the warning sign and the admin's places went
  // and came back at each move (the owner, 2026-10-17: "disappears for a split-second every now and
  // then").
  // Kept by this signed-in page alone, so another sign-in never starts from it.
  const [rememberedMe] = useState(() => new Remembered<components["schemas"]["UserBody"]>());
  const me = useRemembered("me", meNow, rememberedMe);
  return { me, readMeAgain, shellRead };
}

/**
 * The signed-in client. A session the server no longer knows is signed out by the watch every
 * request passes (signed-out-watch.ts), with the server's reason.
 */
function SignedIn(): JSX.Element {
  const { t } = useTranslation();
  const { session } = useServices();
  const route = useRoute();
  const { me, readMeAgain, shellRead } = useSignedInRead();
  // A person's shell and screen start reading alongside the read of who is signed in, not after it
  // (D476). A guest receiver, refused every read of a person's own, the client settings and the
  // person's Watch Together sessions among them (D469 §4, D465 §2), starts none: a session not yet
  // known to be a person's waits for the read, which says whose it is for the next start.
  const knownAs = session.knownAs();
  if (me.state === "loaded") {
    const kind = isGuestReceiver(me) ? "guestReceiver" : "person";
    if (kind !== knownAs) {
      session.noteKnownAs(kind);
    }
  }
  if (me.state === "loading" && knownAs !== "person") {
    return <p>{t("common.loading")}</p>;
  }
  if (isGuestReceiver(me)) {
    return (
      <IsGuestContext value={true}>
        <GuestReceiving route={route} />
      </IsGuestContext>
    );
  }
  return (
    <ShellReadContext value={shellRead}>
      <PersonSignedIn me={me} readMeAgain={readMeAgain} route={route} />
    </ShellReadContext>
  );
}

/**
 * The signed-in client of a person, not a guest receiver: its shell, its screens and what every
 * screen shares, read alongside who is signed in (me), which may still be loading.
 */
function PersonSignedIn(props: {
  readonly me: Resource<components["schemas"]["UserBody"]>;
  readonly readMeAgain: () => void;
  readonly route: Route;
}): JSX.Element {
  const { api, session, realm, switchHousehold } = useServices();
  const { me, readMeAgain, route } = props;
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
                    signedInAs={signedInAsOf(me)}
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
  const [clientSettings] = useResource(
    "client-settings",
    async () => dataOrThrow(await api.GET("/client-settings")),
    { given: useShellPart((shell) => shell.clientSettings) },
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
  // A film's and a series' pages lead to Play: its code loads ahead there (later-screens.tsx).
  usePreloadsPlay(props.route.screen === "title" || props.route.screen === "series-page");
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
 * A page signed in for one cast, as a guest receiver a device code made (D469 §4): no shell and none
 * of the person's screens, only the cast's, its ready page and its film, and while none has come, a
 * line saying it waits for it. Its cast's end signs it out, as a guest receiver's does (D465 §1).
 */
function GuestReceiving(props: { readonly route: Route }): JSX.Element {
  const { t } = useTranslation();
  const { route } = props;
  return (
    <LiveProvider>
      {route.screen === "play" || route.screen === "cast-ready" ? (
        <Screen route={route} />
      ) : (
        <p className="guest-receiving">{t("activate.waitingAsGuest")}</p>
      )}
    </LiveProvider>
  );
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

/** Whether me is a guest receiver's, signed in for one cast (D465 §1, D469). */
function isGuestReceiver(me: Resource<components["schemas"]["UserBody"]>): boolean {
  return me.state === "loaded" && me.value.isGuestReceiver === true;
}

/** Who the shell says is signed in, once me is read. */
function signedInAsOf(
  me: Resource<components["schemas"]["UserBody"]>,
): { readonly name: string; readonly deviceName: string | undefined } | undefined {
  return me.state === "loaded"
    ? { name: me.value.name, deviceName: me.value.device?.name }
    : undefined;
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
