'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
import { 
  Card, 
  CardBody, 
  CardHeader, 
  Divider, 
  Spinner, 
  Select, 
  SelectItem,
  Progress,
  Chip,
  Button
} from '@nextui-org/react'
import { TrendingUp, Package, BarChart3, TrendingDown, DollarSign } from 'lucide-react'
import analyticsApi, { ItemStats } from '@/api/analytics'

interface ItemAnalyticsProps {
  businessId: string
}

export default function ItemAnalytics({ businessId }: ItemAnalyticsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.itemAnalytics.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [items, setItems] = useState<ItemStats[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [period, setPeriod] = useState('week')
  const [sortBy, setSortBy] = useState('total_sold')

  const fetchItemAnalytics = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await analyticsApi.getItemAnalytics(businessId, period)
      console.log('ItemAnalytics - Raw API response:', data)
      console.log('ItemAnalytics - Data type:', typeof data, Array.isArray(data))
      setItems(data || [])
    } catch (err) {
      console.error('ItemAnalytics - API Error:', err)
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [businessId, period])

  useEffect(() => {
    fetchItemAnalytics()
  }, [fetchItemAnalytics])

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(amount)
  }

  const sortedItems = [...items].sort((a, b) => {
    switch (sortBy) {
      case 'total_sold':
        return b.total_sold - a.total_sold
      case 'revenue':
        return b.revenue - a.revenue
      case 'bills_featured':
        return b.bills_featured - a.bills_featured
      case 'avg_price':
        return b.avg_price - a.avg_price
      default:
        return 0
    }
  })

  const maxValue = Math.max(...items.map(item => {
    switch (sortBy) {
      case 'total_sold': return item.total_sold
      case 'revenue': return item.revenue
      case 'bills_featured': return item.bills_featured
      case 'avg_price': return item.avg_price
      default: return 0
    }
  }))

  const getPerformanceColor = (rank: number) => {
    if (rank <= 3) return 'success'
    if (rank <= 10) return 'warning'
    return 'default'
  }

  const getPerformanceIcon = (rank: number) => {
    if (rank <= 3) return <TrendingUp className="w-4 h-4" />
    if (rank <= 10) return <BarChart3 className="w-4 h-4" />
    return <TrendingDown className="w-4 h-4" />
  }

  const periods = [
    { key: 'today', label: tString('periods.today') },
    { key: 'yesterday', label: tString('periods.yesterday') },
    { key: 'week', label: tString('periods.week') },
    { key: 'month', label: tString('periods.month') },
    { key: 'quarter', label: tString('periods.quarter') },
  ]

  const sortOptions = [
    { key: 'total_sold', label: tString('sortOptions.quantitySold') },
    { key: 'revenue', label: tString('sortOptions.revenue') },
    { key: 'bills_featured', label: tString('sortOptions.billsFeatured') },
    { key: 'avg_price', label: tString('sortOptions.averagePrice') },
  ]

  // Calculate category performance
  const categoryStats = items.reduce((acc, item) => {
    if (!acc[item.category]) {
      acc[item.category] = {
        total_sold: 0,
        revenue: 0,
        item_count: 0,
      }
    }
    acc[item.category].total_sold += item.total_sold
    acc[item.category].revenue += item.revenue
    acc[item.category].item_count += 1
    return acc
  }, {} as Record<string, { total_sold: number; revenue: number; item_count: number }>)

  const topCategories = Object.entries(categoryStats)
    .sort(([,a], [,b]) => b.revenue - a.revenue)
    .slice(0, 5)

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
            <p>Error loading item analytics: {error}</p>
          </div>
        </CardBody>
      </Card>
    )
  }

  return (
    <div className="space-y-8">
      {/* Header with Controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <h2 className="text-3xl font-bold text-default-900">{tString('title')}</h2>
          <p className="text-default-600 mt-1">{tString('subtitle')}</p>
        </div>
        
        <div className="flex flex-col sm:flex-row gap-3">
          <Select
            size="md"
            selectedKeys={[period]}
            onSelectionChange={(keys) => setPeriod(Array.from(keys)[0] as string)}
            className="w-full sm:w-40"
            variant="bordered"
            label={tString('period')}
          >
            {periods.map((p) => (
              <SelectItem key={p.key} value={p.key}>
                {p.label}
              </SelectItem>
            ))}
          </Select>
          
          <Select
            size="md"
            selectedKeys={[sortBy]}
            onSelectionChange={(keys) => setSortBy(Array.from(keys)[0] as string)}
            className="w-full sm:w-48"
            variant="bordered"
            label={tString('sortBy')}
          >
            {sortOptions.map((option) => (
              <SelectItem key={option.key} value={option.key}>
                {option.label}
              </SelectItem>
            ))}
          </Select>
        </div>
      </div>

      {items.length === 0 ? (
        <Card>
          <CardBody className="p-8">
            <div className="text-center text-default-500">
              <Package className="w-12 h-12 mx-auto mb-4 opacity-50" />
              <p className="text-lg font-medium mb-2">{tString('noItemData')}</p>
              <p>{tString('noSalesData')}</p>
            </div>
          </CardBody>
        </Card>
      ) : (
        <>
          {/* Top Categories */}
          {topCategories.length > 0 && (
            <Card>
              <CardHeader>
                <h3 className="text-lg font-semibold">{tString('topCategories')}</h3>
              </CardHeader>
              <Divider />
              <CardBody className="p-4">
                <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">
                  {topCategories.map(([category, stats], index) => (
                    <div key={category} className="text-center p-4 bg-default-50 rounded-lg">
                      <div className="flex items-center justify-center gap-2 mb-2">
                        <span className="text-lg font-bold text-primary">#{index + 1}</span>
                        <Chip size="sm" color="primary" variant="flat">
                          {stats.item_count} {tString('items')}
                        </Chip>
                      </div>
                      <p className="font-semibold text-sm mb-1">{category}</p>
                      <p className="text-xs text-success font-medium">
                        {formatCurrency(stats.revenue)}
                      </p>
                      <p className="text-xs text-default-600">
                        {stats.total_sold} {tString('sold')}
                      </p>
                    </div>
                  ))}
                </div>
              </CardBody>
            </Card>
          )}

          {/* Item Performance List */}
          <Card>
            <CardHeader>
              <div className="flex justify-between items-center w-full">
                <h3 className="text-lg font-semibold">{tString('itemPerformance')}</h3>
                <span className="text-sm text-default-500">
                  {tString('sortedBy')} {sortOptions.find(opt => opt.key === sortBy)?.label}
                </span>
              </div>
            </CardHeader>
            <Divider />
            <CardBody className="p-0">
              <div className="divide-y divide-default-200">
                {sortedItems.slice(0, 20).map((item, index) => {
                  const currentValue = (() => {
                    switch (sortBy) {
                      case 'total_sold': return item.total_sold
                      case 'revenue': return item.revenue
                      case 'bills_featured': return item.bills_featured
                      case 'avg_price': return item.avg_price
                      default: return 0
                    }
                  })()
                  
                  const progressValue = maxValue > 0 ? (currentValue / maxValue) * 100 : 0
                  
                  return (
                    <div key={item.item_id} className="p-4 hover:bg-default-50 transition-colors">
                      <div className="flex items-center justify-between mb-3">
                        <div className="flex items-center gap-3">
                          <div className="flex items-center gap-2">
                            <span className="text-lg font-bold text-default-400">
                              #{index + 1}
                            </span>
                            <Chip
                              size="sm"
                              color={getPerformanceColor(item.popularity_rank)}
                              variant="flat"
                              startContent={getPerformanceIcon(item.popularity_rank)}
                            >
                              {tString('rank')} {item.popularity_rank}
                            </Chip>
                          </div>
                          
                          <div>
                            <h4 className="font-semibold text-lg">{item.item_name}</h4>
                            <p className="text-sm text-default-600">{item.category}</p>
                          </div>
                        </div>
                        
                        <div className="text-right">
                          <p className="text-lg font-bold">
                            {sortBy === 'revenue' || sortBy === 'avg_price' 
                              ? formatCurrency(currentValue)
                              : currentValue.toLocaleString()
                            }
                          </p>
                          <p className="text-sm text-default-600">
                            {sortOptions.find(opt => opt.key === sortBy)?.label}
                          </p>
                        </div>
                      </div>
                      
                      <Progress 
                        value={progressValue}
                        color={getPerformanceColor(item.popularity_rank)}
                        size="sm"
                        className="mb-3"
                      />
                      
                      <div className="grid grid-cols-4 gap-4 text-sm">
                        <div className="text-center">
                          <p className="font-semibold text-primary">{item.total_sold}</p>
                          <p className="text-xs text-default-600">{tString('sold')}</p>
                        </div>
                        <div className="text-center">
                          <p className="font-semibold text-success">{formatCurrency(item.revenue)}</p>
                          <p className="text-xs text-default-600">{tString('sortOptions.revenue')}</p>
                        </div>
                        <div className="text-center">
                          <p className="font-semibold text-warning">{item.bills_featured}</p>
                          <p className="text-xs text-default-600">{tString('bills')}</p>
                        </div>
                        <div className="text-center">
                          <p className="font-semibold text-secondary">{formatCurrency(item.avg_price)}</p>
                          <p className="text-xs text-default-600">{tString('avgPrice')}</p>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </CardBody>
          </Card>

          {/* Summary Stats */}
          <Card>
            <CardHeader>
              <h3 className="text-lg font-semibold">{tString('summaryStats')}</h3>
            </CardHeader>
            <Divider />
            <CardBody className="p-4">
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="text-center p-4 bg-primary/10 rounded-lg">
                  <Package className="w-8 h-8 mx-auto mb-2 text-primary" />
                  <p className="text-2xl font-bold text-primary">{items.length}</p>
                  <p className="text-sm text-default-600">{tString('totalItems')}</p>
                </div>
                
                <div className="text-center p-4 bg-success/10 rounded-lg">
                  <DollarSign className="w-8 h-8 mx-auto mb-2 text-success" />
                  <p className="text-2xl font-bold text-success">
                    {formatCurrency(items.reduce((sum, item) => sum + item.revenue, 0))}
                  </p>
                  <p className="text-sm text-default-600">{tString('totalRevenue')}</p>
                </div>
                
                <div className="text-center p-4 bg-warning/10 rounded-lg">
                  <TrendingUp className="w-8 h-8 mx-auto mb-2 text-warning" />
                  <p className="text-2xl font-bold text-warning">
                    {items.reduce((sum, item) => sum + item.total_sold, 0)}
                  </p>
                  <p className="text-sm text-default-600">{tString('itemsSold')}</p>
                </div>
                
                <div className="text-center p-4 bg-secondary/10 rounded-lg">
                  <BarChart3 className="w-8 h-8 mx-auto mb-2 text-secondary" />
                  <p className="text-2xl font-bold text-secondary">
                    {formatCurrency(
                      items.length > 0 
                        ? items.reduce((sum, item) => sum + item.revenue, 0) / items.length
                        : 0
                    )}
                  </p>
                  <p className="text-sm text-default-600">{tString('avgRevenuePerItem')}</p>
                </div>
              </div>
            </CardBody>
          </Card>
        </>
      )}
    </div>
  )
}
