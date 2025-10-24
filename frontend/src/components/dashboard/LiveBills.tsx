'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
import { getLiveBills } from '@/api/dashboard'
import { Card, CardBody, CardHeader, Divider, Spinner, Badge, Button, Chip } from '@nextui-org/react'
import { Clock, DollarSign, Users, RefreshCw, Eye } from 'lucide-react'
import { useWebSocket } from '@/hooks/useWebSocket'

interface LiveBill {
  id: number
  bill_number: string
  table_name: string
  table_code: string
  total_amount: number
  paid_amount: number
  remaining_amount: number
  tip_amount: number
  status: string
  created_at: string
  updated_at: string
}

interface LiveBillsProps {
  businessId: string
}

export default function LiveBills({ businessId }: LiveBillsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.liveBills.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [bills, setBills] = useState<LiveBill[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date())

  // WebSocket connection for real-time updates
  const { isConnected, sendMessage } = useWebSocket({
    url: process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080/ws',
    onMessage: (data: any) => {
      if (data.type === 'bill_updated' && data.businessId === businessId) {
        fetchLiveBills()
      }
    },
  })

  const fetchLiveBills = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await getLiveBills(parseInt(businessId))
      console.log('LiveBills - Raw API response:', data)
      console.log('LiveBills - Data type:', typeof data, Array.isArray(data))
      setBills(Array.isArray(data) ? data : [])
      setLastUpdated(new Date())
    } catch (err) {
      console.error('LiveBills - API Error:', err)
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [businessId])

  useEffect(() => {
    fetchLiveBills()
    
    // Subscribe to business updates via WebSocket
    if (isConnected) {
      sendMessage({
        type: 'subscribe',
        room: `business_${businessId}`,
      })
    }

    // Refresh every 30 seconds as fallback
    const interval = setInterval(fetchLiveBills, 30000)
    return () => clearInterval(interval)
  }, [businessId, isConnected, fetchLiveBills, sendMessage])

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(amount)
  }

  const formatTime = (dateString: string) => {
    const date = new Date(dateString)
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  const getTimeElapsed = (dateString: string) => {
    const now = new Date()
    const created = new Date(dateString)
    const diffMs = now.getTime() - created.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    
    if (diffMins < 1) return tString('timeElapsed.justNow')
    if (diffMins < 60) return `${diffMins}${tString('timeElapsed.minutesAgo')}`
    
    const diffHours = Math.floor(diffMins / 60)
    if (diffHours < 24) return `${diffHours}${tString('timeElapsed.hoursAgo')}`
    
    const diffDays = Math.floor(diffHours / 24)
    return `${diffDays}${tString('timeElapsed.daysAgo')}`
  }

  const getStatusColor = (bill: LiveBill) => {
    if (bill.paid_amount >= bill.total_amount) return 'success'
    if (bill.paid_amount > 0) return 'warning'
    return 'default'
  }

  const getStatusText = (bill: LiveBill) => {
    if (bill.paid_amount >= bill.total_amount) return tString('status.paid')
    if (bill.paid_amount > 0) return tString('status.partial')
    return tString('status.unpaid')
  }

  const getPaymentProgress = (bill: LiveBill) => {
    return bill.total_amount > 0 ? (bill.paid_amount / bill.total_amount) * 100 : 0
  }

  if (loading && bills.length === 0) {
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

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
            <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
              {bills.length}
            </span>
            {isConnected && (
              <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-700">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-1 animate-pulse"></div>
                {tString('live')}
              </span>
            )}
          </div>
          <p className="text-gray-600 font-light text-sm">{tString('subtitle')}</p>
        </div>
        
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-500">
            {tString('lastUpdated')}: {lastUpdated.toLocaleTimeString()}
          </span>
          <button
            onClick={fetchLiveBills}
            disabled={loading}
            className="inline-flex items-center gap-2 px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            {tString('refresh')}
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4">
          <div className="flex items-center gap-3">
            <Clock className="w-5 h-5 text-red-600 flex-shrink-0" />
            <p className="text-red-800 font-medium">{tString('error')}: {error}</p>
          </div>
        </div>
      )}

      {bills.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <Users className="w-8 h-8 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString('noActiveBills')}</h3>
          <p className="text-gray-600 font-light text-sm">{tString('noActiveBillsDesc')}</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {bills.map((bill) => (
            <div key={bill.id} className="bg-white border border-gray-200 rounded-lg shadow-sm hover:shadow-md transition-all duration-200">
              <div className="border-b border-gray-200 p-4">
                <div className="flex justify-between items-start">
                  <div>
                    <h3 className="font-semibold text-lg text-gray-900">#{bill.bill_number}</h3>
                    <div className="flex items-center gap-2 mt-1">
                      <DollarSign className="w-4 h-4 text-gray-500" />
                      <p className="text-sm font-medium text-gray-600">{bill.table_name}</p>
                    </div>
                  </div>
                  <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                    getStatusColor(bill) === 'success' ? 'bg-green-100 text-green-700' :
                    getStatusColor(bill) === 'warning' ? 'bg-yellow-100 text-yellow-700' :
                    'bg-gray-100 text-gray-700'
                  }`}>
                    {getStatusText(bill)}
                  </span>
                </div>
              </div>
              
              <div className="p-4">
                <div className="space-y-4">
                  {/* Amount Details */}
                  <div className="space-y-3">
                    <div className="flex justify-between items-center p-3 bg-gray-50 rounded-lg">
                      <span className="text-sm font-medium text-gray-600">{tString('amounts.totalAmount')}</span>
                      <span className="font-semibold text-lg text-gray-900">{formatCurrency(bill.total_amount)}</span>
                    </div>
                    
                    <div className="flex justify-between items-center p-3 bg-green-50 rounded-lg">
                      <span className="text-sm font-medium text-green-700">{tString('amounts.paidAmount')}</span>
                      <span className="font-semibold text-lg text-green-700">
                        {formatCurrency(bill.paid_amount)}
                      </span>
                    </div>
                    
                    {bill.remaining_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-gray-600">{tString('amounts.remaining')}</span>
                        <span className="font-semibold text-yellow-600">
                          {formatCurrency(bill.remaining_amount)}
                        </span>
                      </div>
                    )}
                    
                    {bill.tip_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-gray-600">{tString('amounts.tips')}</span>
                        <span className="font-semibold text-blue-600">
                          {formatCurrency(bill.tip_amount)}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* Payment Progress Bar */}
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-gray-500">
                      <span>{tString('paymentProgress')}</span>
                      <span>{Math.round(getPaymentProgress(bill))}%</span>
                    </div>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          getPaymentProgress(bill) === 100 
                            ? 'bg-green-500' 
                            : getPaymentProgress(bill) > 0 
                              ? 'bg-yellow-500' 
                              : 'bg-gray-300'
                        }`}
                        style={{ width: `${Math.min(getPaymentProgress(bill), 100)}%` }}
                      />
                    </div>
                  </div>

                  {/* Time Info */}
                  <div className="flex items-center justify-between text-xs text-gray-500">
                    <div className="flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      <span>{tString('created')} {formatTime(bill.created_at)}</span>
                    </div>
                    <span>{getTimeElapsed(bill.created_at)}</span>
                  </div>

                  {/* Actions */}
                  <div className="flex gap-2 pt-2">
                    <button
                      onClick={() => {
                        // Navigate to bill details or open modal
                        window.open(`/t/${bill.table_code}/bill`, '_blank')
                      }}
                      className="flex-1 inline-flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-gray-700 bg-gray-50 border border-gray-200 rounded-lg hover:bg-gray-100"
                    >
                      <Eye className="w-4 h-4" />
                      {tString('view')}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Summary Stats */}
      {bills.length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('quickStats.title')}</h3>
          </div>
          <div className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-semibold text-blue-600">{bills.length}</p>
                <p className="text-sm text-gray-600">{tString('quickStats.activeBills')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-semibold text-green-600">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.total_amount, 0))}
                </p>
                <p className="text-sm text-gray-600">{tString('quickStats.totalValue')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-semibold text-yellow-600">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.remaining_amount, 0))}
                </p>
                <p className="text-sm text-gray-600">{tString('quickStats.outstanding')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-semibold text-purple-600">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.tip_amount, 0))}
                </p>
                <p className="text-sm text-gray-600">{tString('quickStats.tipsCollected')}</p>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
