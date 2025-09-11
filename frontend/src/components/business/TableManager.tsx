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
    return <PrimarySpinner />;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">Table Management</h2>
        <Button color="primary" onPress={onCreateOpen}>
          Add Table
        </Button>
      </div>

      {error && (
        <Card className="border-danger">
          <CardBody>
            <p className="text-danger">{error}</p>
            <Button size="sm" color="danger" variant="light" onPress={() => setError(null)}>
              Dismiss
            </Button>
          </CardBody>
        </Card>
      )}

      {tables.length === 0 ? (
        <Card>
          <CardBody className="text-center py-8">
            <p className="text-default-500 mb-4">No tables created yet</p>
            <Button color="primary" onPress={onCreateOpen}>
              Create Your First Table
            </Button>
          </CardBody>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {tables.map((table) => (
            <Card key={table.id}>
              <CardHeader>
                <div className="flex justify-between items-start w-full">
                  <div>
                    <h3 className="text-lg font-semibold">{table.name}</h3>
                    <p className="text-sm text-default-600">Code: {table.table_code}</p>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      color="primary"
                      variant="light"
                      onPress={() => handleEditTable(table)}
                    >
                      Edit
                    </Button>
                    <Button
                      size="sm"
                      color="danger"
                      variant="light"
                      onPress={() => handleDeleteTable(table.id)}
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardBody>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Status:</span>
                    <Chip color={table.is_active ? "success" : "default"} size="sm">
                      {table.is_active ? "Active" : "Inactive"}
                    </Chip>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Created:</span>
                    <span className="text-sm text-default-600">
                      {new Date(table.created_at).toLocaleDateString()}
                    </span>
                  </div>
                  <div className="space-y-2 pt-2">
                    <Button
                      size="sm"
                      color="primary"
                      variant="flat"
                      className="w-full"
                      onPress={() => copyQRUrl(table)}
                    >
                      Copy QR URL
                    </Button>
                    <Button
                      size="sm"
                      variant="bordered"
                      className="w-full"
                      as="a"
                      href={table.qr_url}
                      target="_blank"
                    >
                      View Guest Page
                    </Button>
                  </div>
                </div>
              </CardBody>
            </Card>
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

