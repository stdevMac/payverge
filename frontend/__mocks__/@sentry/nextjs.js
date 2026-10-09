// Jest manual mock for @sentry/nextjs.
//
// The real module eagerly runs pages-router routing instrumentation at import
// time, which reads `Router.events` on a Router singleton that is undefined
// under jsdom — throwing before any test runs. errorLogger.ts imports it and is
// transitively pulled in by most business components, so the real module
// silently masked ~14 suites behind a green run.
//
// A Proxy returns a no-op callable for every accessed export (captureException,
// init, withScope, etc.), so any current or future Sentry call is a harmless
// no-op in tests with zero production impact.
const noop = () => {};

module.exports = new Proxy(
  { __esModule: true },
  {
    get(target, prop) {
      if (prop in target) {
        return target[prop];
      }
      return noop;
    },
  },
);
