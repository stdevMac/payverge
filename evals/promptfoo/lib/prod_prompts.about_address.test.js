/**
 * Offline regression check for the waiter about-block (L10).
 *
 * Production (backend/internal/services/ai.go:483-484) renders the business
 * about-block as "Name: <n>\nDescription: <d>\nAddress: <a>". The eval prompt
 * builder previously dropped the Address line, so the benchmark graded a prompt
 * the model never sees in production. This check asserts the builder restores
 * Address when BUSINESS_ADDRESS is present, and omits it (no dangling
 * "Address:") when it is not.
 *
 * Pure-Node (no test framework / network): run with `node prod_prompts.about_address.test.js`.
 */
const assert = require('assert');
const m = require('./prod_prompts.js');

function systemFor(builder, vars) {
  const out = builder({ vars });
  return out.find((x) => x.role === 'system').content;
}

// 1. address + about present -> address line matches the production shape.
{
  const sys = systemFor(m.waiterConcierge, {
    LANG: 'en',
    BUSINESS_NAME: 'Trattoria Bella Vita',
    ABOUT_BLOCK: 'Cozy Italian spot.',
    BUSINESS_ADDRESS: '123 Main St, Springfield',
    guest_message: 'Where are you located?',
  });
  assert.ok(
    sys.includes('Name: Trattoria Bella Vita\nDescription: Cozy Italian spot.\nAddress: 123 Main St, Springfield'),
    'concierge about-block must carry Name/Description/Address',
  );
}

// 2. ordering surface also carries the address.
{
  const sys = systemFor(m.waiterOrdering, {
    LANG: 'en',
    BUSINESS_NAME: 'Bella',
    ABOUT_BLOCK: 'Cozy spot.',
    BUSINESS_ADDRESS: '9 Elm Ave',
    guest_message: 'hi',
  });
  assert.ok(sys.includes('Address: 9 Elm Ave'), 'ordering about-block must carry the address');
}

// 3. no BUSINESS_ADDRESS -> no Address line (no dangling label).
{
  const sys = systemFor(m.waiterConcierge, {
    LANG: 'en',
    BUSINESS_NAME: 'Bella',
    ABOUT_BLOCK: 'Cozy spot.',
    guest_message: 'hi',
  });
  assert.ok(!sys.includes('Address:'), 'absent BUSINESS_ADDRESS must not emit an Address line');
}

// 4. empty about + address still falls through to Name-only (matches prior behavior).
{
  const sys = systemFor(m.waiterConcierge, {
    LANG: 'en',
    BUSINESS_NAME: 'Bella',
    BUSINESS_ADDRESS: '9 Elm Ave',
    guest_message: 'hi',
  });
  assert.ok(!sys.includes('Address:'), 'empty about-block stays Name-only');
}

console.log('prod_prompts about-block address check: PASS');
