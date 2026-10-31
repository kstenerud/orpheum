// SPDX-License-Identifier: MPL-2.0

/**
 * A remote's or a keyboard's OK in a form. OK on a checkbox or a radio ticks it and never submits its
 * form, whatever key events the browser sends after: on the owner's Fire TV, OK on the discovery's
 * sign-in's "remember my realm" box signed in at once (2026-10-30), its page running no input layer,
 * and a browser takes Enter on a focused checkbox for the form's implicit submission
 * (idiosyncrasies.md). Here the OK's key down ticks the box and is kept from the browser, and the same
 * key's press and key up, and any submit until that key is up, are stopped: a browser that sends them
 * anyway, as Silk sends a held OK's repeats as fresh presses with no key up between, submits nothing.
 * Each OK in a form is noted for the core's log at its debug level, with every event of it, so a
 * recurrence explains itself.
 */

import { keyCodeOf } from "./input-buttons.ts";

/** Android's key code for a remote's D-pad centre, which a TV's browser may send for OK. */
const dpadCentreKeyCode = 23;

/** Enter's key code, the OK of most remotes and every keyboard. */
const enterKeyCode = 13;

/** Whether event is a remote's or a keyboard's OK. */
function isOk(event: KeyboardEvent): boolean {
  const keyCode = keyCodeOf(event);
  return event.key === "Enter" || keyCode === enterKeyCode || keyCode === dpadCentreKeyCode;
}

/** Whether element is a box OK ticks: a checkbox or a radio. */
function isCheck(element: Element | null): element is HTMLInputElement {
  return (
    element instanceof HTMLInputElement && (element.type === "checkbox" || element.type === "radio")
  );
}

/** What element is, as a note says it: "checkbox input", "text input", "button"… */
function kindOf(element: Element): string {
  if (element instanceof HTMLInputElement) {
    return `${element.type} input`;
  }
  return element.tagName.toLowerCase();
}

/** A key event as a note says it: its type, its key and key code, and whether marked a repeat. */
function keyText(event: KeyboardEvent): string {
  return `${event.type} ${event.key}/${String(keyCodeOf(event))}${event.repeat ? " repeat" : ""}`;
}

/** One OK held down in a form: its key, what had focus, and what was heard of it so far. */
interface OkPress {
  readonly key: string;
  readonly keyCode: number;
  readonly on: Element;
  readonly isOnCheck: boolean;
  readonly heard: string[];
}

/** Whether event is a key event of press's own key. */
function isOfPress(press: OkPress | undefined, event: Event): event is KeyboardEvent {
  return (
    event instanceof KeyboardEvent && press?.key === event.key && press.keyCode === keyCodeOf(event)
  );
}

/** Keeps event from the browser and from every listener after, saying so in heard. */
function stopped(event: Event, heard: string[]): void {
  event.preventDefault();
  event.stopImmediatePropagation();
  heard.push("stopped");
}

/** Where guardFormsOk listens and what it reads: the window, and what has focus. */
export interface FormOkPage {
  /** Where key events and submits are heard first: the window, in capture. */
  readonly target: EventTarget;
  readonly focused: () => Element | null;
}

/** The guard's one OK held now, and how each event moves it. */
class FormOkGuard {
  private press: OkPress | undefined;

  constructor(
    private readonly page: FormOkPage,
    private readonly note: (line: string) => void,
  ) {}

  /** Ends the press held, noting what was heard of it. */
  readonly end = (): void => {
    if (this.press !== undefined) {
      this.note(`a form's OK on a ${kindOf(this.press.on)}: ${this.press.heard.join(", ")}`);
      this.press = undefined;
    }
  };

  /** An OK's key down: a held one's repeat, or a press of its own in a form, a box ticked by it. */
  readonly keyDown = (event: Event): void => {
    // One answered already, as by another guard on the same page (the loader's under the client's).
    if (!(event instanceof KeyboardEvent) || !isOk(event) || event.defaultPrevented) {
      return;
    }
    const focused = this.page.focused();
    if (!this.isHeld(event, focused)) {
      this.end();
      if (focused?.closest("form")) {
        this.pressed(event, focused);
      }
    }
  };

  /**
   * Whether event is a key down of the OK held now on focused: Silk's repeats come unmarked, with no
   * key up between, and none ticks the box again.
   */
  private isHeld(event: KeyboardEvent, focused: Element | null): boolean {
    const held = this.press;
    if (held === undefined || !isOfPress(held, event) || held.on !== focused) {
      return false;
    }
    held.heard.push(keyText(event));
    if (held.isOnCheck) {
      event.preventDefault();
    }
    return true;
  }

  /** An OK pressed on focused, in a form: a box it is on ticked, the key kept from the browser. */
  private pressed(event: KeyboardEvent, focused: Element): void {
    this.press = {
      key: event.key,
      keyCode: keyCodeOf(event),
      on: focused,
      isOnCheck: isCheck(focused),
      heard: [keyText(event)],
    };
    if (isCheck(focused)) {
      // Kept from the browser and from the input layer, which leaves a key down already answered.
      event.preventDefault();
      focused.click();
      this.press.heard.push(focused.checked ? "ticked" : "unticked");
    }
  }

  /** The OK's key press or key up: kept from the browser over a box; the key up ends the press. */
  readonly keyAfter = (event: Event): void => {
    const current = this.press;
    if (current === undefined || !isOfPress(current, event)) {
      return;
    }
    current.heard.push(keyText(event));
    if (current.isOnCheck) {
      stopped(event, current.heard);
    }
    if (event.type === "keyup") {
      // Once what the key up does by default is done: a submit it makes is stopped too.
      setTimeout(() => {
        if (this.press === current) {
          this.end();
        }
      }, 0);
    }
  };

  /** A form's submit while an OK is held: stopped where the OK was on a box. */
  readonly submitted = (event: Event): void => {
    if (this.press !== undefined) {
      this.press.heard.push("submit");
      if (this.press.isOnCheck) {
        stopped(event, this.press.heard);
      }
    }
  };
}

/**
 * Guards every form of page against an OK on a checkbox or a radio submitting it, and notes each OK
 * in a form by note. Returns the stopping.
 */
export function guardFormsOk(page: FormOkPage, note: (line: string) => void): () => void {
  const guard = new FormOkGuard(page, note);
  const listened: readonly (readonly [string, (event: Event) => void])[] = [
    ["keydown", guard.keyDown],
    ["keypress", guard.keyAfter],
    ["keyup", guard.keyAfter],
    ["submit", guard.submitted],
    // A pointer's press is no key's: whatever it submits is its own.
    ["pointerdown", guard.end],
  ];
  for (const [type, listener] of listened) {
    page.target.addEventListener(type, listener, true);
  }
  return () => {
    for (const [type, listener] of listened) {
      page.target.removeEventListener(type, listener, true);
    }
  };
}
