import fs from 'fs';
import path from 'path';

import { storefrontLocales } from '../localeRegistry';
import { GUEST_SUPPORTED_LANGUAGES } from '../GuestTranslationProvider';

const GUEST_MESSAGES_DIR = path.join(process.cwd(), 'src/i18n/guest-messages');

describe('guest locale files use ICU interpolation syntax', () => {
  const files = fs.readdirSync(GUEST_MESSAGES_DIR).filter((f) => f.endsWith('.json'));

  test.each(files)('%s has no Mustache-style {{var}} placeholders', (file) => {
    const raw = fs.readFileSync(path.join(GUEST_MESSAGES_DIR, file), 'utf-8');
    const matches = raw.match(/\{\{[a-zA-Z_][a-zA-Z0-9_]*\}\}/g);
    expect(matches).toBeNull();
  });
});

// Regression pin for the 2026-05-14 backend regression that conflated
// operator locales (en/es/es-AR) with menu-translation-target locales (15+).
// Every locale that declares `guestStorefront` in the canonical registry
// must ship a matching JSON bundle, and the runtime dropdown must surface
// the full set.
describe('guest-messages directory matches the canonical locale registry', () => {
  test('every storefront locale has a JSON bundle on disk', () => {
    for (const code of storefrontLocales) {
      const file = path.join(GUEST_MESSAGES_DIR, `${code}.json`);
      expect(fs.existsSync(file)).toBe(true);
    }
  });

  test('GUEST_SUPPORTED_LANGUAGES is the full storefront set', () => {
    const keys = Object.keys(GUEST_SUPPORTED_LANGUAGES).sort();
    const expected = [...storefrontLocales].sort();
    expect(keys).toEqual(expected);
  });

  test('all 21 historical guest languages are exposed', () => {
    // Pre-regression production seeded 21 guest locales. The 2026-05-23 fix
    // restored 16 of them via the registry; the 2026-05-23 follow-up shipped
    // bundles for the remaining 5 historical codes (vi/pl/sv/da/no) plus
    // es-AR's storefront bundle.
    const expected = [
      'ar', 'da', 'de', 'en', 'es', 'es-AR', 'fr', 'hi', 'it',
      'ja', 'ko', 'nl', 'no', 'pl', 'pt', 'ru', 'sv', 'th', 'tr',
      'vi', 'zh',
    ];
    for (const code of expected) {
      expect(GUEST_SUPPORTED_LANGUAGES).toHaveProperty(code);
    }
  });
});

// Argentine (es-AR) guest bundle quality, 2026-05-29. The guest provider loads
// ONE JSON per locale and falls missing keys back to English (not es), so es-AR
// must be a COMPLETE bundle, and it must read as genuine Rioplatense — not the
// Peninsular copy it used to be.
describe('guest es-AR is a complete, genuinely Argentine bundle', () => {
  type Tree = Record<string, unknown>;
  const load = (code: string): Tree =>
    JSON.parse(
      fs.readFileSync(path.join(GUEST_MESSAGES_DIR, `${code}.json`), 'utf-8'),
    );
  const flatten = (o: unknown, prefix = '', out: Record<string, string> = {}) => {
    if (o && typeof o === 'object' && !Array.isArray(o)) {
      for (const [k, v] of Object.entries(o as Tree)) {
        flatten(v, prefix ? `${prefix}.${k}` : k, out);
      }
    } else if (typeof o === 'string') {
      out[prefix] = o;
    }
    return out;
  };

  const es = flatten(load('es'));
  const esAr = flatten(load('es-AR'));

  test('has exact key parity with es (no key falls back to English)', () => {
    expect(Object.keys(esAr).sort()).toEqual(Object.keys(es).sort());
  });

  test('no Peninsular tú-form verb leaks in es-AR values', () => {
    // present-indicative tú (complete words) + tú affirmative imperatives at a
    // sentence boundary with a following object (excludes adjective labels like
    // "Activa" and infinitives/nouns like "Configurar"/"Configuración"). The
    // earlier narrow regex missed imperative leaks such as "Crea Tu Cuenta" and
    // "Selecciona un horario".
    const PENINSULAR_INDICATIVE =
      /\b(?:tienes|puedes|quieres|debes|necesitas|eres|vienes|haces|pones|dices|recibes|sabes|eliges|prefieres|confirmas|guardas|pagas|env[ií]as|conoces)\b/i;
    const PENINSULAR_IMPERATIVE =
      /(?:^|[.!:¡¿]\s+)(?:Configura|Elige|Selecciona|Guarda|Agrega|Añade|Ingresa|Introduce|Rellena|Revisa|Comparte|Escribe|Escanea|Conecta|Sube|Descarga|Copia|Pega|Verifica|Personaliza|Genera|Empieza|Comienza|Establece|Decide|Aprueba|Rechaza|Olvida|Activa|Completa|Pulsa|Crea)\s+[a-záéíóúñ]/;
    const leaks = Object.entries(esAr)
      .filter(([, v]) => PENINSULAR_INDICATIVE.test(v) || PENINSULAR_IMPERATIVE.test(v))
      .map(([k, v]) => `${k}: ${v}`);
    expect(leaks).toEqual([]);
  });

  test('diverges substantially from es (genuinely Argentinized, not a copy)', () => {
    const diverged = Object.keys(es).filter((k) => esAr[k] !== es[k]).length;
    expect(diverged).toBeGreaterThan(100);
  });
});
