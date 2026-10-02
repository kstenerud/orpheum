// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";
import { useMemo, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { About } from "./about.tsx";
import { Account } from "./account.tsx";
import { AppShell } from "./app-shell.tsx";
import { Browse } from "./browse.tsx";
import { Files } from "./files.tsx";
import { Identify } from "./identify.tsx";
import { Jobs } from "./jobs.tsx";
import { Libraries } from "./libraries.tsx";
import { OfflineReminder } from "./offline-reminder.tsx";
import { People } from "./people.tsx";
import {
  BackgroundChangesContext,
  backgroundChanges,
  backingOff,
  clientWaits,
  type Poller,
} from "./polling.ts";
import { ApiProblem, dataOrThrow, throwIfRefused } from "./problem.ts";
import { PersonPage } from "./person.tsx";
import { Play } from "./play.tsx";
import { Register } from "./register.tsx";
import type { Route } from "./router.ts";
import { Search } from "./search.tsx";
import { Series } from "./series.tsx";
import type { Session } from "./session.ts";
import { Settings } from "./settings.tsx";
import { useRoute, useServices, useToken } from "./services.tsx";
import { SignIn } from "./sign-in.tsx";
import { System } from "./system.tsx";
import { Title } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The client: sign-in until there is a session, then the shell and the route's screen. A
 * registration invitation is its own page, signed in or not (D218 §2).
 */
export function App(): JSX.Element {
  const route = useRoute();
  const token = useToken();
  if (route.screen === "register") {
    return <Register />;
  }
  return token === undefined ? <SignIn /> : <SignedIn />;
}

/**
 * The signed-in client. A session the server no longer knows is signed out by the watch every
 * request passes (signed-out-watch.ts), with the server's reason.
 */
function SignedIn(): JSX.Element {
  const { api, session } = useServices();
  const route = useRoute();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const signOut = (): void => {
    signOutHere(api, session);
  };
  const isAdmin = me.state === "loaded" && me.value.isAdmin;
  const poller = useServersPoller();
  return (
    <BackgroundChangesContext value={poller}>
      <AppShell
        isAdmin={isAdmin}
        signedInAs={
          me.state === "loaded"
            ? { name: me.value.name, deviceName: me.value.device?.name }
            : undefined
        }
        currentScreen={route.screen}
        onSignOut={signOut}
      >
        {isAdmin ? <OfflineReminder /> : null}
        <Screen route={route} />
      </AppShell>
    </BackgroundChangesContext>
  );
}

/**
 * The poller of screens showing what the server changes in the background, on the waits the
 * server's client settings say (D237); D237's starting waits until they are read. A longest wait
 * shorter than the first is the first.
 */
function useServersPoller(): Poller {
  const { api } = useServices();
  const [clientSettings] = useResource("client-settings", async () =>
    dataOrThrow(await api.GET("/client-settings")),
  );
  const read = clientSettings.state === "loaded" ? clientSettings.value : undefined;
  const firstSeconds = read?.screenRefreshFirstWaitSeconds;
  const longestSeconds = read?.screenRefreshLongestWaitSeconds;
  return useMemo(
    () =>
      firstSeconds === undefined || longestSeconds === undefined
        ? backgroundChanges
        : backingOff(clientWaits({ firstSeconds, longestSeconds })),
    [firstSeconds, longestSeconds],
  );
}

/** The screen a route names. */
export function Screen(props: { readonly route: Route }): JSX.Element {
  const { t } = useTranslation();
  return viewerScreen(props.route) ?? adminScreen(props.route) ?? <p>{t("shell.notFound")}</p>;
}

/**
 * The screens anyone signed in may see: lists, a title, a series, playing, search, their account
 * and About.
 */
function viewerScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "films" || route.screen === "series") {
    return <Browse key={route.screen} kind={route.screen === "films" ? "movie" : "series"} />;
  }
  if (route.screen === "title") {
    return <Title key={route.itemId} id={route.itemId} />;
  }
  if (route.screen === "series-page") {
    return <Series key={route.collectionId} id={route.collectionId} />;
  }
  if (route.screen === "play") {
    return <Play key={route.fileId} fileId={route.fileId} />;
  }
  if (route.screen === "search") {
    return <Search key={route.query} query={route.query} />;
  }
  if (route.screen === "account") {
    return <Account />;
  }
  return route.screen === "about" ? <About /> : undefined;
}

/** The admin's screens; the server refuses them to anyone else. */
function adminScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "identify") {
    return <Identify />;
  }
  if (route.screen === "libraries") {
    return <Libraries />;
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
  if (route.screen === "people") {
    return <People />;
  }
  if (route.screen === "person") {
    return <PersonPage key={route.userId} userId={route.userId} />;
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
        if (cause instanceof ApiProblem && cause.code === "auth_required") {
          session.signOut();
          return;
        }
        session.signOutBecause("sign_out_unheard");
      },
    );
}
