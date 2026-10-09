import { useEffect, useCallback } from 'react';
import { usePathname } from 'next/navigation';
import { getAnalyticsTracker } from '@/utils/analytics';

export const usePageTracking = () => {
  const pathname = usePathname();
  const tracker = getAnalyticsTracker();

  useEffect(() => {
    if (pathname) {
      void tracker.trackPageView(pathname);
    }
  }, [pathname, tracker]);
};

export const useClickTracking = () => {
  const tracker = getAnalyticsTracker();

  return useCallback(
    (element: string, category: string = 'button', value?: string) => {
      tracker.trackClick(element, category, value);
    },
    [tracker]
  );


};
