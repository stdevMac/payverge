'use client'

import { useState } from 'react'
import { Card, CardBody, Tabs, Tab } from '@nextui-org/react'
import { BarChart3, TrendingUp, Package, DollarSign, Users, Clock } from 'lucide-react'
import SalesOverview from './SalesOverview'
import LiveBills from './LiveBills'
import PaymentHistory from './PaymentHistory'
import ItemAnalytics from './ItemAnalytics'
import TipReports from './TipReports'

interface DashboardProps {
  businessId: string
}

export default function Dashboard({ businessId }: DashboardProps) {
  const [activeTab, setActiveTab] = useState('overview')

  const tabs = [
    {
      key: 'overview',
      title: 'Sales Overview',
      icon: BarChart3,
      component: <SalesOverview businessId={businessId} />
    },
    {
      key: 'live',
      title: 'Live Bills',
      icon: Clock,
      component: <LiveBills businessId={businessId} />
    },
    {
      key: 'payments',
      title: 'Payment History',
      icon: Users,
      component: <PaymentHistory businessId={businessId} />
    },
    {
      key: 'items',
      title: 'Item Analytics',
      icon: Package,
      component: <ItemAnalytics businessId={businessId} />
    },
    {
      key: 'tips',
      title: 'Tip Reports',
      icon: DollarSign,
      component: <TipReports businessId={businessId} />
    }
  ]

  return (
    <div className="w-full max-w-7xl mx-auto p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-default-900 mb-2">Analytics Dashboard</h1>
        <p className="text-default-600">
          Monitor your business performance, track payments, and analyze customer behavior in real-time.
        </p>
      </div>

      <Card className="w-full shadow-lg">
        <CardBody className="p-0">
          <Tabs
            aria-label="Dashboard tabs"
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as string)}
            variant="underlined"
            classNames={{
              tabList: "gap-6 w-full relative rounded-none p-0 border-b border-divider",
              cursor: "w-full bg-primary",
              tab: "max-w-fit px-6 py-4 h-12",
              tabContent: "group-data-[selected=true]:text-primary"
            }}
          >
            {tabs.map((tab) => {
              const Icon = tab.icon
              return (
                <Tab
                  key={tab.key}
                  title={
                    <div className="flex items-center gap-2">
                      <Icon className="w-4 h-4" />
                      <span>{tab.title}</span>
                    </div>
                  }
                >
                  <div className="p-6">
                    {tab.component}
                  </div>
                </Tab>
              )
            })}
          </Tabs>
        </CardBody>
      </Card>
    </div>
  )
}
