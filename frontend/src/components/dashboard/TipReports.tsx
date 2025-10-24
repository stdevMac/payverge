'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
import {
  Card,
  CardHeader,
  CardBody,
  Divider,
  Select,
  SelectItem,
  Spinner,
  Progress,
  Chip,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell
} from '@nextui-org/react'
import { TrendingUp, TrendingDown, DollarSign, Users, Clock, Award, Target } from 'lucide-react'
import analyticsApi, { TipAnalytics } from '@/api/analytics'

interface TipReportsProps {
  businessId: string
}

export default function TipReports({ businessId }: TipReportsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tipReports.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [tipData, setTipData] = useState<TipAnalytics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [period, setPeriod] = useState('week')

  const fetchTipAnalytics = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await analyticsApi.getTipAnalytics(businessId, period)
      console.log('TipReports - Raw API response:', data)
      console.log('TipReports - Data properties:', Object.keys(data || {}))
      setTipData(data)
    } catch (err) {
      console.error('TipReports - API Error:', err)
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [businessId, period])

  useEffect(() => {
    fetchTipAnalytics()
  }, [fetchTipAnalytics, period])

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(amount)
  }

  const formatPercentage = (value: number) => {
    return `${value.toFixed(1)}%`
  }

  const truncateAddress = (address: string) => {
    if (address.length <= 10) return address
    return `${address.slice(0, 6)}...${address.slice(-4)}`
  }

  const getTipRateColor = (rate: number) => {
    if (rate >= 20) return 'success'
    if (rate >= 15) return 'warning'
    return 'danger'
  }

  const getTipRateLabel = (rate: number) => {
    if (rate >= 20) return tString('tipRateLabels.excellent')
    if (rate >= 15) return tString('tipRateLabels.good')
    if (rate >= 10) return tString('tipRateLabels.average')
    return tString('tipRateLabels.belowAverage')
  }

  const periods = [
    { key: 'today', label: tString('periods.today') },
    { key: 'yesterday', label: tString('periods.yesterday') },
    { key: 'week', label: tString('periods.week') },
    { key: 'month', label: tString('periods.month') },
    { key: 'quarter', label: tString('periods.quarter') },
  ]

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

  if (!tipData) {
    return (
      <div className="text-center py-12">
        <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
          <DollarSign className="w-8 h-8 text-gray-400" />
        </div>
        <p className="text-gray-600 font-light tracking-wide">{tString('noData')}</p>
      </div>
    )
  }

  const tipDistributionEntries = Object.entries(tipData.tip_distribution || {})
    .sort(([a], [b]) => {
      // Sort by tip range
      const getMinValue = (range: string) => {
        const match = range.match(/\d+/)
        return match ? parseInt(match[0], 10) : 0
      }
      return getMinValue(a) - getMinValue(b)
    })

  const totalTipTransactions = Object.values(tipData.tip_distribution || {}).reduce((sum, count) => sum + (typeof count === 'number' ? count : 0), 0)

  return (
    <div className="space-y-6">
      {/* Header with Period Selector */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
          <p className="text-gray-600 font-light text-sm">{tString('subtitle')}</p>
        </div>
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
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-4 hover:shadow-md transition-shadow duration-200">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <DollarSign className="w-5 h-5 text-gray-700" />
            </div>
            <div className="flex-1">
              <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('metrics.totalTips')}</p>
              <p className="text-lg font-semibold text-gray-900 mt-1">{formatCurrency(tipData.total_tips || 0)}</p>
            </div>
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-4 hover:shadow-md transition-shadow duration-200">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <TrendingUp className="w-5 h-5 text-gray-700" />
            </div>
            <div className="flex-1">
              <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('metrics.averageTip')}</p>
              <p className="text-lg font-semibold text-gray-900 mt-1">{formatCurrency(tipData.average_tip || 0)}</p>
            </div>
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <Target className="w-5 h-5 text-gray-700" />
            </div>
            <div className="flex-1">
              <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('metrics.tipRate')}</p>
              <div className="flex items-center gap-2 mt-1">
                <p className="text-lg font-semibold text-gray-900">{formatPercentage(tipData.average_tip_rate)}</p>
                <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                  tipData.average_tip_rate >= 20 ? 'bg-green-100 text-green-700' :
                  tipData.average_tip_rate >= 15 ? 'bg-yellow-100 text-yellow-700' :
                  'bg-red-100 text-red-700'
                }`}>
                  {getTipRateLabel(tipData.average_tip_rate)}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <Users className="w-5 h-5 text-gray-700" />
            </div>
            <div className="flex-1">
              <p className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('metrics.tipTransactions')}</p>
              <p className="text-lg font-semibold text-gray-900 mt-1">{tipData.tip_count}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Daily Comparison */}
      {tipData.daily_comparison && (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <Clock className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('dailyComparison.title')}</h3>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="grid grid-cols-3 gap-4">
              <div className="text-center">
                <p className="text-sm text-gray-600">{tString('dailyComparison.today')}</p>
                <p className="text-2xl font-semibold text-blue-600">
                  {formatCurrency(tipData.daily_comparison.today)}
                </p>
              </div>
              
              <div className="text-center">
                <p className="text-sm text-gray-600">{tString('dailyComparison.yesterday')}</p>
                <p className="text-2xl font-semibold text-gray-600">
                  {formatCurrency(tipData.daily_comparison.yesterday)}
                </p>
              </div>
              
              <div className="text-center">
                <p className="text-sm text-gray-600">{tString('dailyComparison.change')}</p>
                <div className="flex items-center justify-center gap-2">
                  {tipData.daily_comparison.change_percentage >= 0 ? (
                    <TrendingUp className="w-5 h-5 text-green-600" />
                  ) : (
                    <TrendingDown className="w-5 h-5 text-red-600" />
                  )}
                  <p className={`text-2xl font-semibold ${tipData.daily_comparison.change_percentage >= 0 ? 'text-green-600' : 'text-red-600'}`}>
                    {formatPercentage(Math.abs(tipData.daily_comparison.change_percentage))}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Tip Distribution */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <DollarSign className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('tipDistribution.title')}</h3>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-4">
              {tipDistributionEntries.map(([range, count]) => {
                const percentage = totalTipTransactions > 0 ? (count / totalTipTransactions) * 100 : 0
                return (
                  <div key={range} className="space-y-2">
                    <div className="flex justify-between items-center">
                      <span className="font-medium text-gray-900">{range}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-gray-600">{count} {tString('tipDistribution.tips')}</span>
                        <span className="font-semibold text-gray-900">{formatPercentage(percentage)}</span>
                      </div>
                    </div>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          percentage > 30 ? 'bg-green-500' : percentage > 15 ? 'bg-yellow-500' : 'bg-gray-400'
                        }`}
                        style={{ width: `${percentage}%` }}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Hourly Tips */}
        {Object.keys(tipData.hourly_tips || {}).length > 0 && (
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
            <div className="border-b border-gray-200 p-4">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                  <Clock className="w-5 h-5 text-gray-700" />
                </div>
                <div>
                  <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('hourlyTips.title')}</h3>
                </div>
              </div>
            </div>
            <div className="p-4">
              <div className="grid grid-cols-3 gap-3">
                {Object.entries(tipData.hourly_tips || {})
                  .sort(([a], [b]) => parseInt(a) - parseInt(b))
                  .map(([hour, amount]) => {
                    const maxHourlyTip = Math.max(...Object.values(tipData.hourly_tips || {}))
                    const percentage = maxHourlyTip > 0 ? (amount / maxHourlyTip) * 100 : 0
                    
                    return (
                      <div key={hour} className="text-center p-3 bg-gray-50 rounded-lg">
                        <p className="text-sm font-medium text-gray-900">{hour}:00</p>
                        <p className="text-lg font-semibold text-green-600">{formatCurrency(amount)}</p>
                        <div className="w-full bg-gray-200 rounded-full h-2 mt-2">
                          <div
                            className="h-2 rounded-full bg-green-500 transition-all duration-300"
                            style={{ width: `${percentage}%` }}
                          />
                        </div>
                      </div>
                    )
                  })}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Top Tippers */}
      {tipData.top_tippers && tipData.top_tippers.length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <Award className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('topTippers.title')}</h3>
              </div>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('topTippers.rank')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('topTippers.customer')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('topTippers.totalTips')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('topTippers.tipCount')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('topTippers.averageTip')}</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {tipData.top_tippers.slice(0, 10).map((tipper, index) => (
                  <tr key={tipper.payer_address} className="hover:bg-gray-50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-lg text-gray-900">#{index + 1}</span>
                        {index < 3 && (
                          <Award className={`w-4 h-4 ${index === 0 ? 'text-yellow-500' : index === 1 ? 'text-gray-400' : 'text-orange-600'}`} />
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="font-mono text-sm text-gray-600">
                        {truncateAddress(tipper.payer_address)}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="font-semibold text-green-600">
                        {formatCurrency(tipper.total_tips)}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="text-gray-600">
                        {tipper.tip_count}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="font-semibold text-gray-900">
                        {formatCurrency(tipper.average_tip)}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
