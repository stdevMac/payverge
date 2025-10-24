'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
import { Card, CardBody, CardHeader, Select, SelectItem, Spinner, Divider } from '@nextui-org/react'
import { TrendingUp, DollarSign, CreditCard, Users, Clock, Receipt, TrendingDown } from 'lucide-react'
import analyticsApi, { SalesData } from '@/api/analytics'

interface SalesOverviewProps {
  businessId: string
}

export default function SalesOverview({ businessId }: SalesOverviewProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.salesOverview.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [salesData, setSalesData] = useState<SalesData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [period, setPeriod] = useState('today')

  const fetchSalesData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await analyticsApi.getSalesAnalytics(businessId, period)
      console.log('SalesOverview - Raw API response:', data)
      console.log('SalesOverview - Data properties:', Object.keys(data || {}))
      setSalesData(data)
    } catch (err) {
      console.error('SalesOverview - API Error:', err)
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [businessId, period])

  useEffect(() => {
    fetchSalesData()
  }, [fetchSalesData])

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(amount)
  }

  const formatPercentage = (value: number) => {
    return `${value.toFixed(1)}%`
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <div className="flex items-center gap-3">
          <TrendingDown className="w-5 h-5 text-red-600 flex-shrink-0" />
          <p className="text-red-800 font-medium">{tString('error')}: {error}</p>
        </div>
      </div>
    )
  }

  if (!salesData) {
    return (
      <div className="text-center py-12">
        <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
          <DollarSign className="w-8 h-8 text-gray-400" />
        </div>
        <p className="text-gray-600 font-light tracking-wide">{tString('noData')}</p>
      </div>
    )
  }

  const periods = [
    { key: 'today', label: tString('periods.today') },
    { key: 'yesterday', label: tString('periods.yesterday') },
    { key: 'week', label: tString('periods.week') },
    { key: 'month', label: tString('periods.month') },
    { key: 'quarter', label: tString('periods.quarter') },
    { key: 'year', label: tString('periods.year') },
  ]

  const metrics = [
    {
      title: tString('metrics.totalRevenue'),
      value: formatCurrency(salesData.total_revenue || 0),
      icon: DollarSign,
      color: 'text-success',
      bgColor: 'bg-success/10',
    },
    {
      title: tString('metrics.totalTips'),
      value: formatCurrency(salesData.total_tips || 0),
      icon: TrendingUp,
      color: 'text-warning',
      bgColor: 'bg-warning/10',
    },
    {
      title: tString('metrics.transactions'),
      value: (salesData.transaction_count || 0).toString(),
      icon: CreditCard,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
    },
    {
      title: tString('metrics.bills'),
      value: (salesData.bill_count || 0).toString(),
      icon: Receipt,
      color: 'text-secondary',
      bgColor: 'bg-secondary/10',
    },
    {
      title: tString('metrics.uniqueCustomers'),
      value: (salesData.unique_customers || 0).toString(),
      icon: Users,
      color: 'text-default-600',
      bgColor: 'bg-default/10',
    },
    {
      title: tString('metrics.averageTicket'),
      value: formatCurrency(salesData.average_ticket || 0),
      icon: TrendingUp,
      color: 'text-success',
      bgColor: 'bg-success/10',
    },
  ]

  // Calculate tip rate
  const tipRate = (salesData.total_revenue || 0) > 0 
    ? ((salesData.total_tips || 0) / (salesData.total_revenue || 1)) * 100 
    : 0

  return (
    <div className="space-y-6">
      {/* Header with Period Selector */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
          <p className="text-gray-600 font-light text-sm">{tString('subtitle')}</p>
        </div>
        <Select
          size="sm"
          selectedKeys={[period]}
          onSelectionChange={(keys) => setPeriod(Array.from(keys)[0] as string)}
          className="w-full sm:w-48"
          variant="bordered"
        >
          {periods.map((p) => (
            <SelectItem key={p.key} value={p.key}>
              {p.label}
            </SelectItem>
          ))}
        </Select>
      </div>

      {/* Key Metrics Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {metrics.map((metric, index) => {
          const Icon = metric.icon
          return (
            <div key={index} className="bg-white border border-gray-200 rounded-lg shadow-sm p-4 hover:shadow-md transition-shadow duration-200">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                  <Icon className="w-5 h-5 text-gray-700" />
                </div>
                <div className="flex-1">
                  <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{metric.title}</p>
                  <p className="text-lg font-semibold text-gray-900 mt-1">{metric.value}</p>
                </div>
              </div>
            </div>
          )
        })}
      </div>

      {/* Additional Insights */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Tip Performance */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <TrendingUp className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('tipPerformance.title')}</h3>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-4">
              <div className="flex justify-between items-center p-3 bg-gray-50 rounded-lg">
                <div>
                  <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('tipPerformance.tipRate')}</p>
                  <p className="text-xs text-gray-500 mt-1">{tString('tipPerformance.tipRateDesc')}</p>
                </div>
                <div className="flex items-center gap-2">
                  {tipRate >= 15 ? (
                    <TrendingUp className="w-4 h-4 text-green-600" />
                  ) : (
                    <TrendingDown className="w-4 h-4 text-red-600" />
                  )}
                  <span className={`text-lg font-semibold ${tipRate >= 15 ? 'text-green-600' : 'text-red-600'}`}>
                    {formatPercentage(tipRate)}
                  </span>
                </div>
              </div>
              <div className="flex justify-between items-center p-3 bg-gray-50 rounded-lg">
                <div>
                  <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('tipPerformance.averageTip')}</p>
                  <p className="text-xs text-gray-500 mt-1">{tString('tipPerformance.averageTipDesc')}</p>
                </div>
                <span className="text-lg font-semibold text-gray-900">
                  {formatCurrency((salesData.transaction_count || 0) > 0 ? (salesData.total_tips || 0) / (salesData.transaction_count || 1) : 0)}
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* Payment Methods */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <CreditCard className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('paymentMethods.title')}</h3>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-3">
              {Object.entries(salesData.payment_methods || {}).length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-gray-500 font-light">{tString('paymentMethods.noData')}</p>
                </div>
              ) : (
                Object.entries(salesData.payment_methods || {}).map(([method, count]) => {
                  const percentage = (salesData.transaction_count || 0) > 0 
                    ? (count / (salesData.transaction_count || 1)) * 100 
                    : 0
                  return (
                    <div key={method} className="flex justify-between items-center p-3 bg-gray-50 rounded-lg">
                      <span className="text-gray-700 font-medium capitalize">{method}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-gray-500">
                          {count} ({formatPercentage(percentage)})
                        </span>
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Hourly Breakdown */}
      {Object.keys(salesData.hourly_breakdown || {}).length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <Clock className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('hourlyBreakdown.title')}</h3>
                <p className="text-sm text-gray-600 font-light">{tString('hourlyBreakdown.subtitle')}</p>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3">
              {Object.entries(salesData.hourly_breakdown || {})
                .sort(([a], [b]) => parseInt(a) - parseInt(b))
                .map(([hour, data]) => (
                  <div key={hour} className="text-center p-3 bg-gray-50 rounded-lg border border-gray-100 hover:shadow-sm transition-all duration-200">
                    <p className="text-sm font-semibold text-gray-900">{hour}:00</p>
                    <p className="text-sm font-medium text-green-600 mt-1">
                      {formatCurrency(data.revenue)}
                    </p>
                    <p className="text-xs text-gray-500 mt-1">
                      {data.bill_count} {tString('hourlyBreakdown.bills')}
                    </p>
                  </div>
                ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
