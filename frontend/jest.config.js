const customJestConfig = {
  setupFilesAfterEnv: ['<rootDir>/jest.setup.js'],
  testEnvironment: 'node',
  // Jest defaults to (CPU cores - 1) workers. On an 8-core dev box that is 7
  // concurrent worker processes, and several suites here do a full RTL render of
  // heavy NextUI <Tabs> trees (react-aria/react-stately + framer-motion, all
  // runtime-Babel-transformed via transformIgnorePatterns below). When 5+ of
  // those heavy suites get co-scheduled, the combined CPU/memory footprint
  // starves the event loop so React's act()/waitFor polling blows past the
  // wall-clock deadline — producing a flaky "Exceeded timeout" / "Unable to
  // find" cascade across whichever heavy suites happened to land together
  // (AiWaiterDashboard, BusinessPageEditor, ReservationManager, BillManager,
  // …). Cap at 2 workers (same as CI shards' --maxWorkers=2): 50% still
  // oversubscribed this suite under full-run load. Pair with asyncUtilTimeout
  // 5s in jest.setup.js and testTimeout below.
  maxWorkers: 2,
  // Default 5s is too tight once waitFor/findBy share a contended worker with
  // a 5s asyncUtilTimeout. 15s fails real hangs without masking them as "pass
  // after a minute".
  testTimeout: 15000,
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
    '^canvas$': '<rootDir>/test/mocks/canvas.js',
    // @sentry/nextjs throws at import time under jsdom (pages-router
    // instrumentation reads an undefined Router.events). No-op it in tests so
    // the ~14 suites that transitively import errorLogger.ts actually run.
    '^@sentry/nextjs$': '<rootDir>/__mocks__/@sentry/nextjs.js',
  },
  transform: {
    '^.+\\.(js|jsx|ts|tsx)$': ['babel-jest', { presets: ['next/babel'] }],
  },
  // wagmi and its dependency chain (viem, ox, abitype, isows, @noble, @scure,
  // @wagmi) ship ESM-only builds with bare `export` syntax that Node's require
  // path can't parse under jsdom/CJS jest. Whitelist the whole chain for
  // Babel transformation so suites that import wagmi hooks (BillsTable, ...)
  // actually parse instead of throwing "Unexpected token 'export'".
  transformIgnorePatterns: [
    '/node_modules/(?!(framer-motion|@nextui-org|@react-aria|@react-stately|wagmi|@wagmi|viem|isows|ox|abitype|@noble|@scure|' +
      'react-markdown|remark(?:-[^/]+)?|rehype(?:-[^/]+)?|unified|vfile(?:-[^/]+)?|unist(?:-[^/]+)?|mdast(?:-[^/]+)?|micromark(?:-[^/]+)?|' +
      '@ungap/structured-clone|bail|ccount|character-entities(?:-[^/]+)?|character-reference-invalid|comma-separated-tokens|decode-named-character-reference|devlop|' +
      'escape-string-regexp|estree-util-is-identifier-name|hast-util-to-jsx-runtime|hast-util-whitespace|html-url-attributes|is-alphabetical|is-alphanumerical|is-decimal|is-hexadecimal|' +
      'is-plain-obj|longest-streak|markdown-table|parse-entities|property-information|space-separated-tokens|stringify-entities|style-to-js|trim-lines|trough|zwitch)(?:/|$))',
  ],
  testPathIgnorePatterns: [
    '/node_modules/',
    '/.next/',
    '/.next/standalone/',
    '/tests/',
    // Underscore-prefixed files inside __tests__ are helpers, not test suites.
    '/__tests__/_',
  ],
  modulePathIgnorePatterns: ['<rootDir>/.next/standalone/'],
  // Jest's DEFAULT coverage provider ('babel') is unusable in this repo, which
  // is why --coverage used to make every suite fail to run. Chain:
  //   babel-jest -> babel-plugin-istanbul@7 -> test-exclude@6
  // test-exclude@6 declares `glob: ^7.1.4` and its module body runs
  // `promisify(require('glob'))`, which is only valid for glob@7 where the
  // export is a callback-style function. But package.json carries a *global*
  // npm override, `"overrides": { "glob": "^13.0.6" }`, which forces glob@13
  // on every consumer in the tree including test-exclude. glob@13 exports an
  // object, so promisify throws
  //   TypeError: The "original" argument must be of type function.
  //              Received an instance of Object
  // at transform time for every file — so all ~1017 suites die the moment
  // --coverage is passed. It is the override, not the machine.
  //
  // The v8 provider reads coverage from V8 directly and never loads istanbul's
  // instrumenter, so it sidesteps the broken chain entirely. This setting only
  // takes effect when coverage is collected; plain `npx jest` is unaffected.
  //
  // The deeper fix (needs package.json + a reinstall, so not done here) is to
  // stop the global glob override reaching test-exclude, e.g. by adding
  // `"test-exclude": "^7.0.1"` to overrides — v7 uses glob's named export and
  // is glob@13-compatible. Until then, keep this on.
  coverageProvider: 'v8',
  collectCoverageFrom: [
    'src/**/*.{js,jsx,ts,tsx}',
    '!src/**/*.d.ts',
    '!src/test/**',
    '!**/node_modules/**',
  ],
  // These used to read 80/80/80/80 and were pure decoration: CI ran jest
  // without --coverage, so the thresholds never evaluated, and --coverage could
  // not be passed at all (see coverageProvider above). A gate that cannot fail
  // is worse than no gate — it reads as enforcement while enforcing nothing.
  //
  // CI now passes --coverage, so these are live. They are set to the MEASURED
  // baseline, not an aspiration, because a threshold nobody meets just gets
  // lowered by whoever is in a hurry. Full-suite measurement, v8 provider,
  // 1011/1017 suites passing (the 4 failures were unrelated in-flight work,
  // which depresses these slightly — the true figure is a little higher):
  //
  //   statements 75.42%  (176309/233767)
  //   branches   75.16%  (19400/25811)
  //   functions  61.92%  (3919/6329)
  //   lines      75.42%  (176309/233767)
  //
  // Each floor is measured-minus-~1pt. That buffer absorbs the depressed
  // baseline and V8-version differences between a dev box and CI's Node 22,
  // while still failing on any real regression (1pt is ~2.3k statements).
  // Ratchet upward as coverage improves; do not lower without saying why.
  coverageThreshold: {
    global: {
      branches: 74,
      functions: 60,
      lines: 74,
      statements: 74,
    },
  },
};

module.exports = customJestConfig;
