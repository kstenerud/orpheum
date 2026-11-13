// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { Register } from "./register.tsx";
import { enter, mount, refusal, servedBy, submit, type Heard, type Mounted } from "./test-dom.tsx";
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
  expect(services.navigation.route()).toEqual({ screen: "home" });
  expect(place.addresses).toEqual(["/"]);
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

it("asks a reset link's person for their new password, and a first registration's for their password (D466)", async () => {
  const headingFor = async (kind: { readonly name: string; readonly isReset: boolean }) => {
    const services = {
      ...servedBy({ "POST /registration/link": [200, { isInvitation: false, ...kind }] }),
      navigation: createNavigation(fakePlace("/register#a-link")),
    };
    const mounted = await mount(withServices(<Register />, services));
    const heading = mounted.container.querySelector("h1")?.textContent;
    mounted.unmount();
    return heading;
  };
  expect(await headingFor({ name: "partner", isReset: true })).toBe("Choose your new password");
  expect(await headingFor({ name: "newcomer", isReset: false })).toBe("Choose your password");
});

it("says as it opens that a link no longer works, and offers nothing to fill in (D466)", async () => {
  const services = {
    ...servedBy({ "POST /registration/link": refusal(401, "authRequired") }),
    navigation: createNavigation(fakePlace("/register#used-link")),
  };
  const mounted = await mount(withServices(<Register />, services));
  expect(mounted.container.querySelector("h1")?.textContent).toBe("This link no longer works");
  expect(mounted.container.textContent).toContain("This link does not work: it was used");
  expect(mounted.container.querySelector("input")).toBeNull();
  expect(mounted.container.querySelector("button")).toBeNull();
  mounted.unmount();
});

it("still offers the form when the link could not be read for another reason", async () => {
  const services = {
    ...servedBy({ "POST /registration/link": refusal(503, "unavailable") }),
    navigation: createNavigation(fakePlace("/register#a-link")),
  };
  const mounted = await mount(withServices(<Register />, services));
  expect(mounted.container.querySelector("h1")?.textContent).toBe("Choose your password");
  expect(mounted.container.querySelectorAll("input")).toHaveLength(2);
  mounted.unmount();
});

/** The registration page for an invitation whose name is taken, with its three fields. */
async function registeringATakenName(): Promise<
  readonly [Mounted, ...(HTMLInputElement | undefined)[]]
> {
  const services = {
    ...servedBy({
      "POST /registration/link": [200, { isInvitation: true }],
      "POST /registration": refusal(409, "conflict"),
    }),
    navigation: createNavigation(fakePlace("/register#invited")),
  };
  const mounted = await mount(withServices(<Register />, services));
  return [mounted, ...mounted.container.querySelectorAll("input")];
}

it("marks the password typed again invalid while it does not match, until it is typed again", async () => {
  const [mounted, name, password, again] = await registeringATakenName();
  await enter(name, "Gran");
  await enter(password, "secret");
  await enter(again, "secrets");
  await submit(mounted.container.querySelector("form"));
  expect(again?.getAttribute("aria-invalid")).toBe("true");
  expect(again?.getAttribute("aria-describedby")).toBe(
    mounted.container.querySelector('[role="alert"]')?.id,
  );
  expect(name?.hasAttribute("aria-invalid")).toBe(false);
  await enter(again, "secret");
  expect(again?.hasAttribute("aria-invalid")).toBe(false);
  mounted.unmount();
});

it("marks the name invalid while a name taken shows, described by the line", async () => {
  const [mounted, name, password, again] = await registeringATakenName();
  await enter(name, "Gran");
  await enter(password, "secret");
  await enter(again, "secret");
  await submit(mounted.container.querySelector("form"));
  const line = mounted.container.querySelector('[role="alert"]');
  expect(line?.textContent).toContain("already has that name");
  expect(name?.getAttribute("aria-invalid")).toBe("true");
  expect(name?.getAttribute("aria-describedby")).toBe(line?.id);
  expect(again?.hasAttribute("aria-invalid")).toBe(false);
  mounted.unmount();
});
