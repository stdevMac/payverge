// Given the page origin the operator is on, derive the backend API base.
// Payverge serves the frontend and backend on different origins:
//   dev:  localhost:3000 (app)  -> localhost:8080 (api)
//   prod: payverge.io   (app)  -> api.payverge.io (api)
// Callers can override this entirely via `defaults.apiBase` in shots.js.
export function deriveApiBase(pageOrigin) {
  const { protocol, hostname } = new URL(pageOrigin);
  if (hostname === "localhost" || hostname === "127.0.0.1") {
    return "http://localhost:8080";
  }
  if (hostname.startsWith("api.")) {
    return `${protocol}//${hostname}`;
  }
  const apex = hostname.replace(/^www\./, "");
  return `${protocol}//api.${apex}`;
}
