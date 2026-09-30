// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { Register } from "./register.tsx";
import { mount, servedBy, submit, type Heard } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";

it("registers with the link's secret, and leaves it in neither the address nor the history (D218 §1)", async () => {
  const place = fakePlace("/register#abc_DEF-123");
  const heard: Heard[] = [];
  const services = {
    ...servedBy(
      {
        "POST /registration": [
          201,
          { token: "new-token", user: { device: { id: 9, name: "Phone" } } },
        ],
      },
      heard,
    ),
    navigation: createNavigation(place),
  };
  const mounted = await mount(withServices(<Register />, services));
  await submit(mounted.container.querySelector("form"));
  mounted.unmount();
  expect(heard.at(-1)).toMatchObject({
    methodAndPath: "POST /registration",
    body: { secret: "abc_DEF-123" },
  });
  expect(services.session.token()).toBe("new-token");
  expect(services.navigation.route()).toEqual({ screen: "films" });
  expect(place.addresses).toEqual(["/films"]);
});
