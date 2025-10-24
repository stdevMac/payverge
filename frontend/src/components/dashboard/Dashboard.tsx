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
    <div className="min-h-screen bg-white">
      {/* Header Section */}
      <section className="py-16 lg:py-20 bg-gradient-to-b from-gray-50 to-white relative overflow-hidden">
        {/* Clean background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
        </div>

        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 hover:bg-white hover:shadow-sm transition-all duration-300">
              <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse"></div>
              Analytics Dashboard
            </div>

            <h1 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              {tString('title')}
            </h1>
            <p className="text-lg font-light leading-relaxed max-w-2xl mx-auto text-gray-600 mb-12 tracking-wide">
              {tString('subtitle')}
            </p>
          </div>
        </div>
      </section>

      {/* Tab Navigation */}
      <section className="py-8 bg-white border-b border-gray-200">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto">
            <div className="flex flex-wrap justify-center gap-2 sm:gap-4">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                const isActive = activeTab === tab.key;
                return (
                  <button
                    key={tab.key}
                    onClick={() => setActiveTab(tab.key)}
                    className={`
                      group flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium transition-all duration-300
                      ${isActive 
                        ? 'bg-gray-900 text-white shadow-lg' 
                        : 'bg-gray-50 text-gray-700 hover:bg-gray-100 hover:shadow-sm border border-gray-200'
                      }
                    `}
                  >
                    <Icon className={`w-4 h-4 transition-colors duration-300 ${
                      isActive ? 'text-white' : 'text-gray-600 group-hover:text-gray-700'
                    }`} />
                    <span>{tab.title}</span>
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      </section>

      {/* Content Section */}
      <section className="py-16 lg:py-20 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-6xl mx-auto">
            <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-8">
              {tabs.find(tab => tab.key === activeTab)?.component}
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}
