'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { Card, CardBody, CardHeader, Select, SelectItem, Spinner, Divider } from '@nextui-org/react'
import { TrendingUp, DollarSign, CreditCard, Users, Clock, Receipt, TrendingDown } from 'lucide-react'
import analyticsApi, { SalesData } from '@/api/analytics'

interface SalesOverviewProps {
  businessId: string
}

export default function SalesOverview({ businessId }: SalesOverviewProps) {
  const [salesData, setSalesData] = useState<SalesData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [period, setPeriod] = useState('today')

  const fetchSalesData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await analyticsApi.getSalesAnalytics(businessId, period)
      setSalesData(data)
    } catch (err) {
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
            <p>Error loading sales data: {error}</p>
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
            <p>No sales data available</p>
          </div>
        </CardBody>
      </Card>
    )
  }

  const periods = [
    { key: 'today', label: 'Today' },
    { key: 'yesterday', label: 'Yesterday' },
    { key: 'week', label: 'This Week' },
    { key: 'month', label: 'This Month' },
    { key: 'quarter', label: 'This Quarter' },
    { key: 'year', label: 'This Year' },
  ]

  const metrics = [
    {
      title: 'Total Revenue',
      value: formatCurrency(salesData.total_revenue),
      icon: DollarSign,
      color: 'text-success',
      bgColor: 'bg-success/10',
    },
    {
      title: 'Total Tips',
      value: formatCurrency(salesData.total_tips),
      icon: TrendingUp,
      color: 'text-warning',
      bgColor: 'bg-warning/10',
    },
    {
      title: 'Transactions',
      value: salesData.transaction_count.toString(),
      icon: CreditCard,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
    },
    {
      title: 'Bills',
      value: salesData.bill_count.toString(),
      icon: Receipt,
      color: 'text-secondary',
      bgColor: 'bg-secondary/10',
    },
    {
      title: 'Unique Customers',
      value: salesData.unique_customers.toString(),
      icon: Users,
      color: 'text-default-600',
      bgColor: 'bg-default/10',
    },
    {
      title: 'Average Ticket',
      value: formatCurrency(salesData.average_ticket),
      icon: TrendingUp,
      color: 'text-success',
      bgColor: 'bg-success/10',
    },
  ]

  // Calculate tip rate
  const tipRate = salesData.total_revenue > 0 
    ? (salesData.total_tips / salesData.total_revenue) * 100 
    : 0

  return (
    <div className="space-y-6">
      {/* Header with Period Selector */}
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">Sales Overview</h2>
        <Select
          size="sm"
          selectedKeys={[period]}
          onSelectionChange={(keys) => setPeriod(Array.from(keys)[0] as string)}
          className="w-40"
        >
          {periods.map((p) => (
            <SelectItem key={p.key} value={p.key}>
              {p.label}
            </SelectItem>
          ))}
        </Select>
      </div>

      {/* Key Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {metrics.map((metric, index) => {
          const Icon = metric.icon
          return (
            <Card key={index} className="border-none shadow-sm">
              <CardBody className="p-4">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${metric.bgColor}`}>
                    <Icon className={`w-5 h-5 ${metric.color}`} />
                  </div>
                  <div className="flex-1">
                    <p className="text-sm text-default-600">{metric.title}</p>
                    <p className="text-xl font-semibold">{metric.value}</p>
                  </div>
                </div>
              </CardBody>
            </Card>
          )
        })}
      </div>

      {/* Additional Insights */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Tip Performance */}
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Tip Performance</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="space-y-4">
              <div className="flex justify-between items-center">
                <span className="text-default-600">Tip Rate</span>
                <div className="flex items-center gap-2">
                  {tipRate >= 15 ? (
                    <TrendingUp className="w-4 h-4 text-success" />
                  ) : (
                    <TrendingDown className="w-4 h-4 text-danger" />
                  )}
                  <span className={`font-semibold ${tipRate >= 15 ? 'text-success' : 'text-danger'}`}>
                    {formatPercentage(tipRate)}
                  </span>
                </div>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-default-600">Average Tip per Transaction</span>
                <span className="font-semibold">
                  {formatCurrency(salesData.transaction_count > 0 ? salesData.total_tips / salesData.transaction_count : 0)}
                </span>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Payment Methods */}
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Payment Methods</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="space-y-3">
              {Object.entries(salesData.payment_methods).map(([method, count]) => {
                const percentage = salesData.transaction_count > 0 
                  ? (count / salesData.transaction_count) * 100 
                  : 0
                return (
                  <div key={method} className="flex justify-between items-center">
                    <span className="text-default-600 capitalize">{method}</span>
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-default-500">
                        {count} ({formatPercentage(percentage)})
                      </span>
                    </div>
                  </div>
                )
              })}
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Hourly Breakdown */}
      {Object.keys(salesData.hourly_breakdown).length > 0 && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Hourly Breakdown</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
              {Object.entries(salesData.hourly_breakdown)
                .sort(([a], [b]) => parseInt(a) - parseInt(b))
                .map(([hour, data]) => (
                  <div key={hour} className="text-center p-3 bg-default-50 rounded-lg">
                    <p className="text-sm font-medium">{hour}:00</p>
                    <p className="text-xs text-default-600 mt-1">
                      {formatCurrency(data.revenue)}
                    </p>
                    <p className="text-xs text-default-500">
                      {data.bill_count} bills
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
