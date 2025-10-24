import React from 'react';
import { Input, Select, SelectItem, Button } from '@nextui-org/react';
import { Search, Calendar, RotateCcw } from 'lucide-react';

interface BillFiltersProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  dateFrom: string;
  onDateFromChange: (value: string) => void;
  dateTo: string;
  onDateToChange: (value: string) => void;
  statusFilter?: 'all' | 'paid' | 'closed';
  onStatusFilterChange?: (value: 'all' | 'paid' | 'closed') => void;
  onReset: () => void;
  tString: (key: string) => string;
  showStatusFilter?: boolean;
}

export const BillFilters: React.FC<BillFiltersProps> = ({
  searchQuery,
  onSearchChange,
  dateFrom,
  onDateFromChange,
  dateTo,
  onDateToChange,
  statusFilter,
  onStatusFilterChange,
  onReset,
  tString,
  showStatusFilter = false,
}) => {
  return (
    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 mb-6">
      <div className="flex flex-col lg:flex-row gap-4 lg:items-center">
        {/* Search Input */}
        <div className="flex-1 min-w-0">
          <Input
            value={searchQuery}
            onValueChange={onSearchChange}
            placeholder={
              showStatusFilter 
                ? tString('search.billHistoryPlaceholder')
                : tString('search.activeBillsPlaceholder')
            }
            startContent={<Search className="w-4 h-4 text-gray-400" />}
            size="sm"
            className="w-full"
            classNames={{
              input: "text-sm",
              inputWrapper: "bg-white border-gray-200 hover:border-gray-300"
            }}
          />
        </div>

        {/* Status Filter (for history tab) */}
        {showStatusFilter && statusFilter && onStatusFilterChange && (
          <div className="w-full lg:w-40">
            <Select
              selectedKeys={[statusFilter]}
              onChange={(e) => onStatusFilterChange((e.target.value as any) || 'all')}
              size="sm"
              aria-label="Filter by status"
              classNames={{
                trigger: "bg-white border-gray-200 hover:border-gray-300"
              }}
            >
              <SelectItem key="all">{tString('allStatuses')}</SelectItem>
              <SelectItem key="paid">{tString('paid')}</SelectItem>
              <SelectItem key="closed">{tString('closed')}</SelectItem>
            </Select>
          </div>
        )}

        {/* Date Range */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <Calendar className="w-4 h-4 text-gray-400" />
            <input
              type="date"
              value={dateFrom}
              onChange={(e) => onDateFromChange(e.target.value)}
              className="border border-gray-200 rounded-lg px-3 py-2 text-sm bg-white hover:border-gray-300 focus:border-gray-400 focus:outline-none transition-colors"
              aria-label="From date"
            />
          </div>
          <span className="text-gray-400 text-sm font-medium">to</span>
          <input
            type="date"
            value={dateTo}
            onChange={(e) => onDateToChange(e.target.value)}
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm bg-white hover:border-gray-300 focus:border-gray-400 focus:outline-none transition-colors"
            aria-label="To date"
          />
        </div>

        {/* Reset Button */}
        <Button
          variant="light"
          size="sm"
          onPress={onReset}
          startContent={<RotateCcw className="w-4 h-4" />}
          className="text-gray-600 hover:text-gray-900 hover:bg-gray-100"
        >
          {tString('reset')}
        </Button>
      </div>
    </div>
  );
};
