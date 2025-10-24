'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import {
  Button,
  Input,
  useDisclosure,
  Select,
  SelectItem,
} from '@nextui-org/react';
import { businessApi, Table, CreateTableRequest, UpdateTableRequest } from '@/api/business';
import { PrimarySpinner } from '@/components/ui/spinners/PrimarySpinner';
import { QrCode, Download, Copy, Edit, Trash2, ExternalLink, Check, Search, X, Plus } from 'lucide-react';
import TableModals from './TableModals';

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
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

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
  }, [businessId, tString]);

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
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
        <button
          onClick={onCreateOpen}
          className="bg-gray-900 text-white px-4 py-2 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          {tString('createTable')}
        </button>
      </div>

      {/* Compact Search Bar */}
      <div className="bg-white border border-gray-200 rounded-lg p-4 mb-6">
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="flex-1">
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
              size="sm"
            />
          </div>
          
          <div className="flex items-center gap-2">
            <Select
              placeholder="Filter by status"
              selectedKeys={[statusFilter]}
              onSelectionChange={(keys: any) => setStatusFilter(Array.from(keys)[0] as 'all' | 'active' | 'inactive')}
              className="w-40"
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

        {/* Compact Search Results */}
        {(searchQuery || statusFilter !== 'all') && (
          <div className="mt-3 pt-3 border-t border-gray-200">
            <div className="text-sm text-gray-600">
{tString('search.foundTables').replace('{count}', filteredTables.length.toString())}
              {searchQuery && <span> {tString('search.matching').replace('{query}', searchQuery)}</span>}
              {statusFilter !== 'all' && <span> ({tString(`search.filters.${statusFilter}`)})</span>}
            </div>
          </div>
        )}
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="w-5 h-5 bg-red-100 rounded flex items-center justify-center">
                <X className="w-3 h-3 text-red-600" />
              </div>
              <p className="text-red-700 text-sm">{error}</p>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800 text-sm"
            >
              ×
            </button>
          </div>
        </div>
      )}

      {tables.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center">
          <div className="w-12 h-12 bg-gray-50 rounded-lg flex items-center justify-center mx-auto mb-4">
            <svg className="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
            </svg>
          </div>
          <h3 className="text-lg font-light text-gray-900 mb-2">{tString('emptyState.title')}</h3>
          <p className="text-gray-600 text-sm mb-4">
            {tString('emptyState.description')}
          </p>
          <button
            onClick={onCreateOpen}
            className="bg-gray-900 text-white px-4 py-2 text-sm font-medium hover:bg-gray-800 transition-colors rounded-lg"
          >
            {tString('emptyState.createButton')}
          </button>
        </div>
      ) : filteredTables.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center">
          <div className="w-12 h-12 bg-gray-50 rounded-lg flex items-center justify-center mx-auto mb-4">
            <Search className="h-6 w-6 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 mb-2">{tString('search.noResults')}</h3>
          <p className="text-gray-600 text-sm mb-4">
            {tString('search.noResultsDescription')}
          </p>
          <Button
            onPress={clearSearch}
            color="primary"
            variant="flat"
            size="sm"
            startContent={<X className="w-4 h-4" />}
          >
            {tString('buttons.clear')}
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredTables.map((table) => (
            <div key={table.id} className="bg-white border border-gray-200 rounded-lg p-6 hover:shadow-md hover:border-gray-300 transition-all duration-200">
              <div className="flex justify-between items-start mb-3">
                <div>
                  <h3 className="text-lg font-medium text-gray-900">{table.name}</h3>
                  <p className="text-xs text-gray-500">{tString('table.tableId')}: {table.id}</p>
                </div>
                <div className={`px-2 py-1 rounded text-xs font-medium ${
                  table.is_active 
                    ? 'bg-green-100 text-green-700' 
                    : 'bg-gray-100 text-gray-700'
                }`}>
{table.is_active ? tString('status.active') : tString('status.inactive')}
                </div>
              </div>

              {/* QR Section */}
              <div className="bg-gray-50 rounded-lg p-4 mb-4">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <QrCode className="w-5 h-5 text-gray-600" />
                    <span className="text-sm font-medium text-gray-700">{tString('qrCode.title')}</span>
                  </div>
                  <button
                    onClick={() => copyQRUrl(table)}
                    className="p-1 hover:bg-gray-200 rounded transition-colors"
                    title={tString('qrCode.copyUrl')}
                  >
                    {copiedUrls[table.id] ? (
                      <Check className="w-4 h-4 text-green-600" />
                    ) : (
                      <Copy className="w-4 h-4 text-gray-500" />
                    )}
                  </button>
                </div>

                {/* Table URL Display */}
                <div className="bg-white rounded-lg p-3 mb-3 border border-gray-200">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs font-medium text-gray-600">{tString('qrCode.tableUrl')}</span>
                  </div>
                  <p className="text-xs text-gray-800 font-mono break-all bg-gray-50 p-2 rounded border">
                    {window.location.origin}{table.qr_url}
                  </p>
                </div>
                
                <div className="flex gap-2">
                  <Button
                    onClick={() => downloadQRCode(table)}
                    variant="solid"
                    color="primary"
                    size="sm"
                    startContent={<Download className="w-3 h-3" />}
                    className="flex-1 text-xs"
                  >
                    {tString('qrCode.download')}
                  </Button>
                  <Button
                    onClick={() => window.open(`${window.location.origin}${table.qr_url}`, '_blank')}
                    variant="bordered"
                    color="primary"
                    size="sm"
                    startContent={<ExternalLink className="w-3 h-3" />}
                    className="flex-1 text-xs"
                  >
                    {tString('qrCode.preview')}
                  </Button>
                </div>
              </div>

              {/* Compact Action Buttons */}
              <div className="flex gap-2">
                <Button
                  onClick={() => handleEditTable(table)}
                  variant="solid"
                  color="default"
                  size="sm"
                  startContent={<Edit className="w-3 h-3" />}
                  className="flex-1 bg-gray-900 text-white hover:bg-gray-800 text-xs"
                >
                  {tString('buttons.edit')}
                </Button>
                <Button
                  onClick={() => handleDeleteTable(table.id)}
                  variant="light"
                  color="danger"
                  size="sm"
                  startContent={<Trash2 className="w-3 h-3" />}
                  className="text-xs"
                >
                  {tString('buttons.delete')}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Table Modals */}
      <TableModals
        isCreateOpen={isCreateOpen}
        onCreateOpenChange={onCreateOpenChange}
        tableName={tableName}
        setTableName={setTableName}
        handleCreateTable={handleCreateTable}
        isEditOpen={isEditOpen}
        onEditOpenChange={onEditOpenChange}
        editingTable={editingTable}
        editName={editName}
        setEditName={setEditName}
        editActive={editActive}
        setEditActive={setEditActive}
        handleUpdateTable={handleUpdateTable}
      />
    </div>
  );
}
