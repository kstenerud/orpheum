// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { Register } from "./register.tsx";
import { enter, mount, servedBy, submit, type Heard } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";

it("with no link in the address, says so and offers nothing to fill in", async () => {
  const services = { ...servedBy({}, []), navigation: createNavigation(fakePlace("/register")) };
  const mounted = await mount(withServices(<Register />, services));
  expect(mounted.container.textContent).toContain("This address has no registration link in it.");
  expect(mounted.container.querySelector("input")).toBeNull();
  expect(mounted.container.querySelector("button")).toBeNull();
  mounted.unmount();
});

it("registers with the link's secret, and leaves it in neither the address nor the history (D218 §1)", async () => {
  const place = fakePlace("/register#abc_DEF-123");
  const heard: Heard[] = [];
  const services = {
    ...servedBy(
      {
        "POST /registration/link": [200, { isInvitation: false, name: "partner" }],
        "POST /registration": [
          201,
          {
            token: "new-token",
            deviceSecret: "phone-secret",
            user: { device: { id: 9, name: "Phone" } },
          },
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
  expect(services.session.deviceProof()).toBe("phone-secret");
  expect(services.navigation.route()).toEqual({ screen: "films" });
  expect(place.addresses).toEqual(["/films"]);
});

it("asks an invitation's person for their own name, and sends it with the password (D246 §2)", async () => {
  const place = fakePlace("/register#invited");
  const heard: Heard[] = [];
  const services = {
    ...servedBy(
      {
        "POST /registration/link": [200, { isInvitation: true }],
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
  expect(mounted.container.textContent).toContain("Make your account");
  const [name, password, again] = [...mounted.container.querySelectorAll("input")];
  await enter(name, "Gran");
  await enter(password, "secret");
  await enter(again, "secret");
  await submit(mounted.container.querySelector("form"));
  mounted.unmount();
  expect(heard.at(-1)).toMatchObject({
    methodAndPath: "POST /registration",
    body: { secret: "invited", name: "Gran", password: "secret" },
  });
});
