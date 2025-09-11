'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Chip,
  Divider,
  Avatar,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from '@nextui-org/react';
import { Users, Calculator, Receipt, Plus, Minus, X } from 'lucide-react';

export type SplitMethod = 'equal' | 'custom' | 'items';

export interface Person {
  id: string;
  name: string;
  amount?: number;
  items?: string[];
}

export interface SplitSelectorProps {
  billAmount: number;
  billItems: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
    subtotal: number;
  }>;
  onSplitChange: (method: SplitMethod, data: any) => void;
  disabled?: boolean;
}

export default function SplitSelector({ 
  billAmount, 
  billItems, 
  onSplitChange, 
  disabled = false 
}: SplitSelectorProps) {
  const [selectedMethod, setSelectedMethod] = useState<SplitMethod>('equal');
  const [numPeople, setNumPeople] = useState(2);
  const [people, setPeople] = useState<Person[]>([
    { id: 'person_1', name: 'Person 1' },
    { id: 'person_2', name: 'Person 2' }
  ]);
  const [isAddPersonModalOpen, setIsAddPersonModalOpen] = useState(false);
  const [newPersonName, setNewPersonName] = useState('');

  const formatCurrency = (amount: number) => `$${amount.toFixed(2)}`;

  const handleMethodChange = (method: SplitMethod) => {
    setSelectedMethod(method);
    
    switch (method) {
      case 'equal':
        handleEqualSplit();
        break;
      case 'custom':
        handleCustomSplit();
        break;
      case 'items':
        handleItemSplit();
        break;
    }
  };

  const handleEqualSplit = () => {
    const data = {
      num_people: people.length,
      people: people.reduce((acc, person) => {
        acc[person.id] = person.name;
        return acc;
      }, {} as Record<string, string>)
    };
    onSplitChange('equal', data);
  };

  const handleCustomSplit = () => {
    const amounts = people.reduce((acc, person) => {
      acc[person.id] = person.amount || 0;
      return acc;
    }, {} as Record<string, number>);
    
    const peopleNames = people.reduce((acc, person) => {
      acc[person.id] = person.name;
      return acc;
    }, {} as Record<string, string>);

    const data = {
      amounts,
      people: peopleNames
    };
    onSplitChange('custom', data);
  };

  const handleItemSplit = () => {
    const itemSelections = people.reduce((acc, person) => {
      acc[person.id] = person.items || [];
      return acc;
    }, {} as Record<string, string[]>);
    
    const peopleNames = people.reduce((acc, person) => {
      acc[person.id] = person.name;
      return acc;
    }, {} as Record<string, string>);

    const data = {
      item_selections: itemSelections,
      people: peopleNames
    };
    onSplitChange('items', data);
  };

  const addPerson = () => {
    if (newPersonName.trim() && people.length < 10) {
      const newPerson: Person = {
        id: `person_${Date.now()}`,
        name: newPersonName.trim(),
        amount: selectedMethod === 'custom' ? billAmount / (people.length + 1) : undefined,
        items: selectedMethod === 'items' ? [] : undefined
      };
      
      setPeople([...people, newPerson]);
      setNewPersonName('');
      setIsAddPersonModalOpen(false);
      
      // Trigger split recalculation
      setTimeout(() => {
        handleMethodChange(selectedMethod);
      }, 0);
    }
  };

  const removePerson = (personId: string) => {
    if (people.length > 1) {
      setPeople(people.filter(p => p.id !== personId));
      
      // Trigger split recalculation
      setTimeout(() => {
        handleMethodChange(selectedMethod);
      }, 0);
    }
  };

  const updatePersonAmount = (personId: string, amount: number) => {
    setPeople(people.map(p => 
      p.id === personId ? { ...p, amount } : p
    ));
    
    // Trigger custom split recalculation
    setTimeout(() => {
      handleCustomSplit();
    }, 300);
  };

  const updatePersonName = (personId: string, name: string) => {
    setPeople(people.map(p => 
      p.id === personId ? { ...p, name } : p
    ));
  };

  const toggleItemForPerson = (personId: string, itemId: string) => {
    setPeople(people.map(p => {
      if (p.id === personId) {
        const currentItems = p.items || [];
        const hasItem = currentItems.includes(itemId);
        const newItems = hasItem 
          ? currentItems.filter(id => id !== itemId)
          : [...currentItems, itemId];
        return { ...p, items: newItems };
      }
      return p;
    }));
    
    // Trigger item split recalculation
    setTimeout(() => {
      handleItemSplit();
    }, 0);
  };

  const getPersonItemsTotal = (person: Person) => {
    if (!person.items) return 0;
    return person.items.reduce((total, itemId) => {
      const item = billItems.find(i => i.id === itemId);
      return total + (item?.subtotal || 0);
    }, 0);
  };

  const renderEqualSplit = () => (
    <div className="space-y-4">
      <div className="text-center">
        <p className="text-sm text-default-500 mb-2">Split equally among</p>
        <p className="text-2xl font-bold text-primary">{people.length} people</p>
        <p className="text-lg text-default-600">
          {formatCurrency(billAmount / people.length)} each
        </p>
      </div>
      
      <Divider />
      
      <div className="space-y-2">
        {people.map((person, index) => (
          <div key={person.id} className="flex items-center justify-between p-3 bg-default-50 rounded-lg">
            <div className="flex items-center gap-3">
              <Avatar 
                size="sm" 
                name={person.name} 
                className="bg-primary text-white"
              />
              <Input
                size="sm"
                value={person.name}
                onValueChange={(value) => updatePersonName(person.id, value)}
                variant="flat"
                className="max-w-32"
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="font-semibold">{formatCurrency(billAmount / people.length)}</span>
              {people.length > 1 && (
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  color="danger"
                  onPress={() => removePerson(person.id)}
                >
                  <X size={16} />
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );

  const renderCustomSplit = () => {
    const totalCustomAmount = people.reduce((sum, p) => sum + (p.amount || 0), 0);
    const isValid = Math.abs(totalCustomAmount - billAmount) < 0.01;
    
    return (
      <div className="space-y-4">
        <div className="text-center">
          <p className="text-sm text-default-500 mb-2">Custom amounts</p>
          <div className={`text-lg font-semibold ${isValid ? 'text-success' : 'text-danger'}`}>
            Total: {formatCurrency(totalCustomAmount)} / {formatCurrency(billAmount)}
          </div>
          {!isValid && (
            <p className="text-sm text-danger">
              Amounts must total {formatCurrency(billAmount)}
            </p>
          )}
        </div>
        
        <Divider />
        
        <div className="space-y-2">
          {people.map((person) => (
            <div key={person.id} className="flex items-center justify-between p-3 bg-default-50 rounded-lg">
              <div className="flex items-center gap-3 flex-1">
                <Avatar 
                  size="sm" 
                  name={person.name} 
                  className="bg-primary text-white"
                />
                <Input
                  size="sm"
                  value={person.name}
                  onValueChange={(value) => updatePersonName(person.id, value)}
                  variant="flat"
                  className="max-w-32"
                />
              </div>
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  size="sm"
                  value={person.amount?.toString() || '0'}
                  onValueChange={(value) => updatePersonAmount(person.id, parseFloat(value) || 0)}
                  startContent="$"
                  variant="bordered"
                  className="w-24"
                  min="0"
                  step="0.01"
                />
                {people.length > 1 && (
                  <Button
                    isIconOnly
                    size="sm"
                    variant="light"
                    color="danger"
                    onPress={() => removePerson(person.id)}
                  >
                    <X size={16} />
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  };

  const renderItemSplit = () => (
    <div className="space-y-4">
      <div className="text-center">
        <p className="text-sm text-default-500 mb-2">Assign items to people</p>
        <p className="text-lg font-semibold">Select who ordered what</p>
      </div>
      
      <Divider />
      
      <div className="space-y-4">
        {people.map((person) => (
          <Card key={person.id} className="border">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center gap-3">
                  <Avatar 
                    size="sm" 
                    name={person.name} 
                    className="bg-primary text-white"
                  />
                  <Input
                    size="sm"
                    value={person.name}
                    onValueChange={(value) => updatePersonName(person.id, value)}
                    variant="flat"
                    className="max-w-32"
                  />
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold">
                    {formatCurrency(getPersonItemsTotal(person))}
                  </span>
                  {people.length > 1 && (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      color="danger"
                      onPress={() => removePerson(person.id)}
                    >
                      <X size={16} />
                    </Button>
                  )}
                </div>
              </div>
            </CardHeader>
            <CardBody className="pt-0">
              <div className="grid grid-cols-1 gap-2">
                {billItems.map((item) => {
                  const isSelected = person.items?.includes(item.id) || false;
                  return (
                    <div
                      key={item.id}
                      className={`p-2 rounded-lg border-2 cursor-pointer transition-colors ${
                        isSelected 
                          ? 'border-primary bg-primary-50' 
                          : 'border-default-200 hover:border-default-300'
                      }`}
                      onClick={() => toggleItemForPerson(person.id, item.id)}
                    >
                      <div className="flex justify-between items-center">
                        <div>
                          <p className="font-medium text-sm">{item.name}</p>
                          <p className="text-xs text-default-500">
                            {item.quantity} × {formatCurrency(item.price)}
                          </p>
                        </div>
                        <span className="font-semibold text-sm">
                          {formatCurrency(item.subtotal)}
                        </span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </CardBody>
          </Card>
        ))}
      </div>
    </div>
  );

  return (
    <div className="space-y-6">
      {/* Method Selection */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Calculator className="w-5 h-5" />
            <h3 className="text-lg font-semibold">Choose Split Method</h3>
          </div>
        </CardHeader>
        <CardBody>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <Button
              variant={selectedMethod === 'equal' ? 'solid' : 'bordered'}
              color={selectedMethod === 'equal' ? 'primary' : 'default'}
              onPress={() => handleMethodChange('equal')}
              startContent={<Users size={16} />}
              disabled={disabled}
              className="h-auto py-4"
            >
              <div className="text-center">
                <div className="font-semibold">Equal Split</div>
                <div className="text-xs opacity-70">Split evenly</div>
              </div>
            </Button>
            
            <Button
              variant={selectedMethod === 'custom' ? 'solid' : 'bordered'}
              color={selectedMethod === 'custom' ? 'primary' : 'default'}
              onPress={() => handleMethodChange('custom')}
              startContent={<Calculator size={16} />}
              disabled={disabled}
              className="h-auto py-4"
            >
              <div className="text-center">
                <div className="font-semibold">Custom Amount</div>
                <div className="text-xs opacity-70">Set amounts</div>
              </div>
            </Button>
            
            <Button
              variant={selectedMethod === 'items' ? 'solid' : 'bordered'}
              color={selectedMethod === 'items' ? 'primary' : 'default'}
              onPress={() => handleMethodChange('items')}
              startContent={<Receipt size={16} />}
              disabled={disabled}
              className="h-auto py-4"
            >
              <div className="text-center">
                <div className="font-semibold">By Items</div>
                <div className="text-xs opacity-70">Who ordered what</div>
              </div>
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Split Configuration */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between w-full">
            <h3 className="text-lg font-semibold">
              {selectedMethod === 'equal' && 'Equal Split'}
              {selectedMethod === 'custom' && 'Custom Amounts'}
              {selectedMethod === 'items' && 'Item Assignment'}
            </h3>
            {people.length < 10 && (
              <Button
                size="sm"
                color="primary"
                variant="flat"
                startContent={<Plus size={16} />}
                onPress={() => setIsAddPersonModalOpen(true)}
                disabled={disabled}
              >
                Add Person
              </Button>
            )}
          </div>
        </CardHeader>
        <CardBody>
          {selectedMethod === 'equal' && renderEqualSplit()}
          {selectedMethod === 'custom' && renderCustomSplit()}
          {selectedMethod === 'items' && renderItemSplit()}
        </CardBody>
      </Card>

      {/* Add Person Modal */}
      <Modal 
        isOpen={isAddPersonModalOpen} 
        onClose={() => setIsAddPersonModalOpen(false)}
        size="sm"
      >
        <ModalContent>
          <ModalHeader>Add Person</ModalHeader>
          <ModalBody>
            <Input
              label="Person Name"
              placeholder="Enter name"
              value={newPersonName}
              onValueChange={setNewPersonName}
              autoFocus
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  addPerson();
                }
              }}
            />
          </ModalBody>
          <ModalFooter>
            <Button 
              variant="light" 
              onPress={() => setIsAddPersonModalOpen(false)}
            >
              Cancel
            </Button>
            <Button 
              color="primary" 
              onPress={addPerson}
              isDisabled={!newPersonName.trim()}
            >
              Add Person
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
