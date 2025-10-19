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
      <Card className="w-full">
        <CardBody className="flex items-center justify-center p-8">
          <Spinner size="lg" />
        </CardBody>
      </Card>
    )
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-3xl font-bold text-default-900">{tString('title')}</h2>
            <Badge 
              content={bills.length} 
              color={bills.length > 0 ? 'primary' : 'default'}
              size="lg"
              className="text-sm"
            >
              <div className="w-6 h-6" />
            </Badge>
            {isConnected && (
              <Chip size="md" color="success" variant="dot" className="animate-pulse">
                {tString('live')}
              </Chip>
            )}
          </div>
          <p className="text-default-600 mt-1">{tString('subtitle')}</p>
        </div>
        
        <div className="flex items-center gap-3">
          <span className="text-sm text-default-500">
            {tString('lastUpdated')}: {lastUpdated.toLocaleTimeString()}
          </span>
          <Button
            size="md"
            variant="bordered"
            onPress={fetchLiveBills}
            isLoading={loading}
            startContent={<RefreshCw className="w-4 h-4" />}
          >
            {tString('refresh')}
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-danger-200 bg-danger-50">
          <CardBody className="p-4">
            <div className="text-danger">
              <p>{tString('error')}: {error}</p>
            </div>
          </CardBody>
        </Card>
      )}

      {bills.length === 0 ? (
        <Card className="shadow-md">
          <CardBody className="p-12">
            <div className="text-center text-default-500">
              <div className="w-20 h-20 mx-auto mb-6 bg-default-100 rounded-full flex items-center justify-center">
                <Users className="w-10 h-10 text-default-400" />
              </div>
              <h3 className="text-xl font-semibold mb-2 text-default-700">{tString('noActiveBills')}</h3>
              <p className="text-default-500">{tString('noActiveBillsDesc')}</p>
            </div>
          </CardBody>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {bills.map((bill) => (
            <Card key={bill.id} className="border-none shadow-md hover:shadow-lg transition-all duration-300 hover:scale-[1.02]">
              <CardHeader className="pb-3">
                <div className="flex justify-between items-start w-full">
                  <div>
                    <h3 className="font-bold text-xl text-default-900">#{bill.bill_number}</h3>
                    <div className="flex items-center gap-2 mt-1">
                      <DollarSign className="w-4 h-4 text-default-500" />
                      <p className="text-sm font-medium text-default-600">{bill.table_name}</p>
                    </div>
                  </div>
                  <Chip
                    size="md"
                    color={getStatusColor(bill)}
                    variant="flat"
                    className="font-semibold"
                  >
                    {getStatusText(bill)}
                  </Chip>
                </div>
              </CardHeader>
              
              <Divider />
              
              <CardBody className="pt-4">
                <div className="space-y-5">
                  {/* Amount Details */}
                  <div className="space-y-3">
                    <div className="flex justify-between items-center p-3 bg-default-50 rounded-lg">
                      <span className="text-sm font-medium text-default-600">{tString('amounts.totalAmount')}</span>
                      <span className="font-bold text-lg text-default-900">{formatCurrency(bill.total_amount)}</span>
                    </div>
                    
                    <div className="flex justify-between items-center p-3 bg-success-50 rounded-lg">
                      <span className="text-sm font-medium text-success-700">{tString('amounts.paidAmount')}</span>
                      <span className="font-bold text-lg text-success-700">
                        {formatCurrency(bill.paid_amount)}
                      </span>
                    </div>
                    
                    {bill.remaining_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-default-600">{tString('amounts.remaining')}</span>
                        <span className="font-semibold text-warning">
                          {formatCurrency(bill.remaining_amount)}
                        </span>
                      </div>
                    )}
                    
                    {bill.tip_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-default-600">{tString('amounts.tips')}</span>
                        <span className="font-semibold text-secondary">
                          {formatCurrency(bill.tip_amount)}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* Payment Progress Bar */}
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-default-500">
                      <span>{tString('paymentProgress')}</span>
                      <span>{Math.round(getPaymentProgress(bill))}%</span>
                    </div>
                    <div className="w-full bg-default-200 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          getPaymentProgress(bill) === 100 
                            ? 'bg-success' 
                            : getPaymentProgress(bill) > 0 
                              ? 'bg-warning' 
                              : 'bg-default-300'
                        }`}
                        style={{ width: `${Math.min(getPaymentProgress(bill), 100)}%` }}
                      />
                    </div>
                  </div>

                  {/* Time Info */}
                  <div className="flex items-center justify-between text-xs text-default-500">
                    <div className="flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      <span>{tString('created')} {formatTime(bill.created_at)}</span>
                    </div>
                    <span>{getTimeElapsed(bill.created_at)}</span>
                  </div>

                  {/* Actions */}
                  <div className="flex gap-2 pt-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      startContent={<Eye className="w-4 h-4" />}
                      className="flex-1"
                      onPress={() => {
                        // Navigate to bill details or open modal
                        window.open(`/t/${bill.table_code}/bill`, '_blank')
                      }}
                    >
                      {tString('view')}
                    </Button>
                  </div>
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      )}

      {/* Summary Stats */}
      {bills.length > 0 && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">{tString('quickStats.title')}</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-bold text-primary">{bills.length}</p>
                <p className="text-sm text-default-600">{tString('quickStats.activeBills')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-success">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.total_amount, 0))}
                </p>
                <p className="text-sm text-default-600">{tString('quickStats.totalValue')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-warning">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.remaining_amount, 0))}
                </p>
                <p className="text-sm text-default-600">{tString('quickStats.outstanding')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-secondary">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.tip_amount, 0))}
                </p>
                <p className="text-sm text-default-600">{tString('quickStats.tipsCollected')}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
