'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Input,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Chip,
  Switch,
  Select,
  SelectItem,
} from '@nextui-org/react';
import { businessApi, Table, CreateTableRequest, UpdateTableRequest } from '@/api/business';
import { PrimarySpinner } from '@/components/ui/spinners/PrimarySpinner';
import { QrCode, Download, Copy, Edit, Trash2, ExternalLink, Check, Search, X, Plus } from 'lucide-react';

interface TableManagerProps {
  businessId: number;
}

export default function TableManager({ businessId }: TableManagerProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [tables, setTables] = useState<Table[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tableName, setTableName] = useState('');
  const [editingTable, setEditingTable] = useState<Table | null>(null);
  const [editName, setEditName] = useState('');
  const [editActive, setEditActive] = useState(false);
  const [copiedUrls, setCopiedUrls] = useState<Record<number, boolean>>({});
  
  // Search functionality
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all');

  // Modal states
  const { isOpen: isCreateOpen, onOpen: onCreateOpen, onOpenChange: onCreateOpenChange } = useDisclosure();
  const { isOpen: isEditOpen, onOpen: onEditOpen, onOpenChange: onEditOpenChange } = useDisclosure();

  const loadTables = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await businessApi.getBusinessTables(businessId);
      setTables(response.tables || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : tString('error.loadTables'));
    } finally {
      setIsLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    loadTables();
  }, [loadTables]);

  // Search and filter functionality
  const filteredTables = React.useMemo(() => {
    if (!searchQuery.trim() && statusFilter === 'all') {
      return tables;
    }

    const query = searchQuery.toLowerCase().trim();
    
    return tables.filter(table => {
      // Text search
      const nameMatches = !query || table.name.toLowerCase().includes(query);
      
      // Status filter
      const statusMatches = statusFilter === 'all' ||
        (statusFilter === 'active' && table.is_active) ||
        (statusFilter === 'inactive' && !table.is_active);

      return nameMatches && statusMatches;
    });
  }, [tables, searchQuery, statusFilter]);

  const clearSearch = () => {
    setSearchQuery('');
    setStatusFilter('all');
  };

  const handleCreateTable = async () => {
    if (!tableName.trim()) return;
    
    try {
      const newTable = await businessApi.createTableWithQR(businessId, { name: tableName.trim() });
      setTables([...tables, newTable]);
      setTableName('');
      onCreateOpenChange();
    } catch (err) {
      setError(err instanceof Error ? err.message : tString('error.createTable'));
    }
  };

  const handleEditTable = (table: Table) => {
    setEditingTable(table);
    setEditName(table.name);
    setEditActive(table.is_active);
    onEditOpen();
  };

  const handleUpdateTable = async () => {
    if (!editingTable) return;
    
    try {
      const updatedTable = await businessApi.updateTableDetails(editingTable.id, {
        name: editName,
        is_active: editActive,
      });
      setTables(tables.map(table => table.id === editingTable.id ? updatedTable : table));
      onEditOpenChange();
      setEditingTable(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : tString('error.updateTable'));
    }
  };

  const handleDeleteTable = async (tableId: number) => {
    if (!confirm(tString('confirmDelete'))) return;
    
    try {
      await businessApi.deleteTable(tableId);
      setTables(tables.filter(table => table.id !== tableId));
    } catch (err) {
      setError(err instanceof Error ? err.message : tString('error.deleteTable'));
    }
  };

  const copyQRUrl = async (table: Table) => {
    try {
      const fullUrl = `${window.location.origin}${table.qr_url}`;
      await navigator.clipboard.writeText(fullUrl);
      setCopiedUrls(prev => ({ ...prev, [table.id]: true }));
      
      // Reset the copied state after 2 seconds
      setTimeout(() => {
        setCopiedUrls(prev => ({ ...prev, [table.id]: false }));
      }, 2000);
    } catch (err) {
      console.error('Failed to copy URL: ', err);
    }
  };

  const downloadQRCode = async (table: Table) => {
    try {
      const fullUrl = `${window.location.origin}${table.qr_url}`;
      
      // Create QR code using a QR code library (we'll use qrcode.js)
      const QRCode = (await import('qrcode')).default;
      
      // Generate QR code as data URL
      const qrDataUrl = await QRCode.toDataURL(fullUrl, {
        width: 512,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#FFFFFF'
        }
      });
      
      // Create download link
      const link = document.createElement('a');
      link.href = qrDataUrl;
      link.download = `${table.name}-qr-code.png`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
    } catch (err) {
      console.error('Failed to download QR code: ', err);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="space-y-6">
        <div className="flex justify-between items-center">
          <div>
            <h2 className="text-3xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
            <p className="text-gray-600 font-light mt-2">{tString('subtitle')}</p>
          </div>
          <Button
            onPress={onCreateOpen}
            color="primary"
            size="lg"
            startContent={<Plus className="w-5 h-5" />}
            className="font-semibold shadow-lg"
          >
            {tString('createTable')}
          </Button>
        </div>

        {/* Search and Filter Section */}
        <Card className="border-gray-200">
          <CardBody className="p-4">
            <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center">
              <div className="flex-1 relative">
                <Input
                  placeholder={tString('search.placeholder')}
                  value={searchQuery}
                  onValueChange={setSearchQuery}
                  startContent={<Search className="w-4 h-4 text-gray-400" />}
                  endContent={
                    searchQuery && (
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        onPress={clearSearch}
                        className="min-w-0 w-6 h-6"
                      >
                        <X className="w-3 h-3" />
                      </Button>
                    )
                  }
                  className="w-full"
                />
              </div>
              
              <div className="flex items-center gap-3">
                <Select
                  label={tString('search.filterByStatus')}
                  selectedKeys={[statusFilter]}
                  onSelectionChange={(keys: any) => setStatusFilter(Array.from(keys)[0] as 'all' | 'active' | 'inactive')}
                  className="w-48"
                  size="sm"
                >
                  <SelectItem key="all" value="all">{tString('search.filters.all')}</SelectItem>
                  <SelectItem key="active" value="active">{tString('search.filters.active')}</SelectItem>
                  <SelectItem key="inactive" value="inactive">{tString('search.filters.inactive')}</SelectItem>
                </Select>
                
                {(searchQuery || statusFilter !== 'all') && (
                  <Button
                    size="sm"
                    variant="flat"
                    onPress={clearSearch}
                    startContent={<X className="w-3 h-3" />}
                  >
                    {tString('buttons.clear')}
                  </Button>
                )}
              </div>
            </div>

            {/* Search Results Summary */}
            {(searchQuery || statusFilter !== 'all') && (
              <div className="mt-3 pt-3 border-t border-gray-200">
                <div className="flex items-center justify-between">
                  <div className="text-sm text-gray-600">
                    Found <strong>{filteredTables.length}</strong> tables
                    {searchQuery && (
                      <span> matching &quot;<strong>{searchQuery}</strong>&quot;</span>
                    )}
                    {statusFilter !== 'all' && (
                      <span> ({statusFilter} tables)</span>
                    )}
                  </div>
                  
                  {searchQuery && (
                    <Chip size="sm" variant="flat" color="primary">
                      {tString('search.active')}
                    </Chip>
                  )}
                </div>
              </div>
            )}
          </CardBody>
        </Card>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-red-100 rounded-xl flex items-center justify-center">
                <svg className="h-6 w-6 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <div>
                <h3 className="text-lg font-medium text-red-900 tracking-wide">{tString('error.title')}</h3>
                <p className="text-red-700 font-light">{error}</p>
              </div>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800 transition-colors duration-200"
            >
              {tString('buttons.close')}
            </button>
          </div>
        </div>
      )}

      {tables.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
          <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
            <svg className="h-10 w-10 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
            </svg>
          </div>
          <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">{tString('emptyState.title')}</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            {tString('emptyState.description')}
          </p>
          <button
            onClick={onCreateOpen}
            className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
          >
            {tString('emptyState.createButton')}
          </button>
        </div>
      ) : filteredTables.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
          <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
            <Search className="h-10 w-10 text-gray-400" />
          </div>
          <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">{tString('search.noResults')}</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            {tString('search.noResultsDescription')}
          </p>
          <Button
            onPress={clearSearch}
            color="primary"
            variant="flat"
            startContent={<X className="w-4 h-4" />}
          >
            {tString('buttons.clear')}
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          {filteredTables.map((table) => (
            <div key={table.id} className="group bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1">
              <div className="flex justify-between items-start mb-6">
                <div>
                  <h3 className="text-xl font-light text-gray-900 tracking-wide mb-1">{table.name}</h3>
                  <p className="text-sm text-gray-500 font-light">{tString('table.tableId')}: {table.id}</p>
                </div>
                <div className={`px-3 py-1 rounded-xl text-xs font-medium tracking-wide ${
                  table.is_active 
                    ? 'bg-green-100 text-green-700 border border-green-200' 
                    : 'bg-gray-100 text-gray-700 border border-gray-200'
                }`}>
                  {table.is_active ? tString('status.active') : tString('status.inactive')}
                </div>
              </div>

              {/* QR Code Section */}
              <div className="space-y-6 mb-8">
                <div className="bg-gradient-to-br from-blue-50 to-indigo-50 border border-blue-200 rounded-2xl p-6">
                  <div className="flex items-center gap-3 mb-4">
                    <div className="w-10 h-10 bg-blue-100 rounded-xl flex items-center justify-center">
                      <QrCode className="w-5 h-5 text-blue-600" />
                    </div>
                    <div>
                      <h4 className="text-lg font-semibold text-gray-900">{tString('qrCode.title')}</h4>
                      <p className="text-sm text-gray-600">{tString('qrCode.subtitle')}</p>
                    </div>
                  </div>
                  
                  <div className="bg-white rounded-xl p-4 border border-blue-100 mb-4">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium text-gray-700">{tString('qrCode.tableUrl')}</span>
                      <button
                        onClick={() => copyQRUrl(table)}
                        className="p-1 hover:bg-gray-100 rounded-md transition-colors duration-200 group"
                        title={tString('qrCode.copyUrl')}
                      >
                        {copiedUrls[table.id] ? (
                          <Check className="w-4 h-4 text-green-600" />
                        ) : (
                          <Copy className="w-4 h-4 text-gray-500 group-hover:text-gray-700" />
                        )}
                      </button>
                    </div>
                    <p className="text-xs text-gray-600 font-mono break-all leading-relaxed bg-gray-50 p-2 rounded-lg">
                      {window.location.origin}{table.qr_url}
                    </p>
                  </div>

                  {/* QR Code Actions */}
                  <div className="grid grid-cols-2 gap-3">
                    <Button
                      onClick={() => downloadQRCode(table)}
                      variant="solid"
                      color="primary"
                      size="sm"
                      startContent={<Download className="w-4 h-4" />}
                      className="font-semibold"
                    >
                      {tString('qrCode.download')}
                    </Button>
                    <Button
                      onClick={() => window.open(`${window.location.origin}${table.qr_url}`, '_blank')}
                      variant="bordered"
                      color="primary"
                      size="sm"
                      startContent={<ExternalLink className="w-4 h-4" />}
                      className="font-semibold"
                    >
                      {tString('qrCode.preview')}
                    </Button>
                  </div>
                </div>
              </div>

              {/* Action Buttons */}
              <div className="flex gap-3">
                <Button
                  onClick={() => handleEditTable(table)}
                  variant="solid"
                  color="default"
                  size="md"
                  startContent={<Edit className="w-4 h-4" />}
                  className="flex-1 font-semibold bg-gray-900 text-white hover:bg-gray-800"
                >
                  {tString('buttons.edit')}
                </Button>
                <Button
                  onClick={() => handleDeleteTable(table.id)}
                  variant="light"
                  color="danger"
                  size="md"
                  startContent={<Trash2 className="w-4 h-4" />}
                  className="font-semibold"
                >
                  {tString('buttons.delete')}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create Table Modal */}
      <Modal isOpen={isCreateOpen} onOpenChange={onCreateOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{tString('modals.create.title')}</ModalHeader>
              <ModalBody>
                <Input
                  label={tString('modals.create.tableName')}
                  placeholder={tString('modals.create.placeholder')}
                  value={tableName}
                  onValueChange={setTableName}
                  isRequired
                />
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  {tString('modals.create.cancel')}
                </Button>
                <Button color="primary" onPress={handleCreateTable}>
                  {tString('modals.create.create')}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Edit Table Modal */}
      <Modal isOpen={isEditOpen} onOpenChange={onEditOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{tString('modals.edit.title')}</ModalHeader>
              <ModalBody>
                <Input
                  label={tString('modals.edit.tableName')}
                  value={editName}
                  onValueChange={setEditName}
                  isRequired
                />
                <div className="flex items-center justify-between">
                  <span>{tString('modals.edit.activeStatus')}</span>
                  <Switch
                    isSelected={editActive}
                    onValueChange={setEditActive}
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  {tString('modals.edit.cancel')}
                </Button>
                <Button color="primary" onPress={handleUpdateTable}>
                  {tString('modals.edit.save')}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}

