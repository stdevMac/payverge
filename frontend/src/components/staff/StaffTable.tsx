'use client';

import React from 'react';
import { Button, Table, TableHeader, TableColumn, TableBody, TableRow, TableCell, Chip } from '@nextui-org/react';
import { Users, Edit, Trash2 } from 'lucide-react';
import * as StaffAPI from '../../api/staff';

type Staff = StaffAPI.StaffMember;

interface StaffTableProps {
  staff: Staff[];
  getRoleLabel: (role: Staff['role']) => string;
  getRoleDescription: (role: Staff['role']) => string;
  tString: (key: string) => string;
  formatDate: (dateString: string) => string;
  handleUpdateRole: (staff: Staff) => void;
  handleRemoveStaff: (staffId: number, staffName: string) => void;
}

const roleColors = {
  manager: 'primary',
  server: 'success',
  host: 'warning',
  kitchen: 'secondary',
} as const;

export default function StaffTable({
  staff,
  getRoleLabel,
  getRoleDescription,
  tString,
  formatDate,
  handleUpdateRole,
  handleRemoveStaff
}: StaffTableProps) {
  if (staff.length === 0) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <Users className="w-8 h-8 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString('table.noStaffYet')}</h3>
          <p className="text-gray-600 font-light text-sm">{tString('table.inviteFirstStaff')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
      <div className="border-b border-gray-200 p-6">
        <div className="flex items-center gap-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <Users className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('table.activeStaffTitle')}</h2>
            <p className="text-gray-600 font-light text-sm">Manage your active staff members</p>
          </div>
        </div>
      </div>
      <div className="p-6">
        <Table aria-label="Staff members table">
          <TableHeader>
            <TableColumn>{tString('table.columns.name')}</TableColumn>
            <TableColumn>{tString('table.columns.email')}</TableColumn>
            <TableColumn>{tString('table.columns.role')}</TableColumn>
            <TableColumn>{tString('table.columns.joined')}</TableColumn>
            <TableColumn>{tString('table.columns.actions')}</TableColumn>
          </TableHeader>
          <TableBody>
            {staff.map((member) => (
              <TableRow key={member.id}>
                <TableCell>
                  <div className="flex flex-col">
                    <p className="font-medium text-gray-900">{member.name}</p>
                  </div>
                </TableCell>
                <TableCell>
                  <span className="text-gray-600">{member.email}</span>
                </TableCell>
                <TableCell>
                  <div className="flex flex-col gap-1">
                    <Chip
                      size="sm"
                      variant="flat"
                      color={roleColors[member.role]}
                    >
                      {getRoleLabel(member.role)}
                    </Chip>
                    <span className="text-xs text-gray-500">
                      {getRoleDescription(member.role)}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <span className="text-gray-600">{formatDate(member.created_at)}</span>
                </TableCell>
                <TableCell>
                  <div className="flex gap-2">
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      onPress={() => handleUpdateRole(member)}
                      className="text-gray-600 hover:text-blue-600"
                    >
                      <Edit className="w-4 h-4" />
                    </Button>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      onPress={() => handleRemoveStaff(member.id, member.name)}
                      className="text-gray-600 hover:text-red-600"
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
