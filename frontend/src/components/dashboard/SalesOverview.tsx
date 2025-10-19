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
      <Card className="w-full">
        <CardBody className="flex items-center justify-center p-8">
          <Spinner size="lg" />
        </CardBody>
      </Card>
    )
  }

  if (error) {
    return (
      <Card className="w-full">
        <CardBody className="p-8">
          <div className="text-center text-danger">
            <p>{tString('error')}: {error}</p>
          </div>
        </CardBody>
      </Card>
    )
  }

  if (!salesData) {
    return (
      <Card className="w-full">
        <CardBody className="p-8">
          <div className="text-center text-default-500">
            <p>{tString('noData')}</p>
          </div>
        </CardBody>
      </Card>
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
    <div className="space-y-8">
      {/* Header with Period Selector */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-3xl font-bold text-default-900">{tString('title')}</h2>
          <p className="text-default-600 mt-1">{tString('subtitle')}</p>
        </div>
        <Select
          size="md"
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
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
        {metrics.map((metric, index) => {
          const Icon = metric.icon
          return (
            <Card key={index} className="border-none shadow-md hover:shadow-lg transition-shadow duration-200">
              <CardBody className="p-6">
                <div className="flex items-center gap-4">
                  <div className={`p-3 rounded-xl ${metric.bgColor}`}>
                    <Icon className={`w-6 h-6 ${metric.color}`} />
                  </div>
                  <div className="flex-1">
                    <p className="text-sm font-medium text-default-600 uppercase tracking-wide">{metric.title}</p>
                    <p className="text-2xl font-bold text-default-900 mt-1">{metric.value}</p>
                  </div>
                </div>
              </CardBody>
            </Card>
          )
        })}
      </div>

      {/* Additional Insights */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Tip Performance */}
        <Card className="shadow-md">
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <TrendingUp className="w-5 h-5 text-success" />
              <h3 className="text-xl font-semibold text-default-900">{tString('tipPerformance.title')}</h3>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="p-6">
            <div className="space-y-6">
              <div className="flex justify-between items-center p-4 bg-default-50 rounded-lg">
                <div>
                  <p className="text-sm font-medium text-default-600 uppercase tracking-wide">{tString('tipPerformance.tipRate')}</p>
                  <p className="text-xs text-default-500 mt-1">{tString('tipPerformance.tipRateDesc')}</p>
                </div>
                <div className="flex items-center gap-3">
                  {tipRate >= 15 ? (
                    <TrendingUp className="w-5 h-5 text-success" />
                  ) : (
                    <TrendingDown className="w-5 h-5 text-danger" />
                  )}
                  <span className={`text-2xl font-bold ${tipRate >= 15 ? 'text-success' : 'text-danger'}`}>
                    {formatPercentage(tipRate)}
                  </span>
                </div>
              </div>
              <div className="flex justify-between items-center p-4 bg-default-50 rounded-lg">
                <div>
                  <p className="text-sm font-medium text-default-600 uppercase tracking-wide">{tString('tipPerformance.averageTip')}</p>
                  <p className="text-xs text-default-500 mt-1">{tString('tipPerformance.averageTipDesc')}</p>
                </div>
                <span className="text-2xl font-bold text-default-900">
                  {formatCurrency((salesData.transaction_count || 0) > 0 ? (salesData.total_tips || 0) / (salesData.transaction_count || 1) : 0)}
                </span>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Payment Methods */}
        <Card className="shadow-md">
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <CreditCard className="w-5 h-5 text-primary" />
              <h3 className="text-xl font-semibold text-default-900">{tString('paymentMethods.title')}</h3>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="p-6">
            <div className="space-y-4">
              {Object.entries(salesData.payment_methods || {}).length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-default-500">{tString('paymentMethods.noData')}</p>
                </div>
              ) : (
                Object.entries(salesData.payment_methods || {}).map(([method, count]) => {
                  const percentage = (salesData.transaction_count || 0) > 0 
                    ? (count / (salesData.transaction_count || 1)) * 100 
                    : 0
                  return (
                    <div key={method} className="flex justify-between items-center p-3 bg-default-50 rounded-lg">
                      <span className="text-default-700 font-medium capitalize">{method}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-default-500">
                          {count} ({formatPercentage(percentage)})
                        </span>
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Hourly Breakdown */}
      {Object.keys(salesData.hourly_breakdown || {}).length > 0 && (
        <Card className="shadow-md">
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <Clock className="w-5 h-5 text-warning" />
              <h3 className="text-xl font-semibold text-default-900">{tString('hourlyBreakdown.title')}</h3>
            </div>
            <p className="text-sm text-default-600 mt-1">{tString('hourlyBreakdown.subtitle')}</p>
          </CardHeader>
          <Divider />
          <CardBody className="p-6">
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
              {Object.entries(salesData.hourly_breakdown || {})
                .sort(([a], [b]) => parseInt(a) - parseInt(b))
                .map(([hour, data]) => (
                  <div key={hour} className="text-center p-4 bg-gradient-to-br from-default-50 to-default-100 rounded-xl border border-default-200 hover:shadow-md transition-all duration-200">
                    <p className="text-lg font-bold text-default-900">{hour}:00</p>
                    <p className="text-sm font-semibold text-success mt-2">
                      {formatCurrency(data.revenue)}
                    </p>
                    <p className="text-xs text-default-500 mt-1">
                      {data.bill_count} {tString('hourlyBreakdown.bills')}
                    </p>
                  </div>
                ))}
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
