// SPDX-License-Identifier: MPL-2.0

/**
 * A form's fields and its submit walked by a mouse, a finger, a keyboard and a remote's keys in a
 * real browser, at a phone's, a computer's and a television's width (D538, interface rule 14): a
 * field's label, hint and error tied to its input (field.tsx); a password's Show password inside its
 * edge (password-field.tsx); a submit that stays enabled, says what is missing at its field and
 * moves focus there, and while it works shows so and takes no second press (submit-button.tsx).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { missingText, TextField, useFormFields } from "./field.tsx";
import { submitWith } from "./form-row.tsx";
import { i18n } from "./i18n.ts";
import { startInput, type Input } from "./input-commands.ts";
import { PasswordField } from "./password-field.tsx";
import { SubmitButton } from "./submit-button.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";

/** A sign-in's form: a name with a hint, a password and its submit, which waits once sent. */
function SignInColumn(props: { readonly onSent: () => void }): JSX.Element {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [isBusy, setIsBusy] = useState(false);
  const fields = useFormFields(undefined, []);
  return (
    <main>
      <button type="button">Above</button>
      <form
        noValidate
        onSubmit={submitWith(() => {
          const isFilled = fields.check([
            ["name", missingText(i18n.t, "Username", name)],
            ["password", missingText(i18n.t, "Password", password)],
          ]);
          if (isFilled) {
            props.onSent();
            setIsBusy(true);
          }
        })}
      >
        <TextField
          {...fields.fieldOf("name")}
          label="Username"
          hint="The one you signed up with"
          autoComplete="username"
          value={name}
          onChange={fields.editing("name", setName)}
        />
        <PasswordField
          {...fields.fieldOf("password")}
          label="Password"
          autoComplete="current-password"
          value={password}
          onChange={fields.editing("password", setPassword)}
        />
        <SubmitButton label="Sign in" isBusy={isBusy} />
      </form>
    </main>
  );
}

let input: Input | undefined;
let mounted: Mounted | undefined;
let sent = 0;

beforeEach(async () => {
  sent = 0;
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(960, 600);
  mounted = await mount(
    <SignInColumn
      onSent={() => {
        sent += 1;
      }}
    />,
  );
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The form's element matching selector. */
function the(selector: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(selector);
  if (found === null) {
    throw new Error(`no ${selector}`);
  }
  return found;
}

/** The form's element matching selector, of the kind kind. */
function theOf<E extends HTMLElement>(kind: new () => E, selector: string): E {
  const found = the(selector);
  if (!(found instanceof kind)) {
    throw new Error(`${selector} is not a ${kind.name}`);
  }
  return found;
}

const nameInput = (): HTMLInputElement => theOf(HTMLInputElement, 'input[autocomplete="username"]');
const passwordInput = (): HTMLInputElement => theOf(HTMLInputElement, ".password-field__input");
const showButton = (): HTMLButtonElement =>
  theOf(HTMLButtonElement, ".password-field .icon-button");
const submitButton = (): HTMLButtonElement => theOf(HTMLButtonElement, 'button[type="submit"]');

describe("a field", () => {
  it("shows its label over its input, a click on the label focusing it, and its hint describing it", async () => {
    await userEvent.click(the("label"));
    expect(document.activeElement).toBe(nameInput());
    const described = nameInput().getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(described)?.textContent).toBe("The one you signed up with");
    expect(nameInput().hasAttribute("placeholder")).toBe(false);
    expect(passwordInput().hasAttribute("placeholder")).toBe(false);
  });

  it("is reached by Tab with its ring, and by a remote's Down and Up", async () => {
    theOf(HTMLButtonElement, "button").focus();
    await userEvent.keyboard("{Tab}");
    expect(document.activeElement).toBe(nameInput());
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Tab}");
    expect(document.activeElement).toBe(passwordInput());
    theOf(HTMLButtonElement, "button").focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(nameInput());
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(passwordInput());
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(nameInput());
  });

  it("lays its input across its width, a fingertip tall, at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const box = nameInput().getBoundingClientRect();
      const label = the("label").getBoundingClientRect();
      expect(box.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(box.right).toBeLessThanOrEqual(width);
      expect(label.bottom).toBeLessThanOrEqual(box.top);
    }
  });
});

describe("a password field", () => {
  it("shows and hides what is typed by a click and by a tap, its button saying which", async () => {
    await userEvent.type(passwordInput(), "secret");
    expect(passwordInput().type).toBe("password");
    await userEvent.click(showButton());
    expect(passwordInput().type).toBe("text");
    expect(showButton().getAttribute("aria-pressed")).toBe("true");
    tapOn(showButton());
    expect(passwordInput().type).toBe("password");
    expect(showButton().getAttribute("aria-pressed")).toBe("false");
    expect(passwordInput().value).toBe("secret");
  });

  it("is shown by Tab then Space or Enter, and by a remote's Right then OK", async () => {
    passwordInput().focus();
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("Show password");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard(" ");
    expect(passwordInput().type).toBe("text");
    await userEvent.keyboard("{Enter}");
    expect(passwordInput().type).toBe("password");
    passwordInput().focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(showButton());
    await userEvent.keyboard("{Enter}");
    expect(passwordInput().type).toBe("text");
    expect(document.activeElement).toBe(showButton());
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(passwordInput());
  });

  it("asks for the password the browser keeps, and hides it again once the form is sent", async () => {
    expect(passwordInput().getAttribute("autocomplete")).toBe("current-password");
    await userEvent.type(nameInput(), "owner");
    await userEvent.type(passwordInput(), "secret");
    await userEvent.click(showButton());
    await userEvent.click(submitButton());
    expect(passwordInput().type).toBe("password");
  });

  it("sets its button beside the input's end, a fingertip square on its line, at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const field = passwordInput().getBoundingClientRect();
      const button = showButton().getBoundingClientRect();
      expect(button.width, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(button.height).toBeGreaterThanOrEqual(44);
      expect(button.left).toBeGreaterThanOrEqual(field.right);
      expect(button.right).toBeLessThanOrEqual(width);
      expect(
        Math.abs(button.top + button.height / 2 - (field.top + field.height / 2)),
      ).toBeLessThan(1);
    }
  });
});

describe("a submit", () => {
  it("stays enabled with the form empty, and says what is missing at its field, focus moving there", async () => {
    expect(submitButton().disabled).toBe(false);
    await userEvent.type(nameInput(), "owner");
    await userEvent.click(submitButton());
    expect(sent).toBe(0);
    expect(document.activeElement).toBe(passwordInput());
    expect(passwordInput().getAttribute("aria-invalid")).toBe("true");
    const error = the(".field__error");
    expect(error.textContent).toBe("Enter password");
    expect(passwordInput().getAttribute("aria-describedby")).toContain(error.id);
    expect(getComputedStyle(passwordInput()).borderColor).toBe(getComputedStyle(error).color);
    await userEvent.type(passwordInput(), "s");
    expect(document.querySelector(".field__error")).toBeNull();
  });

  it("says each missing at its own field when sent by a tap, focus on the first", () => {
    tapOn(submitButton());
    expect(document.querySelectorAll(".field__error")).toHaveLength(2);
    expect(document.activeElement).toBe(nameInput());
    expect(sent).toBe(0);
  });

  it("while it works, shows it waits, keeps its width and its focus, and takes no second press", async () => {
    await userEvent.type(nameInput(), "owner");
    await userEvent.type(passwordInput(), "secret");
    submitButton().focus();
    const before = submitButton().getBoundingClientRect().width;
    await userEvent.keyboard("{Enter}");
    expect(sent).toBe(1);
    expect(submitButton().getAttribute("aria-busy")).toBe("true");
    expect(submitButton().disabled).toBe(false);
    expect(submitButton().querySelector(".submit-button__spinner")).not.toBeNull();
    expect(submitButton().textContent).toBe("Sign in");
    expect(submitButton().getBoundingClientRect().width).toBe(before);
    expect(document.activeElement).toBe(submitButton());
    await userEvent.keyboard("{Enter}");
    await userEvent.click(submitButton());
    tapOn(submitButton());
    passwordInput().focus();
    await userEvent.keyboard("{Enter}");
    expect(sent).toBe(1);
  });

  it("is sent by a remote's OK, which the focus engine reaches by Down", async () => {
    await userEvent.type(nameInput(), "owner");
    await userEvent.type(passwordInput(), "secret");
    passwordInput().focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(submitButton());
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(sent).toBe(1);
  });

  it("sets its spinner inside its start's padding, clear of its label, at each width", async () => {
    await userEvent.type(nameInput(), "owner");
    await userEvent.type(passwordInput(), "secret");
    await userEvent.click(submitButton());
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const button = submitButton().getBoundingClientRect();
      const spinner = the(".submit-button__spinner").getBoundingClientRect();
      expect(button.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(spinner.left).toBeGreaterThanOrEqual(button.left);
      expect(spinner.right).toBeLessThan(button.right);
      expect(spinner.top).toBeGreaterThanOrEqual(button.top);
      expect(spinner.bottom).toBeLessThanOrEqual(button.bottom);
    }
  });
});
