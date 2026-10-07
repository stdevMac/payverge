import { ApiCache, stopCacheCleanup } from './cache';

// The cache module registers a module-level auto-cleanup setInterval at import
// time (browser env). Tear it down so it doesn't leak in the Jest worker, and
// exercise the new stopCacheCleanup export at the same time.
afterAll(() => stopCacheCleanup());

describe('ApiCache fingerprint isolation', () => {
  it('isolates entries across different fingerprints', () => {
    const cache = new ApiCache();
    cache.setFingerprint('user:1:biz:42');
    cache.set('/api/v1/businesses', { name: 'Alpha' });

    cache.setFingerprint('user:2:biz:99');
    expect(cache.get('/api/v1/businesses')).toBeNull();
  });

  it('clears all entries when fingerprint changes', () => {
    const cache = new ApiCache();
    cache.setFingerprint('user:1:biz:42');
    cache.set('/api/v1/bills', [{ id: 1 }]);

    cache.setFingerprint('user:1:biz:43');
    expect(cache.get('/api/v1/bills')).toBeNull();
  });

  it('preserves entries when fingerprint is unchanged', () => {
    const cache = new ApiCache();
    cache.setFingerprint('user:1:biz:42');
    cache.set('/api/v1/bills', [{ id: 1 }]);

    cache.setFingerprint('user:1:biz:42');
    expect(cache.get('/api/v1/bills')).toEqual([{ id: 1 }]);
  });

  it('treats null fingerprint as anonymous', () => {
    const cache = new ApiCache();
    cache.setFingerprint(null);
    cache.set('/api/v1/menu', { items: [] });

    cache.setFingerprint('user:1:biz:42');
    expect(cache.get('/api/v1/menu')).toBeNull();
  });
});

describe('stopCacheCleanup', () => {
  it('is idempotent and does not throw when called twice', () => {
    expect(() => {
      stopCacheCleanup();
      stopCacheCleanup();
    }).not.toThrow();
  });
});

describe('ApiCache session-change listeners', () => {
  it('notifies on logout and on a principal switch, not on first login', () => {
    const cache = new ApiCache();
    const listener = jest.fn();
    const unsubscribe = cache.onSessionChange(listener);

    cache.setFingerprint('user:1:biz:42');
    expect(listener).not.toHaveBeenCalled();

    cache.setFingerprint('user:2:biz:42');
    expect(listener).toHaveBeenCalledTimes(1);

    cache.setFingerprint(null);
    expect(listener).toHaveBeenCalledTimes(2);

    cache.endSession();
    expect(listener).toHaveBeenCalledTimes(3);

    unsubscribe();
    cache.endSession();
    expect(listener).toHaveBeenCalledTimes(3);
  });

  it('endSession drops every entry', () => {
    const cache = new ApiCache();
    cache.setFingerprint('user:1:biz:42');
    cache.set('/api/v1/businesses', { name: 'Alpha' });
    cache.endSession();
    cache.setFingerprint('user:1:biz:42');
    expect(cache.get('/api/v1/businesses')).toBeNull();
  });
});
