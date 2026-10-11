import { publicPathForLocale } from '../metadata';

describe('publicPathForLocale', () => {
  test('keeps English public routes unprefixed', () => {
    expect(publicPathForLocale('en', '/business/register')).toBe('/business/register');
  });

  test('prefixes regional public routes', () => {
    expect(publicPathForLocale('es-AR', '/business/register')).toBe(
      '/es-ar/business/register',
    );
  });

  test('maps the root to the bare locale segment', () => {
    expect(publicPathForLocale('es', '/')).toBe('/es');
  });
});
