'use client'

import { useState, useEffect } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
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
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
      </div>

      {/* Tab Navigation */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
        <div className="border-b border-gray-200 p-6">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <BarChart3 className="w-6 h-6 text-gray-700" />
            </div>
            <div>
              <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('analytics.title')}</h2>
              <p className="text-gray-600 font-light text-sm">{tString('analytics.subtitle')}</p>
            </div>
          </div>
        </div>
        <div className="p-6">
          <div className="flex flex-wrap gap-2">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.key;
              return (
                <button
                  key={tab.key}
                  onClick={() => setActiveTab(tab.key)}
                  className={`
                    group flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200
                    ${isActive 
                      ? 'bg-gray-900 text-white shadow-sm' 
                      : 'bg-gray-50 text-gray-700 hover:bg-gray-100 border border-gray-200'
                    }
                  `}
                >
                  <Icon className={`w-4 h-4 transition-colors duration-200 ${
                    isActive ? 'text-white' : 'text-gray-600 group-hover:text-gray-700'
                  }`} />
                  <span>{tab.title}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>

      {/* Content Section */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="p-6">
          {tabs.find(tab => tab.key === activeTab)?.component}
        </div>
      </div>
    </div>
  )
}
