// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { expect, it } from "vitest";

import { CreateAccountDialog, InviteDialog } from "./people-dialogs.tsx";
import { enter, mount, refusal, servedBy, submit } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const choices = { roleNames: ["user"], roles: undefined };
const ignore = (): void => undefined;

it("marks the name invalid while a name taken shows, described by the line, until it is edited", async () => {
  const mounted = await mount(
    withServices(
      <CreateAccountDialog choices={choices} onClose={ignore} onCreated={ignore} />,
      servedBy({ "POST /users": refusal(409, "conflict") }),
    ),
  );
  const [name, password] = [...mounted.container.ownerDocument.querySelectorAll("dialog input")];
  await enter(name, "partner");
  await enter(password, "pw");
  await submit(mounted.container.ownerDocument.querySelector("dialog form"));
  const line = mounted.container.ownerDocument.querySelector('dialog [role="alert"]');
  expect(line?.textContent).toBe("Someone already has that name. Choose another.");
  expect(name?.getAttribute("aria-invalid")).toBe("true");
  expect(name?.getAttribute("aria-describedby")).toBe(line?.id);
  expect(password?.hasAttribute("aria-invalid")).toBe(false);
  await enter(name, "partner2");
  expect(name?.hasAttribute("aria-invalid")).toBe(false);
  mounted.unmount();
});

it("marks the admin's note of an invitation invalid while the server's refusal of it shows", async () => {
  const mounted = await mount(
    withServices(
      <InviteDialog choices={choices} onClose={ignore} onInvited={ignore} />,
      servedBy({
        "POST /invitations": [
          400,
          {
            error: {
              code: "invalidRequest",
              message: "the note; too long",
              detail: { problems: [{ location: "body.adminNote", message: "too long" }] },
            },
          },
        ],
      }),
    ),
  );
  const note = mounted.container.ownerDocument.querySelector("dialog input");
  await enter(note, "Gran");
  await submit(mounted.container.ownerDocument.querySelector("dialog form"));
  const line = mounted.container.ownerDocument.querySelector('dialog [role="alert"]');
  expect(note?.getAttribute("aria-invalid")).toBe("true");
  expect(note?.getAttribute("aria-describedby")).toBe(line?.id);
  mounted.unmount();
});
