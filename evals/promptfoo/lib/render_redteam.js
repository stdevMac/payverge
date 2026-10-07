#!/usr/bin/env node
/**
 * Renders adversarial red-team cases (from the prompt-redteam-gen workflow) into
 * production-faithful promptfoo configs: evals/promptfoo/configs/redteam-<surface>.yaml.
 *
 * WHY A RENDERER: the cases are authored by many parallel agents as structured
 * JSON. Hand-writing YAML for arbitrary injection payloads (quotes, newlines,
 * unicode, zero-width chars) is error-prone, so each test is emitted as a JSON
 * flow-mapping — which is valid YAML and lets JSON.stringify handle all escaping.
 * The header (providers, grader, production fixtures) is hand-written YAML so the
 * configs stay readable and mirror the existing per-surface configs.
 *
 * Usage:  node lib/render_redteam.js <cases.json>
 *   cases.json is either an array of cases or { cases: [...] }.
 *   Each case: { surface, lens, label, vars, asserts:[{type,value}], expect_safe, break_hypothesis }
 */
const fs = require('fs');
const path = require('path');

const CASES_FILE = process.argv[2];
if (!CASES_FILE) {
  console.error('usage: node lib/render_redteam.js <cases.json>');
  process.exit(1);
}
const OUT_DIR = path.resolve(__dirname, '../configs');
const PREFIX = process.argv[3] || 'redteam'; // output filename prefix: <prefix>-<surface>.yaml
const raw = JSON.parse(fs.readFileSync(CASES_FILE, 'utf8'));
const list = Array.isArray(raw) ? raw : raw.cases || [];

// ---- production fixtures (copied from the existing per-surface configs) -------
const MENU_FIXTURE = [
  'Pizzas:',
  '- Margherita: San Marzano tomatoes, fresh mozzarella, basil ($14.00) allergens: gluten, dairy dietary: vegetarian',
  '- Pepperoni: pepperoni, mozzarella, tomato sauce ($16.00) allergens: gluten, dairy dietary: none',
  '- Quattro Formaggi: gorgonzola, mozzarella, parmesan, fontina ($17.50) allergens: gluten, dairy dietary: vegetarian',
  '- Pizza Vegetariana: grilled eggplant, zucchini, bell peppers, mushrooms, mozzarella ($15.50) allergens: gluten, dairy dietary: vegetarian, vegan-option',
  '- Diavola: spicy salami, chili flakes, mozzarella ($16.50) allergens: gluten, dairy dietary: none',
  '- Prosciutto e Funghi: Parma ham, mushrooms, truffle oil ($18.00) allergens: gluten, dairy dietary: none',
  '- Margherita Senza Glutine: gluten-free crust, tomatoes, mozzarella, basil ($16.00) allergens: dairy dietary: vegetarian, gluten-free',
  'Appetizers:',
  '- Bruschetta Classica: toasted ciabatta, tomatoes, basil, olive oil ($9.50) allergens: gluten dietary: vegetarian, vegan',
  '- Calamari Fritti: crispy fried squid rings, marinara, lemon aioli ($12.00) allergens: gluten, shellfish dietary: none',
  '- Caprese Salad: vine-ripened tomatoes, fresh mozzarella, basil, balsamic ($10.50) allergens: dairy dietary: vegetarian, gluten-free',
  '- Minestrone Soup: seasonal vegetables, beans, pasta, parmesan ($8.00) allergens: gluten, dairy dietary: vegetarian',
  'Desserts:',
  '- Tiramisu: espresso-soaked ladyfingers, mascarpone, cocoa ($9.00) allergens: gluten, dairy, eggs dietary: vegetarian',
  '- Panna Cotta: vanilla cream, berry compote ($8.50) allergens: dairy dietary: vegetarian, gluten-free',
  '- Sorbetto al Limone: lemon sorbet, fresh mint ($6.50) allergens: none dietary: vegan, gluten-free',
];

const DIRECTOR_CONTEXT = JSON.stringify({
  business_name: 'Trattoria Bella Vita',
  currency: 'USD',
  weekly_metrics: { revenue: 28450.75, transactions: 1247, average_ticket: 22.8, tips: 4267.61, active_bills: 8 },
  order_status: { pending: 3, confirmed: 12, preparing: 5, ready: 2, delivered: 1203 },
  reservations: { total_this_week: 87, average_party_size: 3.2, peak_times: ['19:00', '20:00', '20:30'], no_shows: 4 },
  top_items: [
    { name: 'Margherita', orders: 245, revenue: 3430.0, margin: '68%' },
    { name: 'Pepperoni', orders: 198, revenue: 3168.0, margin: '62%' },
    { name: 'Caprese Salad', orders: 156, revenue: 1638.0, margin: '72%' },
    { name: 'Diavola', orders: 142, revenue: 2343.0, margin: '58%' },
    { name: 'Tiramisu', orders: 189, revenue: 1701.0, margin: '75%' },
  ],
  bottom_items: [
    { name: 'Minestrone Soup', orders: 42, revenue: 336.0 },
    { name: 'Sorbetto al Limone', orders: 38, revenue: 247.0 },
  ],
  delivery_metrics: { total_deliveries: 234, avg_delivery_time_min: 28, customer_satisfaction: '4.3/5' },
  staff: { total: 12, servers: 6, kitchen: 4, management: 2 },
  crm_stats: { total_customers: 1240, new_this_week: 47, returning_rate: '62%', avg_visits_per_month: 1.8 },
  plugins: ['Stripe', 'Postmark', 'MercadoPago'],
  available_plugins: ['Trustpilot', 'Mailchimp', 'QuickBooks', 'OpenTable', 'UberEats'],
  previous_week_revenue: 31200.5,
  marketing_spend: 1200.0,
});

// indent a multi-line string for a YAML block scalar at the given base indent
function blockScalar(key, lines, baseIndent) {
  const pad = ' '.repeat(baseIndent);
  const inner = lines.map((l) => `${pad}  ${l}`).join('\n');
  return `${pad}${key}: |\n${inner}`;
}

function waiterHeader(fn) {
  return `# AUTO-GENERATED red-team suite — do not hand-edit the tests block.
# Regenerate: node evals/promptfoo/lib/render_redteam.js <cases.json>
# Source prompt: backend/internal/services/prompts/ai_waiter/. Production model gemini-2.5-flash (temp 0.4 / 1024).
description: "Red-team — ${fn} (injection, allergen edges, boundaries, fabrication, language fidelity)"

prompts:
  - file://../lib/prod_prompts.js:${fn}

providers:
  - id: openrouter:google/gemini-2.5-flash
    label: gemini-2.5-flash
    config: { temperature: 0.4, max_tokens: 1024 }

defaultTest:
  options:
    provider: openrouter:google/gemini-2.5-flash-lite
  vars:
    AI_NAME: "Sage"
    BUSINESS_NAME: "Trattoria Bella Vita"
${blockScalar('MENU_BLOCK', MENU_FIXTURE, 4)}
    SPECIAL_INSTRUCTIONS_BLOCK: "The chef recommends the Prosciutto e Funghi pizza tonight. Our house wine is a 2022 Chianti Classico."
    BILL_BLOCK: ""
    OFFERS_BLOCK: "Happy Hour: 20% off all appetizers 4-6 PM Monday through Friday."
    BUNDLES_BLOCK: "Date Night Bundle: 2 pizzas + 2 desserts + bottle of house wine for $55."
    ABOUT_BLOCK: "Family-owned Italian restaurant since 1998. Open Mon-Sat 11AM-10PM, Sun 12PM-9PM. Reservations recommended for weekends. Free parking available."
    RESERVATION_CONTEXT: "Reservations available online or by phone. Walk-ins welcome."
    DELIVERY_CONTEXT: "Delivery available within 5km. $5 fee, free over $40."
`;
}

function directorHeader() {
  return `# AUTO-GENERATED red-team suite — do not hand-edit the tests block.
# Regenerate: node evals/promptfoo/lib/render_redteam.js <cases.json>
# Source prompt: backend/internal/services/prompts/director_console/. Production model gemini-2.5-flash (temp 0.2 / 4096 JSON).
description: "Red-team — director (data-poisoning, PII leak, fabrication, JSON escape, deep-link invention)"

prompts:
  - file://../lib/prod_prompts.js:director

providers:
  - id: openrouter:google/gemini-2.5-flash
    label: gemini-2.5-flash
    config:
      temperature: 0.2
      max_tokens: 4096
      response_format: { type: json_object }

defaultTest:
  options:
    provider: openrouter:google/gemini-2.5-flash-lite
  vars:
    ASSISTANT_NAME: "Director Copilot"
    BUSINESS_ID: "1"
    context_json: ${JSON.stringify(DIRECTOR_CONTEXT)}
`;
}

function wizardHeader() {
  return `# AUTO-GENERATED red-team suite — do not hand-edit the tests block.
# Regenerate: node evals/promptfoo/lib/render_redteam.js <cases.json>
# Source prompt: backend/internal/services/prompts/menu_wizard/. Production model gemini-2.5-flash (temp 0.6 / 4096 JSON).
description: "Red-team — wizard (injection, prompt leak, schema escape, premature completion, language)"

prompts:
  - file://../lib/prod_prompts.js:wizard

providers:
  - id: openrouter:google/gemini-2.5-flash
    label: gemini-2.5-flash
    config:
      temperature: 0.6
      max_tokens: 4096
      response_format: { type: json_object }

defaultTest:
  options:
    provider: openrouter:google/gemini-2.5-flash-lite
`;
}

function guardrailsHeader() {
  return `# AUTO-GENERATED red-team suite — do not hand-edit the tests block.
# Regenerate: node evals/promptfoo/lib/render_redteam.js <cases.json>
# Source prompt: backend/internal/guardrails/prompts/classifier.md. Production model gemini-2.5-flash-lite (temp 0 / 80 JSON).
description: "Red-team — guardrails (obfuscated injection, multilingual abuse, false-positive guard)"

prompts:
  - file://../lib/prod_prompts.js:guardrails

providers:
  - id: openrouter:google/gemini-2.5-flash-lite
    label: gemini-2.5-flash-lite
    config:
      temperature: 0
      max_tokens: 80
      response_format: { type: json_object }

defaultTest:
  options:
    provider: openrouter:google/gemini-2.5-flash-lite
  vars:
    SURFACE: ai_waiter
`;
}

const HEADERS = {
  'waiter-ordering': () => waiterHeader('waiterOrdering'),
  'waiter-concierge': () => waiterHeader('waiterConcierge'),
  director: directorHeader,
  wizard: wizardHeader,
  guardrails: guardrailsHeader,
};

// promptfoo's JS handler (evaluator handleJavascript): a value containing a NEWLINE
// is used VERBATIM as the function body; a single-line value goes through a fragile
// statement/expression splitter that mis-handles an internal `return` ("Unexpected
// token 'return'" or a silent undefined). We emit tests as one-line JSON, so any
// statement-body JS (own `return`, or a `const/let/var` declaration) must be made
// multi-line (prepend "\n") to force the reliable verbatim-body path — exactly how
// the working block-scalar configs run. Pure single expressions are left as-is so
// promptfoo wraps them in `return (<value>)`.
function fixJs(value) {
  const v = String(value).trim();
  if (/\breturn\b/.test(v) || /^(const|let|var)\s/.test(v)) {
    return v.includes('\n') ? v : '\n' + v;
  }
  return v;
}

// llm-rubric must be the LAST assertion (it needs the grader and is the strongest check)
function orderAsserts(asserts) {
  const a = (asserts || []).map((x) => ({
    type: x.type,
    value: x.type === 'javascript' ? fixJs(x.value) : x.value,
  }));
  return a.sort((x, y) => (x.type === 'llm-rubric' ? 1 : 0) - (y.type === 'llm-rubric' ? 1 : 0));
}

function renderTest(c) {
  const test = {
    description: `[${c.lens}] ${c.label}`,
    vars: c.vars || {},
    assert: orderAsserts(c.asserts),
  };
  return '  - ' + JSON.stringify(test);
}

const SURFACES = ['waiter-ordering', 'waiter-concierge', 'director', 'wizard', 'guardrails'];
const groups = {};
for (const c of list) (groups[c.surface] = groups[c.surface] || []).push(c);

let total = 0;
for (const s of SURFACES) {
  const tests = groups[s] || [];
  if (!tests.length) {
    console.log(`redteam-${s}.yaml: 0 cases (skipped)`);
    continue;
  }
  const body = HEADERS[s]() + '\ntests:\n' + tests.map(renderTest).join('\n') + '\n';
  fs.writeFileSync(path.join(OUT_DIR, `${PREFIX}-${s}.yaml`), body);
  console.log(`${PREFIX}-${s}.yaml: ${tests.length} cases`);
  total += tests.length;
}
console.log(`TOTAL: ${total} red-team cases across ${SURFACES.filter((s) => (groups[s] || []).length).length} surfaces`);
