'use client';

import React, { useState, useEffect, useCallback } from 'react';
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
} from '@nextui-org/react';
import { businessApi, Table, CreateTableRequest, UpdateTableRequest } from '@/api/business';
import { PrimarySpinner } from '@/components/ui/spinners/PrimarySpinner';

interface TableManagerProps {
  businessId: number;
}

export default function TableManager({ businessId }: TableManagerProps) {
  const [tables, setTables] = useState<Table[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tableName, setTableName] = useState('');
  const [editingTable, setEditingTable] = useState<Table | null>(null);
  const [editName, setEditName] = useState('');
  const [editActive, setEditActive] = useState(false);

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
      setError(err instanceof Error ? err.message : 'Failed to load tables');
    } finally {
      setIsLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    loadTables();
  }, [loadTables]);

  const handleCreateTable = async () => {
    if (!tableName.trim()) return;
    
    try {
      const newTable = await businessApi.createTableWithQR(businessId, { name: tableName.trim() });
      setTables([...tables, newTable]);
      setTableName('');
      onCreateOpenChange();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create table');
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
      setError(err instanceof Error ? err.message : 'Failed to update table');
    }
  };

  const handleDeleteTable = async (tableId: number) => {
    if (!confirm('Are you sure you want to delete this table?')) return;
    
    try {
      await businessApi.deleteTable(tableId);
      setTables(tables.filter(table => table.id !== tableId));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete table');
    }
  };

  const copyQRUrl = (table: Table) => {
    const fullUrl = `${window.location.origin}${table.qr_url}`;
    navigator.clipboard.writeText(fullUrl);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">Loading tables...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex justify-between items-center">
        <h2 className="text-3xl font-light text-gray-900 tracking-wide">Table Management</h2>
        <button
          onClick={onCreateOpen}
          className="bg-gray-900 text-white px-6 py-3 text-sm font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-md hover:shadow-lg"
        >
          Add Table
        </button>
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
                <h3 className="text-lg font-medium text-red-900 tracking-wide">Error</h3>
                <p className="text-red-700 font-light">{error}</p>
              </div>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800 transition-colors duration-200"
            >
              Dismiss
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
          <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">No tables created yet</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            Create your first table to start managing customer seating and orders.
          </p>
          <button
            onClick={onCreateOpen}
            className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
          >
            Create Your First Table
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          {tables.map((table) => (
            <div key={table.id} className="group bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1">
              <div className="flex justify-between items-start mb-6">
                <div>
                  <h3 className="text-xl font-light text-gray-900 tracking-wide mb-1">{table.name}</h3>
                  <p className="text-sm text-gray-500 font-light">Table ID: {table.id}</p>
                </div>
                <div className={`px-3 py-1 rounded-xl text-xs font-medium tracking-wide ${
                  table.is_active 
                    ? 'bg-green-100 text-green-700 border border-green-200' 
                    : 'bg-gray-100 text-gray-700 border border-gray-200'
                }`}>
                  {table.is_active ? 'Active' : 'Inactive'}
                </div>
              </div>

              <div className="space-y-4 mb-8">
                <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                  <p className="text-sm font-medium text-gray-900 tracking-wide mb-2">QR Code URL</p>
                  <p className="text-xs text-gray-600 font-mono break-all leading-relaxed">
                    {window.location.origin}{table.qr_url}
                  </p>
                </div>
              </div>

              <div className="flex gap-3">
                <button
                  onClick={() => copyQRUrl(table)}
                  className="flex-1 bg-gray-100 text-gray-700 px-4 py-3 rounded-xl text-sm font-medium hover:bg-gray-200 transition-all duration-200 text-center tracking-wide"
                >
                  Copy URL
                </button>
                <button
                  onClick={() => handleEditTable(table)}
                  className="flex-1 bg-gray-900 text-white px-4 py-3 rounded-xl text-sm font-medium hover:bg-gray-800 transition-all duration-200 text-center tracking-wide group-hover:shadow-lg"
                >
                  Edit
                </button>
                <button
                  onClick={() => handleDeleteTable(table.id)}
                  className="text-red-600 hover:text-red-800 px-4 py-3 text-sm font-medium transition-colors duration-200 tracking-wide"
                >
                  Delete
                </button>
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
              <ModalHeader>Create New Table</ModalHeader>
              <ModalBody>
                <Input
                  label="Table Name"
                  placeholder="e.g., Table 1, Patio A, etc."
                  value={tableName}
                  onValueChange={setTableName}
                  isRequired
                />
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleCreateTable}>
                  Create Table
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
              <ModalHeader>Edit Table</ModalHeader>
              <ModalBody>
                <Input
                  label="Table Name"
                  value={editName}
                  onValueChange={setEditName}
                  isRequired
                />
                <div className="flex items-center justify-between">
                  <span>Active Status</span>
                  <Switch
                    isSelected={editActive}
                    onValueChange={setEditActive}
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleUpdateTable}>
                  Save Changes
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}

