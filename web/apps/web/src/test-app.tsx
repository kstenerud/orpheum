// SPDX-License-Identifier: MPL-2.0

import { App } from "./app.tsx";
import { mount, settleUntil } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/**
 * The markup of the app at path, signed in as a person: the page knows who is signed in before it
 * draws (D469 §4), so GET /me is answered, as a person's, and nothing else is, as a prerender
 * answers nothing. Taken once the page stops changing, its screen's code loaded.
 */
export async function signedInMarkupAt(path: string): Promise<string> {
  const send = async (request: Request): Promise<Response> => {
    if (request.method === "GET" && new URL(request.url).pathname.endsWith("/me")) {
      return new Response(JSON.stringify({ id: 1, name: "owner", role: "user", isAdmin: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return await new Promise<Response>((_resolve, reject) => {
      request.signal.addEventListener("abort", () => {
        reject(new DOMException("aborted", "AbortError"));
      });
    });
  };
  const mounted = await mount(withServices(<App />, testServices("token", path, send)));
  const { container } = mounted;
  await settleUntil(() => container.querySelector(".app-shell") !== null);
  // Unchanged for longer than React holds back a Suspense boundary's reveal, about 300 ms, so a
  // screen whose code loads lazily is drawn in its fallback's place.
  let markup = "";
  let changedAt = Date.now();
  await settleUntil(() => {
    if (container.innerHTML !== markup) {
      markup = container.innerHTML;
      changedAt = Date.now();
    }
    return Date.now() - changedAt >= 600;
  });
  mounted.unmount();
  return markup;
}
