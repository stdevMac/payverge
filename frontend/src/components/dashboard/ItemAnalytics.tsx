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
          <p className="text-red-800 font-medium">Error loading item analytics: {error}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header with Controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
          <p className="text-gray-600 font-light text-sm">{tString('subtitle')}</p>
        </div>
        
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="space-y-1">
            <label className="text-xs text-gray-500 uppercase tracking-wide">{tString('period')}</label>
            <select
              value={period}
              onChange={(e) => setPeriod(e.target.value)}
              className="px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm"
            >
              {periods.map((p) => (
                <option key={p.key} value={p.key}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>
          
          <div className="space-y-1">
            <label className="text-xs text-gray-500 uppercase tracking-wide">{tString('sortBy')}</label>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value)}
              className="px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm"
            >
              {sortOptions.map((option) => (
                <option key={option.key} value={option.key}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {items.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <Package className="w-8 h-8 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString('noItemData')}</h3>
          <p className="text-gray-600 font-light text-sm">{tString('noSalesData')}</p>
        </div>
      ) : (
        <>
          {/* Top Categories */}
          {topCategories.length > 0 && (
            <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
              <div className="border-b border-gray-200 p-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                    <BarChart3 className="w-5 h-5 text-gray-700" />
                  </div>
                  <div>
                    <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('topCategories')}</h3>
                  </div>
                </div>
              </div>
              <div className="p-4">
                <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">
                  {topCategories.map(([category, stats], index) => (
                    <div key={category} className="text-center p-4 bg-gray-50 rounded-lg">
                      <div className="flex items-center justify-center gap-2 mb-2">
                        <span className="text-lg font-semibold text-blue-600">#{index + 1}</span>
                        <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
                          {stats.item_count} {tString('items')}
                        </span>
                      </div>
                      <p className="font-semibold text-sm mb-1 text-gray-900">{category}</p>
                      <p className="text-xs text-green-600 font-medium">
                        {formatCurrency(stats.revenue)}
                      </p>
                      <p className="text-xs text-gray-600">
                        {stats.total_sold} {tString('sold')}
                      </p>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* Item Performance List */}
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
            <div className="border-b border-gray-200 p-4">
              <div className="flex justify-between items-center">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                    <Package className="w-5 h-5 text-gray-700" />
                  </div>
                  <div>
                    <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('itemPerformance')}</h3>
                  </div>
                </div>
                <div className="text-sm text-gray-600">
                  {items.length} {tString('totalItems')}
                </div>
              </div>
            </div>
            <div className="p-4">
              <div className="space-y-4">
                {sortedItems.map((item, index) => {
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
                  const rank = index + 1
                  
                  return (
                    <div key={item.item_name} className="p-4 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors">
                      <div className="flex items-center justify-between mb-3">
                        <div className="flex items-center gap-3">
                          <div className="flex items-center gap-2">
                            <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                              rank <= 3 ? 'bg-green-100 text-green-700' :
                              rank <= 10 ? 'bg-yellow-100 text-yellow-700' :
                              'bg-gray-100 text-gray-700'
                            }`}>
                              {rank <= 3 ? <TrendingUp className="w-3 h-3 mr-1" /> :
                               rank <= 10 ? <BarChart3 className="w-3 h-3 mr-1" /> :
                               <TrendingDown className="w-3 h-3 mr-1" />}
                              #{rank}
                            </span>
                          </div>
                          <div>
                            <h4 className="font-semibold text-gray-900">{item.item_name}</h4>
                            <p className="text-sm text-gray-600">{item.category}</p>
                          </div>
                        </div>
                        
                        <div className="text-right">
                          <p className="font-semibold text-lg text-gray-900">
                            {sortBy === 'revenue' || sortBy === 'avg_price' 
                              ? formatCurrency(currentValue)
                              : currentValue.toLocaleString()
                            }
                          </p>
                          <p className="text-xs text-gray-500 uppercase tracking-wide">
                            {sortOptions.find(opt => opt.key === sortBy)?.label}
                          </p>
                        </div>
                      </div>
                      
                      <div className="w-full bg-gray-200 rounded-full h-2 mb-3">
                        <div
                          className={`h-2 rounded-full transition-all duration-300 ${
                            rank <= 3 ? 'bg-green-500' :
                            rank <= 10 ? 'bg-yellow-500' :
                            'bg-gray-400'
                          }`}
                          style={{ width: `${progressValue}%` }}
                        />
                      </div>
                      
                      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                        <div>
                          <p className="text-gray-500 text-xs uppercase tracking-wide">{tString('metrics.sold')}</p>
                          <p className="font-semibold text-gray-900">{item.total_sold}</p>
                        </div>
                        <div>
                          <p className="text-gray-500 text-xs uppercase tracking-wide">{tString('metrics.revenue')}</p>
                          <p className="font-semibold text-green-600">{formatCurrency(item.revenue)}</p>
                        </div>
                        <div>
                          <p className="text-gray-500 text-xs uppercase tracking-wide">{tString('metrics.avgPrice')}</p>
                          <p className="font-semibold text-gray-900">{formatCurrency(item.avg_price)}</p>
                        </div>
                        <div>
                          <p className="text-gray-500 text-xs uppercase tracking-wide">{tString('metrics.billsFeatured')}</p>
                          <p className="font-semibold text-blue-600">{item.bills_featured}</p>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          </div>

          {/* Summary Stats */}
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
            <div className="border-b border-gray-200 p-4">
              <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('summaryStats')}</h3>
            </div>
            <div className="p-4">
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="text-center">
                  <p className="text-2xl font-semibold text-blue-600">{items.length}</p>
                  <p className="text-sm text-gray-600">{tString('totalItems')}</p>
                </div>
                
                <div className="text-center">
                  <p className="text-2xl font-semibold text-green-600">
                    {formatCurrency(items.reduce((sum, item) => sum + item.revenue, 0))}
                  </p>
                  <p className="text-sm text-gray-600">{tString('totalRevenue')}</p>
                </div>
                
                <div className="text-center">
                  <p className="text-2xl font-semibold text-yellow-600">
                    {items.reduce((sum, item) => sum + item.total_sold, 0)}
                  </p>
                  <p className="text-sm text-gray-600">{tString('itemsSold')}</p>
                </div>
                
                <div className="text-center">
                  <p className="text-2xl font-semibold text-purple-600">
                    {formatCurrency(
                      items.length > 0 
                        ? items.reduce((sum, item) => sum + item.revenue, 0) / items.length
                        : 0
                    )}
                  </p>
                  <p className="text-sm text-gray-600">{tString('avgRevenuePerItem')}</p>
                </div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
