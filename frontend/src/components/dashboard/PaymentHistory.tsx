'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { getPaymentHistory, exportPayments } from '@/api/payments'
import { 
  Card, 
  CardBody, 
  CardHeader, 
  Divider, 
  Spinner, 
  Table, 
  TableHeader, 
  TableColumn, 
  TableBody, 
  TableRow, 
  TableCell,
  Chip,
  Button,
  Input,
  Select,
  SelectItem,
  DateRangePicker,
  Pagination
} from '@nextui-org/react'
import { Search, Filter, Download, ExternalLink } from 'lucide-react'
import { parseDate } from '@internationalized/date'

interface Payment {
  id: number
  bill_id: number
  bill_number: string
  table_name: string
  payer_address: string
  amount: number
  tip_amount: number
  tx_hash: string
  status: 'pending' | 'confirmed' | 'failed'
  created_at: string
  updated_at: string
}

interface PaymentHistoryProps {
  businessId: string
}

export default function PaymentHistory({ businessId }: PaymentHistoryProps) {
  const [payments, setPayments] = useState<Payment[]>([])
  const [filteredPayments, setFilteredPayments] = useState<Payment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  
  // Filter states
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [dateRange, setDateRange] = useState<any>(null)
  
  // Pagination
  const [currentPage, setCurrentPage] = useState(1)
  const itemsPerPage = 10

  const fetchPayments = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await getPaymentHistory(parseInt(businessId))
      setPayments(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [businessId])

  useEffect(() => {
    fetchPayments()
  }, [fetchPayments])

  // Apply filters
  useEffect(() => {
    let filtered = [...payments]

    // Search filter
    if (searchTerm) {
      filtered = filtered.filter(payment => 
        payment.bill_number.toLowerCase().includes(searchTerm.toLowerCase()) ||
        payment.table_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
        payment.payer_address.toLowerCase().includes(searchTerm.toLowerCase()) ||
        payment.tx_hash.toLowerCase().includes(searchTerm.toLowerCase())
      )
    }

    // Status filter
    if (statusFilter !== 'all') {
      filtered = filtered.filter(payment => payment.status === statusFilter)
    }

    // Date range filter
    if (dateRange?.start && dateRange?.end) {
      const startDate = new Date(dateRange.start.toString())
      const endDate = new Date(dateRange.end.toString())
      filtered = filtered.filter(payment => {
        const paymentDate = new Date(payment.created_at)
        return paymentDate >= startDate && paymentDate <= endDate
      })
    }

    setFilteredPayments(filtered)
    setCurrentPage(1)
  }, [payments, searchTerm, statusFilter, dateRange])

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(amount)
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'confirmed': return 'success'
      case 'pending': return 'warning'
      case 'failed': return 'danger'
      default: return 'default'
    }
  }

  const truncateAddress = (address: string) => {
    if (address.length <= 10) return address
    return `${address.slice(0, 6)}...${address.slice(-4)}`
  }

  const handleExportPayments = async () => {
    try {
      const blob = await exportPayments(parseInt(businessId), 'custom', 'csv')
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `payments-${businessId}-${new Date().toISOString().split('T')[0]}.csv`
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
      document.body.removeChild(a)
    } catch (err) {
      console.error('Failed to export payments:', err)
    }
  }

  // Pagination
  const totalPages = Math.ceil(filteredPayments.length / itemsPerPage)
  const paginatedPayments = filteredPayments.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  )

  const statusOptions = [
    { key: 'all', label: 'All Status' },
    { key: 'confirmed', label: 'Confirmed' },
    { key: 'pending', label: 'Pending' },
    { key: 'failed', label: 'Failed' },
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

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">Payment History</h2>
        <Button
          color="primary"
          variant="flat"
          startContent={<Download className="w-4 h-4" />}
          onPress={handleExportPayments}
        >
          Export
        </Button>
      </div>

      {/* Filters */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Filter className="w-5 h-5" />
            <h3 className="text-lg font-semibold">Filters</h3>
          </div>
        </CardHeader>
        <Divider />
        <CardBody className="p-4">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <Input
              placeholder="Search bills, tables, addresses..."
              startContent={<Search className="w-4 h-4 text-default-400" />}
              value={searchTerm}
              onValueChange={setSearchTerm}
              isClearable
            />
            
            <Select
              placeholder="Filter by status"
              selectedKeys={[statusFilter]}
              onSelectionChange={(keys) => setStatusFilter(Array.from(keys)[0] as string)}
            >
              {statusOptions.map((option) => (
                <SelectItem key={option.key} value={option.key}>
                  {option.label}
                </SelectItem>
              ))}
            </Select>

            <DateRangePicker
              label="Date Range"
              value={dateRange}
              onChange={setDateRange}
              className="max-w-xs"
            />

            <Button
              variant="flat"
              onPress={() => {
                setSearchTerm('')
                setStatusFilter('all')
                setDateRange(null)
              }}
            >
              Clear Filters
            </Button>
          </div>
        </CardBody>
      </Card>

      {error && (
        <Card className="border-danger-200 bg-danger-50">
          <CardBody className="p-4">
            <div className="text-danger">
              <p>Error loading payment history: {error}</p>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Payment Table */}
      <Card>
        <CardBody className="p-0">
          <Table aria-label="Payment history table">
            <TableHeader>
              <TableColumn>Bill</TableColumn>
              <TableColumn>Table</TableColumn>
              <TableColumn>Amount</TableColumn>
              <TableColumn>Tips</TableColumn>
              <TableColumn>Payer</TableColumn>
              <TableColumn>Status</TableColumn>
              <TableColumn>Date</TableColumn>
              <TableColumn>Actions</TableColumn>
            </TableHeader>
            <TableBody emptyContent="No payments found">
              {paginatedPayments.map((payment) => (
                <TableRow key={payment.id}>
                  <TableCell>
                    <div className="font-medium">#{payment.bill_number}</div>
                  </TableCell>
                  <TableCell>
                    <div className="text-default-600">{payment.table_name}</div>
                  </TableCell>
                  <TableCell>
                    <div className="font-semibold">{formatCurrency(payment.amount)}</div>
                  </TableCell>
                  <TableCell>
                    <div className="text-success">
                      {payment.tip_amount > 0 ? formatCurrency(payment.tip_amount) : '-'}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="font-mono text-sm">
                      {truncateAddress(payment.payer_address)}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="sm"
                      color={getStatusColor(payment.status)}
                      variant="flat"
                    >
                      {payment.status}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <div className="text-sm text-default-600">
                      {formatDate(payment.created_at)}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        isIconOnly
                        size="sm"
                        variant="ghost"
                        onPress={() => {
                          window.open(`https://etherscan.io/tx/${payment.tx_hash}`, '_blank')
                        }}
                      >
                        <ExternalLink className="w-4 h-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex justify-center p-4">
              <Pagination
                total={totalPages}
                page={currentPage}
                onChange={setCurrentPage}
                showControls
              />
            </div>
          )}
        </CardBody>
      </Card>

      {/* Summary Stats */}
      {filteredPayments.length > 0 && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Summary</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-bold text-primary">{filteredPayments.length}</p>
                <p className="text-sm text-default-600">Total Payments</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-success">
                  {formatCurrency(filteredPayments.reduce((sum, p) => sum + p.amount, 0))}
                </p>
                <p className="text-sm text-default-600">Total Amount</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-secondary">
                  {formatCurrency(filteredPayments.reduce((sum, p) => sum + p.tip_amount, 0))}
                </p>
                <p className="text-sm text-default-600">Total Tips</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-warning">
                  {filteredPayments.filter(p => p.status === 'confirmed').length}
                </p>
                <p className="text-sm text-default-600">Confirmed</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
