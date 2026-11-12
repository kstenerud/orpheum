// SPDX-License-Identifier: MPL-2.0

/**
 * Gives a browser test's frame its focus back when another page takes it. In Firefox (Playwright's,
 * headless) a page opening takes the focus from the frame of the page opened before it: its focused
 * element stops matching `:focus` and `:focus-visible`, and an Enter on it clicks nothing, while
 * `document.activeElement` still names it. Vitest opens pages while tests run (when one browser's
 * files end it gives their pages to the other), so a test there read its focused tile as unfocused
 * (idiosyncrasies.md, "A page opening in Firefox takes the focus from the test frame opened before
 * it"). Self-contained, so its test can hand its source to a page of its own.
 */
export function keepTheFrameFocused(view: Window): void {
  view.addEventListener("blur", (event) => {
    if (event.target === view) {
      view.focus();
    }
  });
}
