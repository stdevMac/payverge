'use client'

import React, { useState, useEffect, useCallback } from 'react'
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
      const response = await fetch(`/inside/businesses/${businessId}/analytics/live-bills`, {
        credentials: 'include',
      })
      
      if (!response.ok) {
        throw new Error('Failed to fetch live bills')
      }
      
      const result = await response.json()
      if (result.success) {
        setBills(result.data || [])
        setLastUpdated(new Date())
      } else {
        throw new Error(result.error || 'Failed to fetch live bills')
      }
    } catch (err) {
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
    
    if (diffMins < 1) return 'Just now'
    if (diffMins < 60) return `${diffMins}m ago`
    
    const diffHours = Math.floor(diffMins / 60)
    if (diffHours < 24) return `${diffHours}h ago`
    
    const diffDays = Math.floor(diffHours / 24)
    return `${diffDays}d ago`
  }

  const getStatusColor = (bill: LiveBill) => {
    if (bill.paid_amount >= bill.total_amount) return 'success'
    if (bill.paid_amount > 0) return 'warning'
    return 'default'
  }

  const getStatusText = (bill: LiveBill) => {
    if (bill.paid_amount >= bill.total_amount) return 'Paid'
    if (bill.paid_amount > 0) return 'Partial'
    return 'Unpaid'
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
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-bold">Live Bills</h2>
          <Badge 
            content={bills.length} 
            color={bills.length > 0 ? 'primary' : 'default'}
            size="sm"
          >
            <div className="w-6 h-6" />
          </Badge>
          {isConnected && (
            <Chip size="sm" color="success" variant="dot">
              Live
            </Chip>
          )}
        </div>
        
        <div className="flex items-center gap-2">
          <span className="text-sm text-default-500">
            Updated {lastUpdated.toLocaleTimeString()}
          </span>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            onPress={fetchLiveBills}
            isLoading={loading}
          >
            <RefreshCw className="w-4 h-4" />
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-danger-200 bg-danger-50">
          <CardBody className="p-4">
            <div className="text-danger">
              <p>Error loading live bills: {error}</p>
            </div>
          </CardBody>
        </Card>
      )}

      {bills.length === 0 ? (
        <Card>
          <CardBody className="p-8">
            <div className="text-center text-default-500">
              <Users className="w-12 h-12 mx-auto mb-4 opacity-50" />
              <p className="text-lg font-medium mb-2">No Active Bills</p>
              <p>All bills have been paid or closed.</p>
            </div>
          </CardBody>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {bills.map((bill) => (
            <Card key={bill.id} className="border-none shadow-sm hover:shadow-md transition-shadow">
              <CardHeader className="pb-2">
                <div className="flex justify-between items-start w-full">
                  <div>
                    <h3 className="font-semibold text-lg">#{bill.bill_number}</h3>
                    <p className="text-sm text-default-600">{bill.table_name}</p>
                  </div>
                  <Chip
                    size="sm"
                    color={getStatusColor(bill)}
                    variant="flat"
                  >
                    {getStatusText(bill)}
                  </Chip>
                </div>
              </CardHeader>
              
              <Divider />
              
              <CardBody className="pt-4">
                <div className="space-y-4">
                  {/* Amount Details */}
                  <div className="space-y-2">
                    <div className="flex justify-between items-center">
                      <span className="text-sm text-default-600">Total</span>
                      <span className="font-semibold">{formatCurrency(bill.total_amount)}</span>
                    </div>
                    
                    <div className="flex justify-between items-center">
                      <span className="text-sm text-default-600">Paid</span>
                      <span className="font-semibold text-success">
                        {formatCurrency(bill.paid_amount)}
                      </span>
                    </div>
                    
                    {bill.remaining_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-default-600">Remaining</span>
                        <span className="font-semibold text-warning">
                          {formatCurrency(bill.remaining_amount)}
                        </span>
                      </div>
                    )}
                    
                    {bill.tip_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-default-600">Tips</span>
                        <span className="font-semibold text-secondary">
                          {formatCurrency(bill.tip_amount)}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* Payment Progress Bar */}
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-default-500">
                      <span>Payment Progress</span>
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
                      <span>Created {formatTime(bill.created_at)}</span>
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
                      View
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
            <h3 className="text-lg font-semibold">Quick Stats</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-bold text-primary">{bills.length}</p>
                <p className="text-sm text-default-600">Active Bills</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-success">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.total_amount, 0))}
                </p>
                <p className="text-sm text-default-600">Total Value</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-warning">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.remaining_amount, 0))}
                </p>
                <p className="text-sm text-default-600">Outstanding</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-secondary">
                  {formatCurrency(bills.reduce((sum, bill) => sum + bill.tip_amount, 0))}
                </p>
                <p className="text-sm text-default-600">Tips Collected</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
