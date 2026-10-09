/**
 * Production-faithful prompt builders for the Payverge model benchmark.
 *
 * WHY THIS EXISTS
 * ---------------
 * The eval used to ship hand-simplified `.txt` copies of the system prompts and
 * — critically — never delivered the user's message to the model (only the
 * guardrails surface did). Every other surface was graded on an *empty*
 * question, so the pass rates measured noise. See git history / the rebuild
 * commit for the full root-cause.
 *
 * This module instead loads the REAL production prompt assets from
 * `backend/internal/...` and reproduces, as closely as a promptfoo prompt
 * function can, how the Go services assemble each request:
 *
 *   - waiter ordering/concierge  -> internal/services/ai.go (buildWaiterSystemPrompt + ChatWithWaiter)
 *   - director                   -> internal/services/director_console_service.go (buildDirectorSystemPrompt + runLoopOrFallback)
 *   - wizard                     -> internal/services/menu_ai_service.go (wizard chat turns)
 *   - guardrails                 -> internal/guardrails/gemini.go (classifierSystemPrompt + buildClassifierMessages)
 *
 * Each exported function returns a promptfoo chat array
 * `[{role:'system',content}, {role:'user',content}]` so the model receives a
 * proper system prompt AND the actual user turn.
 *
 * DELIBERATE DEVIATIONS (documented for honesty — see README):
 *   1. Tools: the waiter omits the `add_to_cart` tool and the director omits its
 *      analytics tool-loop. promptfoo grades conversational TEXT; a tool-call
 *      response has empty text and would corrupt the language/safety/accuracy
 *      rubrics that ARE the benchmark's scorecard. The system prompts (which
 *      drive language/safety/accuracy behavior) are reproduced verbatim.
 *   2. Director runs a single shot over the provided context JSON instead of the
 *      multi-iteration tool loop (promptfoo cannot drive the loop). This is the
 *      first-iteration request exactly as production builds it, which is what we
 *      compare models on (JSON validity + language fidelity + grounded analysis).
 *   3. The data_block spotlighting marker is a fixed constant here; production
 *      uses a per-request random token. The value never affects model behavior.
 */

const fs = require('fs');
const path = require('path');

// backend/ lives three levels up from evals/promptfoo/lib/
const BACKEND = path.resolve(__dirname, '../../../backend');
const WAITER_DIR = path.join(BACKEND, 'internal/services/prompts/ai_waiter');
const DIRECTOR_DIR = path.join(BACKEND, 'internal/services/prompts/director_console');
const WIZARD_DIR = path.join(BACKEND, 'internal/services/prompts/menu_wizard');
const OPS_ASSISTANT_DIR = path.join(BACKEND, 'internal/services/prompts/ops_assistant');
const GUARDRAILS_MD = path.join(BACKEND, 'internal/guardrails/prompts/classifier.md');

// Fixed spotlighting marker — production uses crypto/rand; value is irrelevant.
const MARKER = 'a1b2c3d4';

// LANG var -> { family (prompt file stem suffix), native (LANGUAGE_NATIVE) }.
// Mirrors internal/locales/registry_generated.go NativeName values. Covers all
// 21 guest locales so the tier-2 sweep can exercise every supported language.
const LANGS = {
  en: { family: 'en', native: 'English' },
  es: { family: 'es', native: 'Español' },
  'es-ar': { family: 'es_ar', native: 'Español (Argentina)' },
  es_ar: { family: 'es_ar', native: 'Español (Argentina)' },
  ar: { family: 'ar', native: 'العربية' },
  da: { family: 'da', native: 'Dansk' },
  de: { family: 'de', native: 'Deutsch' },
  fr: { family: 'fr', native: 'Français' },
  hi: { family: 'hi', native: 'हिन्दी' },
  it: { family: 'it', native: 'Italiano' },
  ja: { family: 'ja', native: '日本語' },
  ko: { family: 'ko', native: '한국어' },
  nl: { family: 'nl', native: 'Nederlands' },
  no: { family: 'no', native: 'Norsk' },
  pl: { family: 'pl', native: 'Polski' },
  pt: { family: 'pt', native: 'Português' },
  ru: { family: 'ru', native: 'Русский' },
  sv: { family: 'sv', native: 'Svenska' },
  th: { family: 'th', native: 'ไทย' },
  tr: { family: 'tr', native: 'Türkçe' },
  vi: { family: 'vi', native: 'Tiếng Việt' },
  zh: { family: 'zh', native: '中文' },
};

function resolveLang(code) {
  const key = String(code || 'en').trim().toLowerCase().replace(/_/g, '-');
  return LANGS[key] || LANGS[String(code || '').toLowerCase()] || LANGS.en;
}

// Mirrors waiter_prompts.go parseReviewPending: strips a leading `---`...`---`
// front-matter block and reports whether it contains `review_pending: true`.
function parseReviewPending(raw) {
  const text = String(raw);
  if (!text.startsWith('---')) return { body: text, pending: false };
  const end = text.indexOf('\n---', 3);
  if (end === -1) return { body: text, pending: false };
  const header = text.slice(0, end).toLowerCase();
  const pending = header.includes('review_pending: true');
  // body is everything after the closing `---` line
  const after = text.indexOf('\n', end + 1);
  const body = after === -1 ? '' : text.slice(after + 1);
  return { body: body.replace(/^\s+/, ''), pending };
}

function readPrompt(file) {
  return fs.readFileSync(file, 'utf8');
}

// Mirrors ai.go resolveWaiterPrompt: prefer the requested family's reviewed
// file, else fall back to the English body (keeping the requested NativeName).
function loadWaiterTemplate(mode, family) {
  const tryFamily = (fam) => {
    const f = path.join(WAITER_DIR, `${mode}_${fam}.md`);
    if (!fs.existsSync(f)) return null;
    const { body, pending } = parseReviewPending(readPrompt(f));
    return pending ? null : body;
  };
  return tryFamily(family) || tryFamily('en') || 'You are a restaurant AI waiter.';
}

// Light analogue of services.SanitizePromptField: trim + cap length. The
// benchmark fixtures are already clean, so this is effectively a no-op; it
// exists so injection-laced owner fields can't blow up the template.
function sanitize(s, max) {
  const t = String(s == null ? '' : s).trim();
  return max && t.length > max ? t.slice(0, max) : t;
}

// Mirrors waiter_prompts.go wrapDataBlock (Microsoft spotlighting).
function wrapDataBlock(name, content) {
  return `<data_block name="${name}" marker="${MARKER}">\n${MARKER}\n${content}\n${MARKER}\n</data_block>`;
}

// Mirrors ai.go priorityInstruction (empty -> BALANCED).
function priorityInstruction(p) {
  switch (String(p || '').toLowerCase()) {
    case 'service':
      return 'Your priority is SERVICE: be efficient and brief, take the order correctly, and only suggest pairings when asked.';
    case 'upselling':
      return 'Your priority is UPSELLING: describe dishes enthusiastically and suggest a drink and a side for each main, without being pushy.';
    default:
      return 'Your priority is BALANCED: be helpful and friendly, suggest one pairing per main, and never be pushy.';
  }
}

function replaceAll(template, map) {
  let out = template;
  for (const [k, v] of Object.entries(map)) {
    out = out.split(k).join(v == null ? '' : String(v));
  }
  return out;
}

// ---- Waiter (ordering + concierge) ------------------------------------------
// Mirrors buildWaiterSystemPrompt + ChatWithWaiter message assembly.
function buildWaiter(mode, vars) {
  const lang = resolveLang(vars.LANG || vars.LANGUAGE);
  const tmpl = loadWaiterTemplate(mode, lang.family);

  const aiName = sanitize(vars.AI_NAME, 40) || 'Sage';
  const businessName = sanitize(vars.BUSINESS_NAME, 120) || 'Trattoria Bella Vita';
  const specialInstr = sanitize(vars.SPECIAL_INSTRUCTIONS_BLOCK, 1000);

  const aboutContent =
    vars.ABOUT_BLOCK != null && String(vars.ABOUT_BLOCK).trim() !== ''
      ? `Name: ${businessName}\nDescription: ${vars.ABOUT_BLOCK}${
          vars.BUSINESS_ADDRESS ? `\nAddress: ${vars.BUSINESS_ADDRESS}` : ''
        }`
      : `Name: ${businessName}`;

  const system = replaceAll(tmpl, {
    '{{AI_NAME}}': aiName,
    '{{BUSINESS_NAME}}': businessName,
    '{{LANGUAGE_NATIVE}}': lang.native,
    '{{LANGUAGE_INSTRUCTION}}': `Respond only in ${lang.native}.`,
    '{{PRIORITY_INSTRUCTION}}': priorityInstruction(vars.AI_PRIORITY),
    '{{RESERVATION_CONTEXT}}': vars.RESERVATION_CONTEXT && String(vars.RESERVATION_CONTEXT).trim() !== '' ? wrapDataBlock('reservation', vars.RESERVATION_CONTEXT) : '',
    '{{DELIVERY_CONTEXT}}': vars.DELIVERY_CONTEXT && String(vars.DELIVERY_CONTEXT).trim() !== '' ? wrapDataBlock('delivery', vars.DELIVERY_CONTEXT) : '',
    '{{ABOUT_BLOCK}}': wrapDataBlock('about', aboutContent),
    '{{SPECIAL_INSTRUCTIONS_BLOCK}}': specialInstr ? wrapDataBlock('owner_notes', specialInstr) : '(none)',
    '{{BILL_BLOCK}}': vars.BILL_BLOCK && String(vars.BILL_BLOCK).trim() !== '' ? wrapDataBlock('bill', vars.BILL_BLOCK) : '(empty)',
    '{{MENU_BLOCK}}': wrapDataBlock('menu', vars.MENU_BLOCK || ''),
    '{{OFFERS_BLOCK}}': wrapDataBlock('offers', vars.OFFERS_BLOCK || ''),
    '{{BUNDLES_BLOCK}}': wrapDataBlock('bundles', vars.BUNDLES_BLOCK || ''),
  });

  // ChatWithWaiter appends a trailing language anchor to the last user turn.
  const userText = `${vars.guest_message || ''}\n\n(Respond only in ${lang.native}.)`;

  return [
    { role: 'system', content: system },
    { role: 'user', content: userText },
  ];
}

function waiterOrdering(context) {
  return buildWaiter('ordering', context.vars || {});
}

function waiterConcierge(context) {
  return buildWaiter('concierge', context.vars || {});
}

// ---- Director ----------------------------------------------------------------
// Mirrors buildDirectorSystemPrompt + runLoopOrFallback's first user turn.
function directorLanguageInstruction(family, native) {
  if (family === 'es_ar') return `Respond in ${native} with Argentine Spanish phrasing for restaurant operators.`;
  if (family === 'es') return `Respond in ${native}, using clear, professional Spanish.`;
  return `Respond in ${native}.`;
}

function directorUserLanguageAnchor(family, native) {
  if (family === 'es_ar') return `Respondé únicamente en ${native}. Todos los valores de texto del JSON deben estar en ${native}.`;
  if (family === 'es') return `Responde únicamente en ${native}. Todos los valores de texto del JSON deben estar en ${native}.`;
  return `Respond only in ${native}. Every JSON string value must be in ${native}.`;
}

function director(context) {
  const vars = context.vars || {};
  const lang = resolveLang(vars.LANG || vars.LANGUAGE);
  const family = lang.family === 'ja' || lang.family === 'ar' ? 'en' : lang.family; // director ships en/es/es_ar only
  const native = lang.native;
  const businessId = vars.BUSINESS_ID || '1';
  const assistantName = sanitize(vars.ASSISTANT_NAME, 40) || 'Sage';

  const file = path.join(DIRECTOR_DIR, `${family}.md`);
  const tmpl = fs.existsSync(file) ? readPrompt(file) : readPrompt(path.join(DIRECTOR_DIR, 'en.md'));

  const system = replaceAll(tmpl.trim(), {
    '{{ASSISTANT_NAME}}': assistantName,
    '{{BUSINESS_ID}}': businessId,
    '{{LANGUAGE_INSTRUCTION}}': directorLanguageInstruction(family, native),
  });

  const contextJson = vars.context_json || '{}';
  const anchor = directorUserLanguageAnchor(family, native);
  const userText = `Owner question:\n${String(vars.owner_question || '').trim()}\n\nBusiness context JSON:\n${contextJson}\n\n${anchor}`;

  return [
    { role: 'system', content: system },
    { role: 'user', content: userText },
  ];
}

// ---- Wizard ------------------------------------------------------------------
// Mirrors menu_ai_service.go wizard chat (static system prompt + user turns).
function wizard(context) {
  const vars = context.vars || {};
  const lang = resolveLang(vars.LANG || vars.LANGUAGE);
  const family = lang.family === 'ja' || lang.family === 'ar' ? 'en' : lang.family; // wizard ships en/es/es_ar only

  const file = path.join(WIZARD_DIR, `${family}.md`);
  const system = (fs.existsSync(file) ? readPrompt(file) : readPrompt(path.join(WIZARD_DIR, 'en.md'))).trim();

  const userText = String(vars.user_message || '').trim() || 'Hello, I want to create a menu for my restaurant.';

  return [
    { role: 'system', content: system },
    { role: 'user', content: userText },
  ];
}

// ---- Guardrails --------------------------------------------------------------
// Mirrors gemini.go classifierSystemPrompt() (md as-is, placeholders unfilled)
// + buildClassifierMessages (surface/language_hint/message in the user turn).
function guardrails(context) {
  const vars = context.vars || {};
  const system = readPrompt(GUARDRAILS_MD).trim();
  const lang = resolveLang(vars.LANG || vars.LANGUAGE || 'en');
  const hint = vars.LANGUAGE_HINT || lang.native;
  const body = `surface: ${vars.SURFACE || 'ai_waiter'}\nlanguage_hint: ${hint}\nmessage:\n${vars.MESSAGE || ''}`;
  return [
    { role: 'system', content: system },
    { role: 'user', content: body },
  ];
}

function opsAssistant(context) {
  const vars = context.vars || {};
  const lang = resolveLang(vars.LANG || vars.LANGUAGE);
  const family = lang.family;
  const file = path.join(OPS_ASSISTANT_DIR, `${family}.md`);
  const system = (fs.existsSync(file) ? readPrompt(file) : readPrompt(path.join(OPS_ASSISTANT_DIR, 'en.md'))).trim();
  const tab = vars.active_tab || 'overview';
  const userText = String(vars.user_message || vars.MESSAGE || '').trim() || 'How do I add a menu item?';
  return [
    { role: 'system', content: `${system}\n\nActive tab: ${tab}. Business id: ${vars.business_id || '1'}.` },
    { role: 'user', content: userText },
  ];
}

module.exports = {
  waiterOrdering,
  waiterConcierge,
  director,
  wizard,
  guardrails,
  opsAssistant,
  // exported for unit-testing the builders without promptfoo
  _internal: { resolveLang, parseReviewPending, loadWaiterTemplate, buildWaiter },
};
