import fs from 'fs';
import path from 'path';
import enErrors from '../locales/en/apiErrors.json';
import esErrors from '../locales/es/apiErrors.json';
import esArErrors from '../locales/es-ar/apiErrors.json';
import enCatalog from '../locales/en/apiErrors.json';
import {
  translateApiError,
  __loadAllApiErrorCatalogsForTests,
} from '../apiErrors';
import { getLocalizedApiError } from '../../utils/apiError';

describe('translateApiError', () => {
  test('uses Argentine Spanish API error translations', () => {
    expect(translateApiError({ code: 'AUTH_FORBIDDEN' }, 'es-AR')).toContain(
      'No tenés acceso',
    );
  });

  test('falls back to English for unsupported locales', () => {
    // Truly unknown tags (no guest catalog / base language) land on English.
    expect(translateApiError({ code: 'BIZ_NOT_FOUND' }, 'xx-YY')).toBe(
      'Business not found.',
    );
  });

  test('interpolates params in localized API error messages', () => {
    expect(
      translateApiError(
        {
          code: 'VALIDATION_FIELD_INVALID',
          params: { field: 'email' },
        },
        'en',
      ),
    ).toBe('Invalid email. Check the value and try again.');
  });

  test('translates backend-defined validation codes in Argentine Spanish', () => {
    expect(translateApiError({ code: 'VALIDATION_WEAK_PASSWORD' }, 'es-AR')).toBe(
      'La contraseña no cumple los requisitos de seguridad.',
    );
  });

  test('interpolates replacement syntax params literally', () => {
    expect(
      translateApiError(
        {
          code: 'VALIDATION_FIELD_INVALID',
          params: { field: '$&' },
        },
        'en',
      ),
    ).toBe('Invalid $&. Check the value and try again.');
  });
});

describe('getLocalizedApiError (live error path)', () => {
  // Mirror the sanitized axiosInstance error shape:
  //   { response: { status, data: { error, code, params } } }
  const apiError = (data: unknown, status = 400): unknown => ({
    status,
    response: { status, data },
  });

  test('localizes a coded error in neutral Spanish', () => {
    const err = apiError({
      code: 'AUTH_FORBIDDEN',
      error: 'You do not have access to this action.',
    });
    expect(getLocalizedApiError(err, 'es')).toBe(
      'No tienes acceso a esta acción.',
    );
  });

  test('localizes a coded error in Argentine Spanish (voseo)', () => {
    const err = apiError({
      code: 'AUTH_FORBIDDEN',
      error: 'You do not have access to this action.',
    });
    expect(getLocalizedApiError(err, 'es-AR')).toBe(
      'No tenés acceso a esta acción.',
    );
  });

  test('localizes the public-demo refusal instead of the English guard copy', () => {
    // backend/internal/demomode/guard.go abort(): 403 DEMO_MODE_FORBIDDEN
    // with a hard-coded English `error`.
    const err = apiError(
      {
        code: 'DEMO_MODE_FORBIDDEN',
        error:
          'Disabled in the public demo: inviting users. Install your own Payverge to use it — everything else here works, and the demo resets nightly.',
        params: { reason: 'demo_mode', kind: 'staff_access' },
      },
      403,
    );
    expect(getLocalizedApiError(err, 'es')).toBe(
      'Esta acción está deshabilitada en la demo pública. Instala tu propio Payverge para usarla; la demo se reinicia cada noche.',
    );
    expect(getLocalizedApiError(err, 'es-AR')).toContain('Instalá tu propio Payverge');
    expect(getLocalizedApiError(err, 'en')).toBe(
      'This action is disabled in the public demo. Install your own Payverge to use it; the demo resets nightly.',
    );
  });

  test('interpolates coded validation errors with params', () => {
    const err = apiError({
      code: 'VALIDATION_FIELD_INVALID',
      error: 'Invalid email.',
      params: { field: 'email' },
    });
    expect(getLocalizedApiError(err, 'en')).toBe(
      'Invalid email. Check the value and try again.',
    );
  });

  test('falls back to the raw backend string for an unknown/uncoded code', () => {
    const err = apiError({
      code: 'SOME_BRAND_NEW_CODE',
      error: 'A very specific backend message.',
    });
    expect(getLocalizedApiError(err, 'es')).toBe(
      'A very specific backend message.',
    );
  });

  test('falls back to the raw backend string when there is no code at all', () => {
    const err = apiError({ error: 'Plain uncoded backend error.' });
    expect(getLocalizedApiError(err, 'es-AR')).toBe(
      'Plain uncoded backend error.',
    );
  });

  test('falls back to a generic localized message when there is no error string', () => {
    // No code AND no error string: should not surface an empty toast.
    const err = apiError({});
    expect(getLocalizedApiError(err, 'es')).toBe(esErrors.GENERIC_ERROR);
    expect(getLocalizedApiError(err, 'es-AR')).toBe(esArErrors.GENERIC_ERROR);
    expect(getLocalizedApiError(err, 'en')).toBe(enErrors.GENERIC_ERROR);
  });

  test('returns a generic localized message for a completely non-api throw', () => {
    expect(getLocalizedApiError(new TypeError('boom'), 'es-AR')).toBe(
      esArErrors.GENERIC_ERROR,
    );
    expect(getLocalizedApiError(new TypeError('boom'), 'es')).toBe(
      esErrors.GENERIC_ERROR,
    );
    expect(getLocalizedApiError(new TypeError('boom'), 'en')).toBe(
      enErrors.GENERIC_ERROR,
    );
  });

  test('defaults to English for an unsupported locale', () => {
    const err = apiError({ code: 'BIZ_NOT_FOUND' });
    expect(getLocalizedApiError(err, 'xx-YY')).toBe('Business not found.');
  });
});

describe('guest-tier locale resolution (Wave 5)', () => {
  let catalogs: Record<string, Record<string, string>>;

  beforeAll(async () => {
    catalogs = await __loadAllApiErrorCatalogsForTests();
  });

  it('resolves a guest-only locale instead of falling back to English', () => {
    const fr = translateApiError({ code: 'RATE_LIMITED' }, 'fr');
    expect(fr).not.toBe('');
    expect(fr).not.toBe(translateApiError({ code: 'RATE_LIMITED' }, 'en'));
  });

  it('falls back by base language, then to English', () => {
    expect(translateApiError({ code: 'GENERIC_ERROR' }, 'fr-CA')).toBe(
      translateApiError({ code: 'GENERIC_ERROR' }, 'fr'),
    );
    expect(translateApiError({ code: 'GENERIC_ERROR' }, 'xx-YY')).toBe(
      translateApiError({ code: 'GENERIC_ERROR' }, 'en'),
    );
  });

  it('every guest locale ships the shared catalog', () => {
    // All 21 catalogs, including the 18 loaded on demand, match the English
    // key set exactly. Loyalty codes are no longer operator-only.
    const enKeys = Object.keys(enCatalog).sort();
    expect(enKeys.length).toBeGreaterThan(50);
    expect(Object.keys(catalogs)).toHaveLength(21);

    for (const [locale, catalog] of Object.entries(catalogs)) {
      expect({ locale, keys: Object.keys(catalog).sort() }).toEqual({
        locale,
        keys: enKeys,
      });
    }
  });
});

describe('apiErrors static imports', () => {
  it('statically imports apiErrors catalogs only for en, es, and es-ar', () => {
    const source = fs.readFileSync(
      path.join(__dirname, '..', 'apiErrors.ts'),
      'utf8',
    );
    const dirs = [
      ...source.matchAll(
        /import\s+\w+\s+from\s+['"]\.\/locales\/([^/'"]+)\/apiErrors\.json['"]/g,
      ),
    ].map((match) => match[1]);
    expect(dirs).toHaveLength(3);
    expect([...dirs].sort()).toEqual(['en', 'es', 'es-ar']);
  });
});

describe('apiErrors voseo correctness (OP-3)', () => {
  // The es base must be NEUTRAL Spanish (no voseo); es-AR must be voseo.
  // VOSEO present-indicative carries an accented final syllable (tenés, podés,
  // necesitás) and VOSEO affirmative imperatives of -ar verbs carry an accented
  // final -á (iniciá, revisá, intentá, esperá, volvé). The neutral forms drop
  // the accent (tienes, inicia, revisa) — so REQUIRING the accent cleanly
  // separates the two registers without false-positiving on neutral copy.
  const VOSEO_VERBS =
    /\b(?:ten[ée]s|pod[ée]s|quer[ée]s|deb[ée]s|necesit[áa]s|sos|hac[ée]s|inici[á]|revis[á]|intent[á]|esper[á]|verific[á]|ingres[á]|complet[á]|agreg[á]|configur[á]|eleg[í]|seleccion[á]|guard[á]|conect[á]|actualiz[á]|us[á]|volv[é])\b/i;
  // tú-form / neutral leaks we explicitly forbid in es-AR (must be voseo there).
  const TU_FORM =
    /\b(?:tienes|puedes|quieres|debes|necesitas|eres|haces|inicia|revisa|intenta|espera|verifica|ingresa|completa|agrega|configura|elige|selecciona|guarda|conecta|actualiza|vuelve)\b/i;

  const values = (o: Record<string, string>) => Object.values(o);

  test('es base is neutral Spanish — no voseo verb forms', () => {
    const leaks = values(esErrors as Record<string, string>).filter((v) =>
      VOSEO_VERBS.test(v),
    );
    expect(leaks).toEqual([]);
  });

  test('es-AR has no Peninsular tú-form leaks', () => {
    const leaks = values(esArErrors as Record<string, string>).filter((v) =>
      TU_FORM.test(v),
    );
    expect(leaks).toEqual([]);
  });

  test('es-AR genuinely uses voseo where second-person verbs appear', () => {
    const hasVoseo = values(esArErrors as Record<string, string>).some((v) =>
      VOSEO_VERBS.test(v),
    );
    expect(hasVoseo).toBe(true);
  });
});
