// Empty polyfill stub. Next 14 imports next/dist/build/polyfills/polyfill-module.js
// into the main client bundle (~11 KiB of Array.at/flat/flatMap/Object.fromEntries
// /Object.hasOwn/String.trim* shims). Our supported browser floor (iOS 14 +
// evergreen Chrome/Firefox/Safari/Edge 90+) ships every one of these natively,
// so the shim is dead weight on the LCP critical path. NormalModuleReplacement
// in next.config.mjs swaps the original file with this stub.
module.exports = {};
