// SPDX-License-Identifier: MPL-2.0

/*
 * Says a browser is too old for the client, where the client's own script would leave a blank page:
 * the first beta tester's TV ran Chrome 67, which cannot read the client's script, built for ES2023
 * syntax, and showed nothing at all (the install report, 2026-11-19). The client supports Chrome and
 * Edge 120, Firefox 140 and Safari 17.1 (D271).
 *
 * This file is a classic script in ES5 alone, kept out of the build, so every browser that loads the
 * page reads it; it is a file, not inline, for the page's policy runs no inline script. It asks for
 * two things a browser that reads ES2023 syntax has: Array's findLast (Chrome 97, Firefox 104,
 * Safari 15.4) and String's isWellFormed (Chrome 111, Firefox 119, Safari 16.4, the version that
 * first reads class static blocks). A browser with both reads the client's script; one without is
 * older than D271's floor, and is told so in the page's root, written as text alone.
 */
/* eslint-disable no-var -- ES5 on purpose: the browsers this is for read no let or const. */
(function (window, document) {
  "use strict";
  var tooOld =
    "This browser is too old for Orpheum. Update it, or use another browser: Chrome, Edge, Firefox or Safari from the last two years.";
  var arrays = window.Array && window.Array.prototype;
  var strings = window.String && window.String.prototype;
  if (
    arrays &&
    typeof arrays.findLast === "function" &&
    strings &&
    typeof strings.isWellFormed === "function"
  ) {
    return;
  }
  var root = document.getElementById("root");
  if (!root) {
    return;
  }
  while (root.firstChild) {
    root.removeChild(root.firstChild);
  }
  var note = document.createElement("p");
  note.appendChild(document.createTextNode(tooOld));
  root.appendChild(note);
})(window, document);
/* eslint-enable no-var -- ES5 ends with the file. */
