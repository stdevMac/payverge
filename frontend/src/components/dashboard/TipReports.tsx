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

  if (!tipData) {
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

      {/* Key Metrics */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
        <Card className="border-none shadow-md hover:shadow-lg transition-shadow duration-200">
          <CardBody className="p-6">
            <div className="flex items-center gap-4">
              <div className="p-3 rounded-xl bg-success/10">
                <DollarSign className="w-6 h-6 text-success" />
              </div>
              <div className="flex-1">
                <p className="text-sm font-medium text-default-600 uppercase tracking-wide">{tString('metrics.totalTips')}</p>
                <p className="text-2xl font-bold text-default-900 mt-1">{formatCurrency(tipData.total_tips || 0)}</p>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border-none shadow-md hover:shadow-lg transition-shadow duration-200">
          <CardBody className="p-6">
            <div className="flex items-center gap-4">
              <div className="p-3 rounded-xl bg-primary/10">
                <TrendingUp className="w-6 h-6 text-primary" />
              </div>
              <div className="flex-1">
                <p className="text-sm font-medium text-default-600 uppercase tracking-wide">{tString('metrics.averageTip')}</p>
                <p className="text-2xl font-bold text-default-900 mt-1">{formatCurrency(tipData.average_tip || 0)}</p>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border-none shadow-sm">
          <CardBody className="p-4">
            <div className="flex items-center gap-3">
              <div className={`p-2 rounded-lg ${getTipRateColor(tipData.average_tip_rate) === 'success' ? 'bg-success/10' : getTipRateColor(tipData.average_tip_rate) === 'warning' ? 'bg-warning/10' : 'bg-danger/10'}`}>
                <TrendingUp className={`w-5 h-5 ${getTipRateColor(tipData.average_tip_rate) === 'success' ? 'text-success' : getTipRateColor(tipData.average_tip_rate) === 'warning' ? 'text-warning' : 'text-danger'}`} />
              </div>
              <div className="flex-1">
                <p className="text-sm text-default-600">{tString('metrics.tipRate')}</p>
                <div className="flex items-center gap-2">
                  <p className="text-xl font-semibold">{formatPercentage(tipData.average_tip_rate)}</p>
                  <Chip
                    size="sm"
                    color={getTipRateColor(tipData.average_tip_rate)}
                    variant="flat"
                  >
                    {getTipRateLabel(tipData.average_tip_rate)}
                  </Chip>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border-none shadow-sm">
          <CardBody className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-secondary/10">
                <Users className="w-5 h-5 text-secondary" />
              </div>
              <div className="flex-1">
                <p className="text-sm text-default-600">{tString('metrics.tipTransactions')}</p>
                <p className="text-xl font-semibold">{tipData.tip_count}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Daily Comparison */}
      {tipData.daily_comparison && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">{tString('dailyComparison.title')}</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-3 gap-4">
              <div className="text-center">
                <p className="text-sm text-default-600">{tString('dailyComparison.today')}</p>
                <p className="text-2xl font-bold text-primary">
                  {formatCurrency(tipData.daily_comparison.today)}
                </p>
              </div>
              
              <div className="text-center">
                <p className="text-sm text-default-600">{tString('dailyComparison.yesterday')}</p>
                <p className="text-2xl font-bold text-default-600">
                  {formatCurrency(tipData.daily_comparison.yesterday)}
                </p>
              </div>
              
              <div className="text-center">
                <p className="text-sm text-default-600">{tString('dailyComparison.change')}</p>
                <div className="flex items-center justify-center gap-2">
                  {tipData.daily_comparison.change_percentage >= 0 ? (
                    <TrendingUp className="w-5 h-5 text-success" />
                  ) : (
                    <TrendingDown className="w-5 h-5 text-danger" />
                  )}
                  <p className={`text-2xl font-bold ${tipData.daily_comparison.change_percentage >= 0 ? 'text-success' : 'text-danger'}`}>
                    {formatPercentage(Math.abs(tipData.daily_comparison.change_percentage))}
                  </p>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Tip Distribution */}
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">{tString('tipDistribution.title')}</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="space-y-4">
              {tipDistributionEntries.map(([range, count]) => {
                const percentage = totalTipTransactions > 0 ? (count / totalTipTransactions) * 100 : 0
                return (
                  <div key={range} className="space-y-2">
                    <div className="flex justify-between items-center">
                      <span className="font-medium">{range}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-default-600">{count} {tString('tipDistribution.tips')}</span>
                        <span className="font-semibold">{formatPercentage(percentage)}</span>
                      </div>
                    </div>
                    <Progress 
                      value={percentage}
                      color={percentage > 30 ? 'success' : percentage > 15 ? 'warning' : 'default'}
                      size="sm"
                    />
                  </div>
                )
              })}
            </div>
          </CardBody>
        </Card>

        {/* Hourly Tips */}
        {Object.keys(tipData.hourly_tips || {}).length > 0 && (
          <Card>
            <CardHeader>
              <h3 className="text-lg font-semibold">{tString('hourlyTips.title')}</h3>
            </CardHeader>
            <Divider />
            <CardBody className="p-4">
              <div className="grid grid-cols-3 gap-3">
                {Object.entries(tipData.hourly_tips || {})
                  .sort(([a], [b]) => parseInt(a) - parseInt(b))
                  .map(([hour, amount]) => {
                    const maxHourlyTip = Math.max(...Object.values(tipData.hourly_tips || {}))
                    const percentage = maxHourlyTip > 0 ? (amount / maxHourlyTip) * 100 : 0
                    
                    return (
                      <div key={hour} className="text-center p-3 bg-default-50 rounded-lg">
                        <p className="text-sm font-medium">{hour}:00</p>
                        <p className="text-lg font-bold text-success">{formatCurrency(amount)}</p>
                        <Progress 
                          value={percentage}
                          color="success"
                          size="sm"
                          className="mt-2"
                        />
                      </div>
                    )
                  })}
              </div>
            </CardBody>
          </Card>
        )}
      </div>

      {/* Top Tippers */}
      {tipData.top_tippers && tipData.top_tippers.length > 0 && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Award className="w-5 h-5 text-warning" />
              <h3 className="text-lg font-semibold">{tString('topTippers.title')}</h3>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="p-0">
            <Table aria-label="Top tippers table">
              <TableHeader>
                <TableColumn>{tString('topTippers.rank')}</TableColumn>
                <TableColumn>{tString('topTippers.customer')}</TableColumn>
                <TableColumn>{tString('topTippers.totalTips')}</TableColumn>
                <TableColumn>{tString('topTippers.tipCount')}</TableColumn>
                <TableColumn>{tString('topTippers.averageTip')}</TableColumn>
              </TableHeader>
              <TableBody>
                {tipData.top_tippers.slice(0, 10).map((tipper, index) => (
                  <TableRow key={tipper.payer_address}>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-lg">#{index + 1}</span>
                        {index < 3 && (
                          <Award className={`w-4 h-4 ${index === 0 ? 'text-warning' : index === 1 ? 'text-default-400' : 'text-orange-600'}`} />
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-mono text-sm">
                        {truncateAddress(tipper.payer_address)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-semibold text-success">
                        {formatCurrency(tipper.total_tips)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-default-600">
                        {tipper.tip_count}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-semibold">
                        {formatCurrency(tipper.average_tip)}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
