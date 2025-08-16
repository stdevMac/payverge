"use client";

import React from 'react';
import ComingSoon from './ComingSoon';
import { usePathname } from 'next/navigation';

const WithComingSoon = ({ children }: { children: React.ReactNode }) => {
  const isComingSoonMode = process.env.NEXT_PUBLIC_COMING_SOON_MODE === 'true';

  const pathname = usePathname();
  // Don't show Coming Soon on drop pages
  if (pathname?.startsWith('/drop') || !isComingSoonMode) {
    return <>{children}</>;
  }
  return <ComingSoon />;
};

export default WithComingSoon;
