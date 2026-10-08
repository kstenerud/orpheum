// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { describe, expect, it } from "vitest";

import { createUpdateWatch } from "./app-update.ts";
import { ReloadWhenOutdated } from "./reload-when-outdated.tsx";
import type { Route } from "./router.ts";
import { mount } from "./test-dom.tsx";

describe("an out-of-date page", () => {
  it("reloads at once where nothing plays, and only as its person leaves a film", async () => {
    const watch = createUpdateWatch();
    let reloads = 0;
    const reload = (): void => {
      reloads += 1;
    };
    let goTo: (route: Route) => void = () => undefined;
    function Routed(): JSX.Element {
      const [route, setRoute] = useState<Route>({ screen: "play", fileId: 3 });
      goTo = setRoute;
      return <ReloadWhenOutdated route={route} reload={reload} watch={watch} />;
    }
    const mounted = await mount(<Routed />);
    await act(async () => {
      watch.noteServed("index-old.js", "index-new.js");
      await Promise.resolve();
    });
    expect(reloads).toBe(0);
    expect(mounted.container.textContent).toContain("it loads as you leave the film");
    await act(async () => {
      goTo({ screen: "films" });
      await Promise.resolve();
    });
    expect(reloads).toBe(1);
    mounted.unmount();
  });

  it("does nothing while the page is up to date", async () => {
    let reloads = 0;
    const mounted = await mount(
      <ReloadWhenOutdated
        route={{ screen: "films" }}
        reload={() => {
          reloads += 1;
        }}
        watch={createUpdateWatch()}
      />,
    );
    expect(reloads).toBe(0);
    expect(mounted.container.textContent).toBe("");
    mounted.unmount();
  });
});
