'use client'

import { useState, useEffect } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
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
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [activeTab, setActiveTab] = useState('overview')

  const tabs = [
    {
      key: 'overview',
      title: tString('tabs.salesOverview'),
      icon: BarChart3,
      component: <SalesOverview businessId={businessId} />
    },
    {
      key: 'live',
      title: tString('tabs.liveBills'),
      icon: Clock,
      component: <LiveBills businessId={businessId} />
    },
    {
      key: 'payments',
      title: tString('tabs.paymentHistory'),
      icon: Users,
      component: <PaymentHistory businessId={businessId} />
    },
    {
      key: 'items',
      title: tString('tabs.itemAnalytics'),
      icon: Package,
      component: <ItemAnalytics businessId={businessId} />
    },
    {
      key: 'tips',
      title: tString('tabs.tipReports'),
      icon: DollarSign,
      component: <TipReports businessId={businessId} />
    }
  ]

  return (
    <div className="w-full max-w-7xl mx-auto p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-default-900 mb-2">{tString('title')}</h1>
        <p className="text-default-600">
          {tString('subtitle')}
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
