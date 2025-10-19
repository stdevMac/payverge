'use client'

import React, { useState, useEffect, useCallback } from 'react'
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider'
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
import { Search, Filter, Download, ExternalLink, X } from 'lucide-react'
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
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.paymentHistory.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

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
      const startDate = new Date(dateRange.start?.toString() || '')
      const endDate = new Date(dateRange.end?.toString() || '')
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
    { key: 'all', label: tString('statusOptions.all') },
    { key: 'confirmed', label: tString('statusOptions.confirmed') },
    { key: 'pending', label: tString('statusOptions.pending') },
    { key: 'failed', label: tString('statusOptions.failed') },
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
    <div className="space-y-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-3xl font-bold text-default-900">{tString('title')}</h2>
          <p className="text-default-600 mt-1">{tString('subtitle')}</p>
        </div>
        <Button
          color="primary"
          size="lg"
          variant="flat"
          startContent={<Download className="w-5 h-5" />}
          onPress={handleExportPayments}
          className="w-full sm:w-auto"
        >
          {tString('exportData')}
        </Button>
      </div>

      {/* Filters */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Filter className="w-5 h-5 text-primary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('searchAndFilter')}</h3>
          </div>
        </CardHeader>
        <Divider />
        <CardBody className="p-6">
          <div className="space-y-6">
            {/* Search Section */}
            <div className="space-y-2">
              <label className="text-sm font-medium text-default-700">{tString('searchPayments')}</label>
              <Input
                placeholder={tString('searchPlaceholder')}
                startContent={<Search className="w-4 h-4 text-default-400" />}
                value={searchTerm}
                onValueChange={setSearchTerm}
                isClearable
                size="lg"
                variant="bordered"
                className="w-full"
                classNames={{
                  input: "text-sm",
                  inputWrapper: "h-12"
                }}
              />
            </div>
            
            {/* Filters Section */}
            <div className="space-y-4">
              <label className="text-sm font-medium text-default-700">{tString('filterOptions')}</label>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="space-y-2">
                  <label className="text-xs text-default-500 uppercase tracking-wide">{tString('status')}</label>
                  <Select
                    placeholder={tString('statusOptions.all')}
                    selectedKeys={[statusFilter]}
                    onSelectionChange={(keys) => setStatusFilter(Array.from(keys)[0] as string)}
                    size="lg"
                    variant="bordered"
                    className="w-full"
                    classNames={{
                      trigger: "h-12"
                    }}
                  >
                    {statusOptions.map((option) => (
                      <SelectItem key={option.key} value={option.key}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </Select>
                </div>

                <div className="space-y-2">
                  <label className="text-xs text-default-500 uppercase tracking-wide">{tString('dateRange')}</label>
                  <DateRangePicker
                    value={dateRange}
                    onChange={setDateRange}
                    size="lg"
                    variant="bordered"
                    className="w-full"
                    classNames={{
                      base: "w-full",
                      input: "h-12"
                    }}
                  />
                </div>

                <div className="flex items-end">
                  <Button
                    variant="flat"
                    size="lg"
                    startContent={<X className="w-4 h-4" />}
                    onPress={() => {
                      setSearchTerm('')
                      setStatusFilter('all')
                      setDateRange(null)
                    }}
                    className="w-full h-12"
                    color="default"
                  >
                    {tString('clearAll')}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>

      {error && (
        <Card className="border-danger-200 bg-danger-50 shadow-md">
          <CardBody className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-danger-100 rounded-xl flex items-center justify-center">
                <X className="h-6 w-6 text-danger-600" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-danger-900">{tString('error')}</h3>
                <p className="text-danger-700">{error}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Payment Table */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between w-full">
            <h3 className="text-xl font-semibold text-default-900">{tString('paymentRecords')}</h3>
            <div className="text-sm text-default-600">
              {filteredPayments.length} {tString('paymentsCount').replace('{total}', payments.length.toString())}
            </div>
          </div>
        </CardHeader>
        <Divider />
        <CardBody className="p-0">
          <Table aria-label="Payment history table">
            <TableHeader>
              <TableColumn>{tString('tableHeaders.bill')}</TableColumn>
              <TableColumn>{tString('tableHeaders.table')}</TableColumn>
              <TableColumn>{tString('tableHeaders.amount')}</TableColumn>
              <TableColumn>{tString('tableHeaders.tips')}</TableColumn>
              <TableColumn>{tString('tableHeaders.payer')}</TableColumn>
              <TableColumn>{tString('tableHeaders.status')}</TableColumn>
              <TableColumn>{tString('tableHeaders.date')}</TableColumn>
              <TableColumn>{tString('tableHeaders.actions')}</TableColumn>
            </TableHeader>
            <TableBody emptyContent={tString('noPayments')}>
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
            <h3 className="text-lg font-semibold">{tString('summary.title')}</h3>
          </CardHeader>
          <Divider />
          <CardBody className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-bold text-primary">{filteredPayments.length}</p>
                <p className="text-sm text-default-600">{tString('summary.totalPayments')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-success">
                  {formatCurrency(filteredPayments.reduce((sum, p) => sum + p.amount, 0))}
                </p>
                <p className="text-sm text-default-600">{tString('summary.totalAmount')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-secondary">
                  {formatCurrency(filteredPayments.reduce((sum, p) => sum + p.tip_amount, 0))}
                </p>
                <p className="text-sm text-default-600">{tString('summary.totalTips')}</p>
              </div>
              
              <div className="text-center">
                <p className="text-2xl font-bold text-warning">
                  {filteredPayments.filter(p => p.status === 'confirmed').length}
                </p>
                <p className="text-sm text-default-600">{tString('summary.confirmed')}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
