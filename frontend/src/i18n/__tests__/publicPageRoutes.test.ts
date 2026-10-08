import {
  PUBLIC_PAGE_LOCALES,
  publicPageAlternates,
  localizedPublicHref,
} from '../publicPageRoutes';

const SITE = 'https://payverge.io';

describe('public page hreflang cluster', () => {
  test('builds a reciprocal en/es/es-AR cluster with x-default at the English base', () => {
    const a = publicPageAlternates('es-AR', '/business/register');
    expect(a.canonical).toBe(`${SITE}/es-ar/business/register`);
    expect(a.languages['en']).toBe(`${SITE}/business/register`);
    expect(a.languages['es']).toBe(`${SITE}/es/business/register`);
    expect(a.languages['es-AR']).toBe(`${SITE}/es-ar/business/register`);
    expect(a.languages['x-default']).toBe(`${SITE}/business/register`);

    const home = publicPageAlternates('es', '/');
    expect(home.canonical).toBe(`${SITE}/es`);
    expect(home.languages['en']).toBe(`${SITE}/`);
    expect(home.languages['es-AR']).toBe(`${SITE}/es-ar`);
  });

  test('the cluster is exactly en, es and es-AR', () => {
    expect([...PUBLIC_PAGE_LOCALES].sort()).toEqual(['en', 'es', 'es-AR']);
  });

  test.each([
    ['en', '/staff/login', '/staff/login'],
    ['es', '/staff/login', '/es/staff/login'],
    ['es-AR', '/business/register', '/es-ar/business/register'],
    ['en', '/', '/'],
    ['es', '/', '/es'],
  ] as const)('localizedPublicHref(%s, %s) → %s', (locale, route, expected) => {
    expect(localizedPublicHref(locale, route)).toBe(expected);
  });
});
