import React from 'react';
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Button,
} from '@nextui-org/react';
import { Eye, DollarSign } from 'lucide-react';
import { Bill } from '../../api/bills';

interface BillsTableProps {
  bills: Bill[];
  isActive: boolean;
  actionLoading: number | null;
  onViewBill: (billId: number) => void;
  onCloseBill: (billId: number) => void;
  onCreateBill: () => void;
  onResetFilters: () => void;
  tString: (key: string) => string;
}

export const BillsTable: React.FC<BillsTableProps> = ({
  bills,
  isActive,
  actionLoading,
  onViewBill,
  onCloseBill,
  onCreateBill,
  onResetFilters,
  tString,
}) => {
  const formatCurrency = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString();
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'open':
        return 'success';
      case 'paid':
        return 'primary';
      case 'closed':
        return 'default';
      default:
        return 'default';
    }
  };

  if (bills.length === 0) {
    return (
      <div className="text-center py-12">
        <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
          <DollarSign className="w-8 h-8 text-gray-400" />
        </div>
        <h3 className="text-lg font-light text-gray-900 mb-2 tracking-wide">
          {isActive ? tString('emptyState.noActiveBills') : tString('emptyState.noMatchingBills')}
        </h3>
        <p className="text-gray-600 font-light mb-6">
          {isActive 
            ? tString('emptyState.createFirstBill') 
            : tString('emptyState.adjustFilters')
          }
        </p>
        {isActive ? (
          <button
            onClick={onCreateBill}
            className="bg-gray-900 text-white px-6 py-3 rounded-lg font-medium hover:bg-gray-800 transition-colors"
          >
            {tString('buttons.createBill')}
          </button>
        ) : (
          <button
            onClick={onResetFilters}
            className="bg-gray-100 text-gray-700 px-6 py-3 rounded-lg font-medium hover:bg-gray-200 transition-colors"
          >
            {tString('buttons.resetFilters')}
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
      <Table aria-label="Bills table" removeWrapper>
        <TableHeader>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.billNumber')}
          </TableColumn>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.table')}
          </TableColumn>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.total')}
          </TableColumn>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.status')}
          </TableColumn>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.created')}
          </TableColumn>
          <TableColumn className="bg-gray-50 text-gray-700 font-medium">
            {tString('tableColumns.actions')}
          </TableColumn>
        </TableHeader>
        <TableBody>
          {bills.map((bill) => (
            <TableRow key={bill.id} className="hover:bg-gray-50 transition-colors">
              <TableCell>
                <span className="font-mono text-sm font-medium text-gray-900">
                  {bill.bill_number}
                </span>
              </TableCell>
              <TableCell>
                <span className="text-sm text-gray-600">
                  {tString('table')} {bill.table_id}
                </span>
              </TableCell>
              <TableCell>
                <span className="font-semibold text-gray-900">
                  {formatCurrency(bill.total_amount)}
                </span>
              </TableCell>
              <TableCell>
                <Chip
                  color={getStatusColor(bill.status)}
                  variant="flat"
                  size="sm"
                  className="font-medium"
                >
                  {bill.status.toUpperCase()}
                </Chip>
              </TableCell>
              <TableCell>
                <span className="text-sm text-gray-500">
                  {formatDate(bill.created_at)}
                </span>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="light"
                    startContent={<Eye className="w-4 h-4" />}
                    onPress={() => onViewBill(bill.id)}
                    className="text-gray-600 hover:text-gray-900"
                  >
                    {tString('view')}
                  </Button>
                  {bill.status === 'open' && (
                    <Button
                      size="sm"
                      color="warning"
                      variant="light"
                      isLoading={actionLoading === bill.id}
                      onPress={() => onCloseBill(bill.id)}
                    >
                      {tString('close')}
                    </Button>
                  )}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
};
