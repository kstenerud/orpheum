// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { About } from "./about.tsx";
import { Account } from "./account.tsx";
import { AppShell } from "./app-shell.tsx";
import { Browse } from "./browse.tsx";
import { Files } from "./files.tsx";
import { Identify } from "./identify.tsx";
import { Libraries } from "./libraries.tsx";
import { People } from "./people.tsx";
import { PersonPage } from "./person.tsx";
import { Play } from "./play.tsx";
import { Register } from "./register.tsx";
import type { Route } from "./router.ts";
import { Search } from "./search.tsx";
import { Series } from "./series.tsx";
import { Settings } from "./settings.tsx";
import { useRoute, useServices, useToken } from "./services.tsx";
import { SignIn } from "./sign-in.tsx";
import { Title } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The client: sign-in until there is a session, then the shell and the route's screen. A
 * registration link is its own page, signed in or not (D218 §2).
 */
export function App(): JSX.Element {
  const route = useRoute();
  const token = useToken();
  if (route.screen === "register") {
    return <Register />;
  }
  return token === undefined ? <SignIn /> : <SignedIn />;
}

/** The signed-in client. A session the server no longer knows signs out here too. */
function SignedIn(): JSX.Element {
  const { api, session } = useServices();
  const route = useRoute();
  const [me] = useResource("me", async () => {
    const { data, response } = await api.GET("/me");
    if (response.status === 401) {
      session.signOut();
    }
    if (data === undefined) {
      throw new Error(response.statusText);
    }
    return data;
  });
  const signOut = (): void => {
    void api.DELETE("/session").finally(() => {
      session.signOut();
    });
  };
  return (
    <AppShell
      isAdmin={me.state === "loaded" && me.value.isAdmin}
      signedInAs={
        me.state === "loaded"
          ? { name: me.value.name, deviceName: me.value.device?.name }
          : undefined
      }
      current={route.screen}
      onSignOut={signOut}
    >
      <Screen route={route} />
    </AppShell>
  );
}

/** The screen a route names. */
export function Screen(props: { readonly route: Route }): JSX.Element {
  const { t } = useTranslation();
  return catalogueScreen(props.route) ?? adminScreen(props.route) ?? <p>{t("shell.notFound")}</p>;
}

/** The screens for watching: lists, a title, a series, search and About. */
function catalogueScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "films" || route.screen === "series") {
    return <Browse key={route.screen} kind={route.screen === "films" ? "movie" : "series"} />;
  }
  if (route.screen === "title") {
    return <Title key={route.id} id={route.id} />;
  }
  if (route.screen === "collection") {
    return <Series key={route.id} id={route.id} />;
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
  if (route.screen === "people") {
    return <People />;
  }
  if (route.screen === "person") {
    return <PersonPage key={route.id} userId={route.id} />;
  }
  return route.screen === "files" ? (
    <Files key={route.libraryId} libraryId={route.libraryId} />
  ) : undefined;
}
