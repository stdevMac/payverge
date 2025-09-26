'use client';

import React, { useState, useEffect } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Spinner,
  Input,
  Card,
  CardBody,
  Chip,
  Divider,
  Progress,
  Checkbox,
  CheckboxGroup,
  Tooltip,
  Alert,
} from '@nextui-org/react';
import { AlertCircle, Users, Calculator, CheckCircle, ArrowLeft, Plus, Minus } from 'lucide-react';
import SplittingAPI from '../../api/splitting';

export interface BillData {
  id: number;
  billNumber: string;
  subtotal: number;
  taxAmount: number;
  serviceFeeAmount: number;
  totalAmount: number;
  items: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
    subtotal: number;
  }>;
}

export interface BillSplittingFlowProps {
  bill: BillData;
  businessName: string;
  businessAddress?: string;
  tableNumber?: string;
  isOpen: boolean;
  onClose: () => void;
  onPaymentInitiate: (personId: string, amount: number, tipAmount: number) => void;
  className?: string;
}

type SplitMethod = 'equal' | 'custom' | 'items';
type SplitStage = 'method' | 'configure' | 'results';

interface Person {
  id: string;
  name: string;
  amount?: number;
  percentage?: number;
  items?: string[];
  errors?: string[];
}

interface ValidationResult {
  isValid: boolean;
  errors: string[];
  warnings: string[];
  totalMismatch?: number;
}

interface BillItem {
  id: string;
  name: string;
  price: number;
  quantity: number;
  subtotal: number;
}

const BillSplittingFlow: React.FC<BillSplittingFlowProps> = ({
  bill,
  businessName,
  isOpen,
  onClose,
  onPaymentInitiate,
}) => {
  const [loading, setLoading] = useState(false);
  const [splitOptions, setSplitOptions] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);
  const [currentStage, setCurrentStage] = useState<SplitStage>('method');
  const [selectedMethod, setSelectedMethod] = useState<SplitMethod | null>(null);
  const [people, setPeople] = useState<Person[]>([]);
  const [splitResult, setSplitResult] = useState<any>(null);
  const [calculating, setCalculating] = useState(false);
  const [validation, setValidation] = useState<ValidationResult>({ isValid: true, errors: [], warnings: [] });
  const [showValidation, setShowValidation] = useState(false);

  useEffect(() => {
    if (isOpen && bill.id) {
      console.log('BillSplittingFlow: Modal opened for bill:', bill);
      // Reset state when modal opens
      setCurrentStage('method');
      setSelectedMethod(null);
      setPeople([]);
      setSplitResult(null);
      setError(null);
      setValidation({ isValid: true, errors: [], warnings: [] });
      setShowValidation(false);
      loadSplitOptions();
    }
  }, [isOpen, bill.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadSplitOptions = async () => {
    try {
      setLoading(true);
      setError(null);
      console.log('Loading split options for bill:', bill.id);
      
      const options = await SplittingAPI.getSplitOptions(bill.id);
      console.log('Split options loaded:', options);
      setSplitOptions(options);
    } catch (err) {
      console.error('Error loading split options:', err);
      setError(err instanceof Error ? err.message : 'Failed to load split options');
    } finally {
      setLoading(false);
    }
  };

  const addPerson = () => {
    const newPerson: Person = {
      id: `person_${Date.now()}`,
      name: `Person ${people.length + 1}`,
      amount: 0,
      items: []
    };
    setPeople([...people, newPerson]);
  };

  const removePerson = (personId: string) => {
    setPeople(people.filter(p => p.id !== personId));
  };

  const updatePersonName = (personId: string, name: string) => {
    setPeople(people.map(p => p.id === personId ? { ...p, name } : p));
    // Validate after name change
    setTimeout(() => validateSplit(), 100);
  };

  const updatePersonAmount = (personId: string, amount: number) => {
    const percentage = bill.totalAmount > 0 ? (amount / bill.totalAmount) * 100 : 0;
    setPeople(people.map(p => p.id === personId ? { ...p, amount, percentage } : p));
    // Validate after amount change
    if (selectedMethod === 'custom') {
      setTimeout(() => validateSplit(), 100);
    }
  };

  const updatePersonPercentage = (personId: string, percentage: number) => {
    const amount = (bill.totalAmount * percentage) / 100;
    setPeople(people.map(p => p.id === personId ? { ...p, percentage, amount } : p));
    // Validate after percentage change
    if (selectedMethod === 'custom') {
      setTimeout(() => validateSplit(), 100);
    }
  };

  const setPersonPercentagePreset = (personId: string, percentage: number) => {
    updatePersonPercentage(personId, percentage);
  };

  const togglePersonItem = (personId: string, itemId: string) => {
    setPeople(people.map(person => {
      if (person.id === personId) {
        const currentItems = person.items || [];
        const hasItem = currentItems.includes(itemId);
        const newItems = hasItem 
          ? currentItems.filter(id => id !== itemId)
          : [...currentItems, itemId];
        return { ...person, items: newItems };
      }
      return person;
    }));
    // Validate after item selection changes
    if (selectedMethod === 'items') {
      validateSplit();
    }
  };

  const validateSplit = (): ValidationResult => {
    const errors: string[] = [];
    const warnings: string[] = [];
    let totalMismatch = 0;

    // Validate people have names
    const emptyNames = people.filter(p => !p.name.trim());
    if (emptyNames.length > 0) {
      errors.push(`${emptyNames.length} person(s) need names`);
    }

    // Validate based on split method
    if (selectedMethod === 'custom') {
      const totalAssigned = people.reduce((sum, person) => sum + (person.amount || 0), 0);
      totalMismatch = Math.abs(totalAssigned - bill.totalAmount);
      
      if (totalMismatch > 0.01) { // Allow for small rounding differences
        if (totalAssigned > bill.totalAmount) {
          errors.push(`Total assigned ($${totalAssigned.toFixed(2)}) exceeds bill total ($${bill.totalAmount.toFixed(2)})`);
        } else {
          warnings.push(`Total assigned ($${totalAssigned.toFixed(2)}) is less than bill total ($${bill.totalAmount.toFixed(2)})`);
        }
      }

      // Check for people with no amount
      const zeroAmounts = people.filter(p => !p.amount || p.amount <= 0);
      if (zeroAmounts.length > 0) {
        warnings.push(`${zeroAmounts.length} person(s) have no amount assigned`);
      }
    }

    if (selectedMethod === 'items') {
      // Check if all items are assigned
      const allItemIds = splitOptions?.items?.map((item: BillItem) => item.id) || [];
      const assignedItemIds = new Set();
      people.forEach(person => {
        person.items?.forEach(itemId => assignedItemIds.add(itemId));
      });
      
      const unassignedItems = allItemIds.filter((id: string) => !assignedItemIds.has(id));
      if (unassignedItems.length > 0) {
        warnings.push(`${unassignedItems.length} item(s) not assigned to anyone`);
      }

      // Check for people with no items
      const noItems = people.filter(p => !p.items || p.items.length === 0);
      if (noItems.length > 0) {
        warnings.push(`${noItems.length} person(s) have no items assigned`);
      }

      // Check for duplicate item assignments
      const itemAssignments = new Map<string, string[]>();
      people.forEach(person => {
        person.items?.forEach(itemId => {
          if (!itemAssignments.has(itemId)) {
            itemAssignments.set(itemId, []);
          }
          itemAssignments.get(itemId)!.push(person.name);
        });
      });

      const duplicateItems = Array.from(itemAssignments.entries())
        .filter(([_, assignees]) => assignees.length > 1);
      
      if (duplicateItems.length > 0) {
        errors.push(`${duplicateItems.length} item(s) assigned to multiple people`);
      }
    }

    // General validations
    if (people.length < 2) {
      errors.push('At least 2 people required for splitting');
    }

    const result: ValidationResult = {
      isValid: errors.length === 0,
      errors,
      warnings,
      totalMismatch
    };

    setValidation(result);
    return result;
  };

  const calculateSplit = async () => {
    if (!selectedMethod || people.length === 0) return;

    // Validate before calculating
    const validationResult = validateSplit();
    setShowValidation(true);
    
    if (!validationResult.isValid) {
      setError('Please fix the validation errors before calculating');
      return;
    }

    try {
      setCalculating(true);
      setError(null);
      setShowValidation(false);

      let result;
      const peopleMap = people.reduce((acc, person) => {
        acc[person.id] = person.name;
        return acc;
      }, {} as Record<string, string>);

      switch (selectedMethod) {
        case 'equal':
          result = await SplittingAPI.calculateEqualSplit({
            bill_id: bill.id,
            num_people: people.length,
            people: peopleMap
          });
          break;

        case 'custom':
          const amounts = people.reduce((acc, person) => {
            acc[person.id] = person.amount || 0;
            return acc;
          }, {} as Record<string, number>);
          
          result = await SplittingAPI.calculateCustomSplit({
            bill_id: bill.id,
            amounts,
            people: peopleMap
          });
          break;

        case 'items':
          const itemSelections = people.reduce((acc, person) => {
            acc[person.id] = person.items || [];
            return acc;
          }, {} as Record<string, string[]>);
          
          result = await SplittingAPI.calculateItemSplit({
            bill_id: bill.id,
            item_selections: itemSelections,
            people: peopleMap
          });
          break;
      }

      console.log('Split calculation result:', result);
      console.log('People configured:', people);
      console.log('Result people:', result.people);
      setSplitResult(result);
      goToResults();
    } catch (err) {
      console.error('Error calculating split:', err);
      setError(err instanceof Error ? err.message : 'Failed to calculate split');
    } finally {
      setCalculating(false);
    }
  };

  const handleMethodSelect = (method: SplitMethod) => {
    console.log('Method selected:', method, 'Current people count:', people.length);
    setSelectedMethod(method);
    setSplitResult(null);
    setCurrentStage('configure');
    
    // Initialize with 2 people for equal/custom splits
    if ((method === 'equal' || method === 'custom') && people.length === 0) {
      const initialPeople = [
        { id: 'person_1', name: 'Person 1', amount: 0, percentage: 50, items: [] },
        { id: 'person_2', name: 'Person 2', amount: 0, percentage: 50, items: [] }
      ];
      console.log('Initializing people:', initialPeople);
      setPeople(initialPeople);
    }
  };

  const resetSplit = () => {
    setCurrentStage('method');
    setSelectedMethod(null);
    setPeople([]);
    setSplitResult(null);
    setError(null);
  };

  const goToResults = () => {
    setCurrentStage('results');
  };

  const goBackToMethod = () => {
    setCurrentStage('method');
  };

  const goBackToConfigure = () => {
    setCurrentStage('configure');
  };

  // Helper functions for better UX
  const getPersonItemsTotal = (person: Person): number => {
    if (!person.items || !splitOptions?.items) return 0;
    return person.items.reduce((total, itemId) => {
      const item = splitOptions.items.find((i: BillItem) => i.id === itemId);
      return total + (item?.subtotal || 0);
    }, 0);
  };

  const getUnassignedItems = (): BillItem[] => {
    if (!splitOptions?.items) return [];
    const assignedItemIds = new Set();
    people.forEach(person => {
      person.items?.forEach(itemId => assignedItemIds.add(itemId));
    });
    return splitOptions.items.filter((item: BillItem) => !assignedItemIds.has(item.id));
  };

  const assignAllRemainingItems = (personId: string) => {
    const unassignedItems = getUnassignedItems();
    const unassignedItemIds = unassignedItems.map(item => item.id);
    
    setPeople(people.map(person => {
      if (person.id === personId) {
        const currentItems = person.items || [];
        return { ...person, items: [...currentItems, ...unassignedItemIds] };
      }
      return person;
    }));
    
    setTimeout(() => validateSplit(), 100);
  };

  const clearPersonItems = (personId: string) => {
    setPeople(people.map(person => {
      if (person.id === personId) {
        return { ...person, items: [] };
      }
      return person;
    }));
    
    setTimeout(() => validateSplit(), 100);
  };

  const getStageProgress = (): number => {
    switch (currentStage) {
      case 'method': return 33;
      case 'configure': return 66;
      case 'results': return 100;
      default: return 0;
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="2xl"
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh]",
        body: "py-6",
      }}
    >
      <ModalContent>
        {/* Dynamic Header with Progress */}
        <ModalHeader className="flex flex-col gap-3">
          <div className="flex items-center gap-3">
            {currentStage !== 'method' && (
              <Tooltip content="Go back">
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  onPress={currentStage === 'configure' ? goBackToMethod : goBackToConfigure}
                >
                  <ArrowLeft className="w-4 h-4" />
                </Button>
              </Tooltip>
            )}
            <div className="flex-1">
              <div className="flex items-center gap-2 mb-1">
                <h2 className="text-xl font-semibold">
                  {currentStage === 'method' && 'Split Bill'}
                  {currentStage === 'configure' && `Configure ${selectedMethod === 'equal' ? 'Equal' : selectedMethod === 'custom' ? 'Custom' : 'Item'} Split`}
                  {currentStage === 'results' && 'Split Results'}
                </h2>
                {currentStage === 'configure' && (
                  <Chip size="sm" variant="flat" color={validation.isValid ? 'success' : validation.errors.length > 0 ? 'danger' : 'warning'}>
                    {validation.isValid ? 'Valid' : validation.errors.length > 0 ? 'Errors' : 'Warnings'}
                  </Chip>
                )}
              </div>
              <p className="text-sm text-gray-600">
                {businessName} • Bill #{bill.billNumber} • ${bill.totalAmount.toFixed(2)}
              </p>
            </div>
          </div>
          
          {/* Progress Bar */}
          <Progress 
            value={getStageProgress()} 
            className="w-full" 
            color="primary"
            size="sm"
          />
          
          {/* Stage Indicators */}
          <div className="flex justify-between text-xs text-gray-500">
            <span className={currentStage === 'method' ? 'text-primary font-medium' : ''}>
              1. Method
            </span>
            <span className={currentStage === 'configure' ? 'text-primary font-medium' : ''}>
              2. Configure
            </span>
            <span className={currentStage === 'results' ? 'text-primary font-medium' : ''}>
              3. Results
            </span>
          </div>
        </ModalHeader>
        
        <ModalBody>
          {loading && (
            <div className="flex justify-center items-center py-8">
              <Spinner size="lg" />
              <span className="ml-2">Loading split options...</span>
            </div>
          )}
          
          {error && (
            <div className="text-center text-red-600 py-8">
              <p>Error: {error}</p>
            </div>
          )}
          
          {!loading && !error && splitOptions && (
            <>
              {/* Stage 1: Method Selection */}
              {currentStage === 'method' && (
                <div className="space-y-6">
                  {/* Bill Summary */}
                  <div className="bg-gray-50 rounded-lg p-4">
                    <h4 className="font-medium text-gray-900 mb-2">Bill Summary</h4>
                    <div className="space-y-1 text-sm">
                      <div className="flex justify-between">
                        <span>Subtotal:</span>
                        <span>${splitOptions.bill?.subtotal?.toFixed(2) || '0.00'}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Tax:</span>
                        <span>${splitOptions.bill?.tax_amount?.toFixed(2) || '0.00'}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Service Fee:</span>
                        <span>${splitOptions.bill?.service_fee_amount?.toFixed(2) || '0.00'}</span>
                      </div>
                      <div className="flex justify-between font-medium text-lg border-t pt-1">
                        <span>Total:</span>
                        <span>${splitOptions.bill?.total_amount?.toFixed(2) || '0.00'}</span>
                      </div>
                    </div>
                  </div>

                  {/* Split Methods */}
                  <div className="space-y-4">
                    <h4 className="font-medium text-gray-900">Choose Split Method</h4>
                    
                    {splitOptions.split_options?.equal?.available && (
                      <button 
                        onClick={() => handleMethodSelect('equal')}
                        className="w-full text-left p-4 border border-gray-200 rounded-lg hover:border-blue-300 hover:bg-blue-50 transition-colors"
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <h5 className="font-medium text-gray-900">Split Equally</h5>
                            <p className="text-sm text-gray-600">Divide the bill equally among all people</p>
                          </div>
                          <div className="text-blue-600">→</div>
                        </div>
                      </button>
                    )}

                    {splitOptions.split_options?.custom?.available && (
                      <button 
                        onClick={() => handleMethodSelect('custom')}
                        className="w-full text-left p-4 border border-gray-200 rounded-lg hover:border-blue-300 hover:bg-blue-50 transition-colors"
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <h5 className="font-medium text-gray-900">Custom Amounts</h5>
                            <p className="text-sm text-gray-600">Specify exact amounts or percentages for each person</p>
                          </div>
                          <div className="text-blue-600">→</div>
                        </div>
                      </button>
                    )}

                    {splitOptions.split_options?.items?.available && (
                      <button 
                        onClick={() => handleMethodSelect('items')}
                        className="w-full text-left p-4 border border-gray-200 rounded-lg hover:border-blue-300 hover:bg-blue-50 transition-colors"
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <h5 className="font-medium text-gray-900">Split by Items</h5>
                            <p className="text-sm text-gray-600">Assign specific items to each person ({splitOptions.items?.length || 0} items)</p>
                          </div>
                          <div className="text-blue-600">→</div>
                        </div>
                      </button>
                    )}
                  </div>
                </div>
              )}

              {/* Stage 2: Configuration */}
              {currentStage === 'configure' && selectedMethod && (
                <div className="space-y-6">
                  {/* Validation Alerts */}
                  {showValidation && (validation.errors.length > 0 || validation.warnings.length > 0) && (
                    <div className="space-y-2">
                      {validation.errors.length > 0 && (
                        <Alert color="danger" variant="flat" className="p-3">
                          <AlertCircle className="w-4 h-4" />
                          <div className="ml-2">
                            <p className="font-medium">Please fix these errors:</p>
                            <ul className="text-sm mt-1 space-y-1">
                              {validation.errors.map((error, index) => (
                                <li key={index}>• {error}</li>
                              ))}
                            </ul>
                          </div>
                        </Alert>
                      )}
                      
                      {validation.warnings.length > 0 && (
                        <Alert color="warning" variant="flat" className="p-3">
                          <AlertCircle className="w-4 h-4" />
                          <div className="ml-2">
                            <p className="font-medium">Warnings:</p>
                            <ul className="text-sm mt-1 space-y-1">
                              {validation.warnings.map((warning, index) => (
                                <li key={index}>• {warning}</li>
                              ))}
                            </ul>
                          </div>
                        </Alert>
                      )}
                    </div>
                  )}

                  {/* Split Summary for Custom Method */}
                  {selectedMethod === 'custom' && (
                    <Card className="bg-blue-50 border-blue-200">
                      <CardBody className="p-4">
                        <div className="flex justify-between items-center">
                          <span className="font-medium text-blue-900">Total Assigned:</span>
                          <div className="text-right">
                            <span className="font-bold text-blue-900">
                              ${people.reduce((sum, p) => sum + (p.amount || 0), 0).toFixed(2)}
                            </span>
                            <span className="text-sm text-blue-700 block">
                              of ${bill.totalAmount.toFixed(2)}
                            </span>
                          </div>
                        </div>
                        {validation.totalMismatch && validation.totalMismatch > 0.01 && (
                          <div className="mt-2 text-sm">
                            <span className={validation.totalMismatch > 0 ? 'text-red-600' : 'text-green-600'}>
                              Difference: ${validation.totalMismatch.toFixed(2)}
                            </span>
                          </div>
                        )}
                      </CardBody>
                    </Card>
                  )}

                  {/* Unassigned Items Alert for Item Method */}
                  {selectedMethod === 'items' && getUnassignedItems().length > 0 && (
                    <Alert color="warning" variant="flat" className="p-3">
                      <AlertCircle className="w-4 h-4" />
                      <div className="ml-2">
                        <p className="font-medium">{getUnassignedItems().length} items not assigned</p>
                        <p className="text-sm">Total value: ${getUnassignedItems().reduce((sum, item) => sum + item.subtotal, 0).toFixed(2)}</p>
                      </div>
                    </Alert>
                  )}

                  {/* People Management */}
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <h4 className="font-medium text-gray-900 flex items-center gap-2">
                        <Users className="w-4 h-4" />
                        People ({people.length})
                      </h4>
                      <Button size="sm" color="primary" variant="light" onPress={addPerson} startContent={<Plus className="w-4 h-4" />}>
                        Add Person
                      </Button>
                    </div>

                    {people.map((person) => (
                      <Card key={person.id} className="p-4">
                        <div className="space-y-3">
                          <div className="flex items-center gap-3">
                            <Input
                              size="sm"
                              placeholder="Person name"
                              value={person.name}
                              onChange={(e) => updatePersonName(person.id, e.target.value)}
                              className="flex-1"
                            />
                            
                            {people.length > 1 && (
                              <Button
                                size="sm"
                                color="danger"
                                variant="light"
                                onPress={() => removePerson(person.id)}
                              >
                                Remove
                              </Button>
                            )}
                          </div>
                          
                          {selectedMethod === 'custom' && (
                            <div className="space-y-3">
                              {/* Percentage Presets */}
                              <div className="flex gap-2 flex-wrap">
                                <span className="text-sm text-gray-600 self-center">Quick:</span>
                                {[10, 25, 33, 50, 75].map(percentage => (
                                  <Button
                                    key={percentage}
                                    size="sm"
                                    variant={person.percentage === percentage ? "solid" : "bordered"}
                                    color={person.percentage === percentage ? "primary" : "default"}
                                    onPress={() => setPersonPercentagePreset(person.id, percentage)}
                                  >
                                    {percentage}%
                                  </Button>
                                ))}
                              </div>
                              
                              {/* Custom Amount/Percentage */}
                              <div className="flex gap-3">
                                <Input
                                  size="sm"
                                  type="number"
                                  placeholder="Percentage"
                                  value={person.percentage?.toString() || ''}
                                  onChange={(e) => updatePersonPercentage(person.id, parseFloat(e.target.value) || 0)}
                                  endContent="%"
                                  className="flex-1"
                                />
                                <Input
                                  size="sm"
                                  type="number"
                                  placeholder="Amount"
                                  value={person.amount?.toFixed(2) || ''}
                                  onChange={(e) => updatePersonAmount(person.id, parseFloat(e.target.value) || 0)}
                                  startContent="$"
                                  className="flex-1"
                                />
                              </div>
                            </div>
                          )}

                          {selectedMethod === 'items' && (
                            <div className="space-y-3">
                              <div className="flex items-center justify-between">
                                <h6 className="text-sm font-medium text-gray-700">
                                  Select Items ({person.items?.length || 0} selected)
                                  {person.items && person.items.length > 0 && (
                                    <span className="ml-2 text-green-600 font-medium">
                                      ${getPersonItemsTotal(person).toFixed(2)}
                                    </span>
                                  )}
                                </h6>
                                <div className="flex gap-1">
                                  {getUnassignedItems().length > 0 && (
                                    <Tooltip content="Assign all remaining items">
                                      <Button
                                        size="sm"
                                        variant="light"
                                        color="primary"
                                        onPress={() => assignAllRemainingItems(person.id)}
                                      >
                                        All
                                      </Button>
                                    </Tooltip>
                                  )}
                                  {person.items && person.items.length > 0 && (
                                    <Tooltip content="Clear all items">
                                      <Button
                                        size="sm"
                                        variant="light"
                                        color="danger"
                                        onPress={() => clearPersonItems(person.id)}
                                      >
                                        Clear
                                      </Button>
                                    </Tooltip>
                                  )}
                                </div>
                              </div>
                              
                              <div className="max-h-40 overflow-y-auto space-y-2 border rounded-lg p-2">
                                {splitOptions.items?.map((item: BillItem) => {
                                  const isAssignedToOther = people.some(p => 
                                    p.id !== person.id && p.items?.includes(item.id)
                                  );
                                  const isSelected = person.items?.includes(item.id) || false;
                                  
                                  return (
                                    <div 
                                      key={item.id} 
                                      className={`flex items-center gap-3 p-2 rounded transition-colors ${
                                        isSelected ? 'bg-blue-50 border border-blue-200' : 
                                        isAssignedToOther ? 'bg-gray-50 opacity-50' : 
                                        'hover:bg-gray-50'
                                      }`}
                                    >
                                      <Checkbox
                                        isSelected={isSelected}
                                        isDisabled={isAssignedToOther}
                                        onChange={() => togglePersonItem(person.id, item.id)}
                                        size="sm"
                                      />
                                      <div className="flex-1 min-w-0">
                                        <div className="flex items-center justify-between">
                                          <span className="text-sm font-medium truncate">{item.name}</span>
                                          <span className="text-sm font-medium text-green-600">
                                            ${item.subtotal.toFixed(2)}
                                          </span>
                                        </div>
                                        <div className="flex items-center justify-between text-xs text-gray-500">
                                          <span>${item.price.toFixed(2)} × {item.quantity}</span>
                                          {isAssignedToOther && (
                                            <span className="text-orange-600">Assigned to other</span>
                                          )}
                                        </div>
                                      </div>
                                    </div>
                                  );
                                })}
                              </div>
                            </div>
                          )}
                        </div>
                      </Card>
                    ))}
                  </div>

                  {/* Calculate Button */}
                  <div className="space-y-3">
                    {error && (
                      <Alert color="danger" variant="flat" className="p-3">
                        <AlertCircle className="w-4 h-4" />
                        <div className="ml-2">
                          <p className="font-medium">Calculation Error</p>
                          <p className="text-sm">{error}</p>
                        </div>
                      </Alert>
                    )}
                    
                    <Button
                      color="primary"
                      onPress={calculateSplit}
                      isLoading={calculating}
                      isDisabled={people.length === 0 || (showValidation && !validation.isValid)}
                      className="w-full"
                      size="lg"
                      startContent={!calculating && <Calculator className="w-4 h-4" />}
                    >
                      {calculating ? 'Calculating Split...' : 'Calculate Split'}
                    </Button>
                    
                    {!validation.isValid && showValidation && (
                      <p className="text-sm text-gray-500 text-center">
                        Fix validation errors to continue
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* Stage 3: Results */}
              {currentStage === 'results' && splitResult && (
                <div className="space-y-6">
                  <div className="bg-green-50 p-4 rounded-lg">
                    <div className="flex justify-between items-center">
                      <span className="font-medium text-green-800">Total Split:</span>
                      <span className="font-bold text-green-800">${splitResult.total_amount?.toFixed(2) || '0.00'}</span>
                    </div>
                    <p className="text-sm text-green-600 mt-1">
                      Split method: {splitResult.split_method} • {splitResult.people?.length || people.length} people
                    </p>
                  </div>
                  
                  <div className="space-y-3">
                    {splitResult.people?.map((person: any) => (
                      <Card key={person.person_id} className="p-4">
                        <div className="flex items-center justify-between mb-3">
                          <h5 className="font-medium text-gray-900">{person.name}</h5>
                          <Chip color="primary" variant="flat" size="lg">
                            ${person.total_amount.toFixed(2)}
                          </Chip>
                        </div>
                        
                        <div className="text-sm text-gray-600 space-y-1 mb-3">
                          <div className="flex justify-between">
                            <span>Base amount:</span>
                            <span>${person.base_amount.toFixed(2)}</span>
                          </div>
                          {person.tax_amount > 0 && (
                            <div className="flex justify-between">
                              <span>Tax:</span>
                              <span>${person.tax_amount.toFixed(2)}</span>
                            </div>
                          )}
                          {person.service_fee_amount > 0 && (
                            <div className="flex justify-between">
                              <span>Service fee:</span>
                              <span>${person.service_fee_amount.toFixed(2)}</span>
                            </div>
                          )}
                        </div>

                        <Button
                          color="success"
                          className="w-full"
                          size="lg"
                          onPress={() => onPaymentInitiate(person.person_id, person.total_amount, 0)}
                        >
                          Pay ${person.total_amount.toFixed(2)}
                        </Button>
                      </Card>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
          
          {!loading && !error && !splitOptions && (
            <div className="text-center text-gray-600 py-8">
              <p>Ready to load split options...</p>
            </div>
          )}
        </ModalBody>
        
        <ModalFooter>
          <Button
            color="danger"
            variant="light"
            onPress={onClose}
          >
            Close
          </Button>
          {currentStage === 'results' && (
            <Button
              color="primary"
              variant="light"
              onPress={goBackToConfigure}
            >
              Modify Split
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};

export default BillSplittingFlow;
