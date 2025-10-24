'use client';

import React from 'react';
import { Button, Input, Select, SelectItem } from '@nextui-org/react';
import { Search, X } from 'lucide-react';
import * as StaffAPI from '../../api/staff';

type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

interface StaffSearchFilterProps {
  searchQuery: string;
  setSearchQuery: (query: string) => void;
  roleFilter: 'all' | Staff['role'];
  setRoleFilter: (role: 'all' | Staff['role']) => void;
  filteredStaff: Staff[];
  filteredInvitations: PendingInvitation[];
  getRoleLabel: (role: Staff['role']) => string;
  tString: (key: string) => string;
  clearSearch: () => void;
}

export default function StaffSearchFilter({
  searchQuery,
  setSearchQuery,
  roleFilter,
  setRoleFilter,
  filteredStaff,
  filteredInvitations,
  getRoleLabel,
  tString,
  clearSearch
}: StaffSearchFilterProps) {
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
      <div className="border-b border-gray-200 p-6">
        <div className="flex items-center gap-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <Search className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('search.title')}</h2>
            <p className="text-gray-600 font-light text-sm">Search and filter your staff members</p>
          </div>
        </div>
      </div>
      <div className="p-6">
        <div className="flex flex-col sm:flex-row gap-4">
          <div className="flex-1">
            <Input
              placeholder={tString('search.placeholder')}
              value={searchQuery}
              onValueChange={setSearchQuery}
              startContent={<Search className="w-4 h-4 text-default-400" />}
              endContent={
                searchQuery && (
                  <Button
                    isIconOnly
                    size="sm"
                    variant="light"
                    onPress={() => setSearchQuery('')}
                    className="min-w-0 w-6 h-6"
                  >
                    <X className="w-3 h-3" />
                  </Button>
                )
              }
              size="lg"
              variant="bordered"
              isClearable
            />
          </div>
          
          <div className="flex items-center gap-3">
            <Select
              placeholder={tString('search.filterByRole')}
              selectedKeys={[roleFilter]}
              onSelectionChange={(keys) => setRoleFilter(Array.from(keys)[0] as any)}
              className="w-full sm:w-48"
              size="lg"
              variant="bordered"
            >
              <SelectItem key="all" value="all">{tString('search.allRoles')}</SelectItem>
              <SelectItem key="manager" value="manager">{getRoleLabel('manager')}</SelectItem>
              <SelectItem key="server" value="server">{getRoleLabel('server')}</SelectItem>
              <SelectItem key="host" value="host">{getRoleLabel('host')}</SelectItem>
              <SelectItem key="kitchen" value="kitchen">{getRoleLabel('kitchen')}</SelectItem>
            </Select>
            
            {(searchQuery || roleFilter !== 'all') && (
              <Button
                variant="flat"
                size="lg"
                startContent={<X className="w-4 h-4" />}
                onPress={clearSearch}
              >
                {tString('buttons.clear')}
              </Button>
            )}
          </div>
        </div>
        
        {/* Search Results Summary */}
        {(searchQuery || roleFilter !== 'all') && (
          <div className="mt-4 pt-4 border-t border-default-200">
            <div className="flex items-center justify-between">
              <div className="text-sm text-default-600">
                {tString('search.showing')} <strong className="text-default-900">{filteredStaff.length}</strong> {tString('search.staffMembers')}
                {filteredInvitations.length > 0 && (
                  <span> {tString('search.and')} <strong className="text-default-900">{filteredInvitations.length}</strong> {tString('search.pendingInvitations')}</span>
                )}
                {searchQuery && (
                  <span> {tString('search.matching')} &quot;<strong className="text-primary">{searchQuery}</strong>&quot;</span>
                )}
                {roleFilter !== 'all' && (
                  <span> • {getRoleLabel(roleFilter as Staff['role'])} {tString('search.only')}</span>
                )}
              </div>
              
              {searchQuery && (
                <span className="inline-block px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
                  {tString('search.active')}
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
