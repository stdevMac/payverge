'use client'

import { useParams } from 'next/navigation'
import Dashboard from '@/components/dashboard/Dashboard'

export default function AnalyticsPage() {
  const params = useParams()
  const businessId = params.businessId as string

  return <Dashboard businessId={businessId} />
}
