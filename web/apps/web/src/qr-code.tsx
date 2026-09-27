// SPDX-License-Identifier: MPL-2.0

import qrcode from "qrcode-generator";
import type { JSX } from "react";

/** The quiet zone around a QR code, in modules, as the standard asks. */
const quietModules = 4;

/** The dark modules of text's QR code as one SVG path, and the code's size in modules. */
export function qrPath(text: string): { path: string; modules: number } {
  const code = qrcode(0, "M");
  code.addData(text);
  code.make();
  const modules = code.getModuleCount();
  const squares: string[] = [];
  for (let row = 0; row < modules; row++) {
    for (let column = 0; column < modules; column++) {
      if (code.isDark(row, column)) {
        squares.push(`M${String(column + quietModules)} ${String(row + quietModules)}h1v1h-1z`);
      }
    }
  }
  return { path: squares.join(""), modules };
}

/** Text's QR code as an SVG image's address: dark modules on light, with the quiet zone. */
export function qrImage(text: string): string {
  const { path, modules } = qrPath(text);
  const size = String(modules + 2 * quietModules);
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${size} ${size}" shape-rendering="crispEdges">` +
    `<rect width="${size}" height="${size}" fill="#fff"/><path d="${path}" fill="#000"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

/** Text as a QR code a phone's camera opens (D218 §1). */
export function QrCode(props: { readonly text: string; readonly label: string }): JSX.Element {
  return <img className="qr-code" src={qrImage(props.text)} alt={props.label} />;
}
