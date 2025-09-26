'use client';

import React from 'react';
import { useRouter } from 'next/navigation';
import StaffLogin from '../../../components/staff/StaffLogin';

export default function StaffLoginPage() {
  const router = useRouter();

  const handleLoginSuccess = (staffData: any) => {
    // Redirect to staff dashboard or appropriate page
    // For now, we'll redirect to the business dashboard
    if (staffData.business_id) {
      router.push(`/business/${staffData.business_id}/dashboard`);
    } else {
      router.push('/dashboard');
    }
  };

  return <StaffLogin onLoginSuccess={handleLoginSuccess} />;
}
