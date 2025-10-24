'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Button, useDisclosure, Spinner } from '@nextui-org/react';
import { Plus } from 'lucide-react';
import { toast } from 'react-hot-toast';
import * as StaffAPI from '../../api/staff';

// Import sub-components
import StaffOverviewCards from '../staff/StaffOverviewCards';
import StaffSearchFilter from '../staff/StaffSearchFilter';
import StaffTable from '../staff/StaffTable';
import InvitationTable from '../staff/InvitationTable';
import StaffInviteModal from '../staff/StaffInviteModal';
import StaffRoleUpdateModal from '../staff/StaffRoleUpdateModal';

// Use types from API
type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

interface StaffManagementProps {
  businessId: string;
}

// Role colors remain constant
const roleLabels = {
  manager: 'Manager',
  server: 'Server', 
  host: 'Host',
  kitchen: 'Kitchen',
};

const roleDescriptions = {
  manager: 'Can manage menu, tables, bills (no financial settings)',
  server: 'Can view tables, create/close bills',
  host: 'Can view tables, seat guests',
  kitchen: 'Can view orders, mark items ready',
};

const roleColors = {
  manager: 'primary',
  server: 'success',
  host: 'warning',
  kitchen: 'secondary',
} as const;

const statusColors = {
  pending: 'warning',
  accepted: 'success',
  expired: 'danger',
  revoked: 'default',
} as const;

export default function StaffManagement({ businessId }: StaffManagementProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.staffManagement.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

  // Translated role labels and descriptions
  const getRoleLabel = (role: Staff['role']): string => {
    return tString(`roles.${role}.label`);
  };

  const getRoleDescription = (role: Staff['role']): string => {
    return tString(`roles.${role}.description`);
  };

  const [staff, setStaff] = useState<Staff[]>([]);
  const [invitations, setInvitations] = useState<PendingInvitation[]>([]);
  const [loading, setLoading] = useState(true);
  const [inviteLoading, setInviteLoading] = useState(false);
  
  // Search and filter state
  const [searchQuery, setSearchQuery] = useState('');
  const [roleFilter, setRoleFilter] = useState<'all' | Staff['role']>('all');
  
  const { isOpen, onOpen, onClose } = useDisclosure();
  const { isOpen: isRoleModalOpen, onOpen: onRoleModalOpen, onClose: onRoleModalClose } = useDisclosure();
  
  // Invite form state
  const [inviteForm, setInviteForm] = useState({
    email: '',
    name: '',
    role: 'server' as Staff['role'],
  });

  // Role update state
  const [selectedStaff, setSelectedStaff] = useState<Staff | null>(null);
  const [newRole, setNewRole] = useState<Staff['role']>('server');
  const [roleUpdateLoading, setRoleUpdateLoading] = useState(false);

  // Load staff and invitations
  const loadStaffData = async () => {
    try {
      setLoading(true);
      const data = await StaffAPI.getBusinessStaff(businessId);
      setStaff(data.staff || []);
      setInvitations(data.pending_invitations || []);
    } catch (error) {
      console.error('Error loading staff data:', error);
      toast.error(tString('error.loadStaffData'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadStaffData();
  }, [businessId]); // eslint-disable-line react-hooks/exhaustive-deps

  // Handle staff invitation
  const handleInviteStaff = async () => {
    if (!inviteForm.email || !inviteForm.name || !inviteForm.role) {
      toast.error(tString('error.fillAllFields'));
      return;
    }

    try {
      setInviteLoading(true);
      await StaffAPI.inviteStaff(businessId, inviteForm);
      
      toast.success(tString('success.invitationSent'));
      setInviteForm({ email: '', name: '', role: 'server' });
      onClose();
      loadStaffData(); // Reload to show new invitation
    } catch (error: any) {
      console.error('Error inviting staff:', error);
      toast.error(error.message || tString('error.sendInvitation'));
    } finally {
      setInviteLoading(false);
    }
  };

  // Handle staff removal
  const handleRemoveStaff = async (staffId: number, staffName: string) => {
    if (!confirm(tString('confirmRemove').replace('{name}', staffName))) {
      return;
    }

    try {
      await StaffAPI.removeStaff(businessId, staffId);
      toast.success(tString('success.staffRemoved'));
      loadStaffData(); // Reload staff list
    } catch (error) {
      console.error('Error removing staff:', error);
      toast.error(tString('error.removeStaff'));
    }
  };

  // Handle resend invitation
  const handleResendInvitation = async (invitationId: number, staffName: string) => {
    if (!confirm(tString('confirmResend').replace('{name}', staffName))) {
      return;
    }

    try {
      await StaffAPI.resendInvitation(businessId, invitationId);
      toast.success(tString('success.invitationResent'));
      loadStaffData(); // Reload to show updated expiry
    } catch (error: any) {
      console.error('Error resending invitation:', error);
      toast.error(error.message || tString('error.resendInvitation'));
    }
  };

  // Handle role update
  const handleUpdateRole = (staff: Staff) => {
    setSelectedStaff(staff);
    setNewRole(staff.role);
    onRoleModalOpen();
  };

  // Handle invite modal open
  const handleInviteModalOpen = () => {
    setInviteForm({ email: '', name: '', role: 'server' });
    onOpen();
  };

  const handleRoleUpdateSubmit = async () => {
    if (!selectedStaff) return;

    try {
      setRoleUpdateLoading(true);
      await StaffAPI.updateStaffRole(businessId, selectedStaff.id, newRole);
      toast.success(tString('success.roleUpdated'));
      onRoleModalClose();
      loadStaffData(); // Reload to show updated role
    } catch (error: any) {
      console.error('Error updating staff role:', error);
      toast.error(error.message || tString('error.updateRole'));
    } finally {
      setRoleUpdateLoading(false);
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const isInvitationExpired = (expiresAt: string) => {
    return new Date(expiresAt) < new Date();
  };

  // Filter and search logic
  const filteredStaff = React.useMemo(() => {
    if (!searchQuery.trim() && roleFilter === 'all') {
      return staff;
    }

    const query = searchQuery.toLowerCase().trim();
    
    return staff.filter(member => {
      // Text search across name, email, and role
      const nameMatches = !query || member.name.toLowerCase().includes(query);
      const emailMatches = !query || member.email.toLowerCase().includes(query);
      const roleMatches = !query || roleLabels[member.role].toLowerCase().includes(query);
      
      const textMatches = nameMatches || emailMatches || roleMatches;
      
      // Role filter
      const roleFilterMatches = roleFilter === 'all' || member.role === roleFilter;

      return textMatches && roleFilterMatches;
    });
  }, [staff, searchQuery, roleFilter]);

  const filteredInvitations = React.useMemo(() => {
    if (!searchQuery.trim() && roleFilter === 'all') {
      return invitations;
    }

    const query = searchQuery.toLowerCase().trim();
    
    return invitations.filter(invitation => {
      // Text search across name, email, and role
      const nameMatches = !query || invitation.name.toLowerCase().includes(query);
      const emailMatches = !query || invitation.email.toLowerCase().includes(query);
      const roleMatches = !query || roleLabels[invitation.role].toLowerCase().includes(query);
      
      const textMatches = nameMatches || emailMatches || roleMatches;
      
      // Role filter
      const roleFilterMatches = roleFilter === 'all' || invitation.role === roleFilter;

      return textMatches && roleFilterMatches;
    });
  }, [invitations, searchQuery, roleFilter]);

  const clearSearch = () => {
    setSearchQuery('');
    setRoleFilter('all');
  };

  if (loading) {
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
          onClick={handleInviteModalOpen}
          className="bg-gray-900 text-white px-4 py-2 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          {tString('buttons.inviteStaff')}
        </button>
      </div>

      {/* Search and Filter */}
      <StaffSearchFilter
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        roleFilter={roleFilter}
        setRoleFilter={setRoleFilter}
        filteredStaff={filteredStaff}
        filteredInvitations={filteredInvitations}
        getRoleLabel={getRoleLabel}
        tString={tString}
        clearSearch={clearSearch}
      />

      {/* Staff Overview Cards */}
      <StaffOverviewCards
        staff={staff}
        invitations={invitations}
        tString={tString}
      />

      {/* Active Staff */}
      <StaffTable
        staff={filteredStaff}
        getRoleLabel={getRoleLabel}
        getRoleDescription={getRoleDescription}
        tString={tString}
        formatDate={formatDate}
        handleUpdateRole={handleUpdateRole}
        handleRemoveStaff={handleRemoveStaff}
      />

      {/* Pending Invitations */}
      <InvitationTable
        invitations={filteredInvitations}
        getRoleLabel={getRoleLabel}
        tString={tString}
        formatDate={formatDate}
        isInvitationExpired={isInvitationExpired}
        handleResendInvitation={handleResendInvitation}
      />

      {/* Staff Invitation Modal */}
      <StaffInviteModal
        isOpen={isOpen}
        onClose={onClose}
        inviteForm={inviteForm}
        setInviteForm={setInviteForm}
        inviteLoading={inviteLoading}
        getRoleLabel={getRoleLabel}
        getRoleDescription={getRoleDescription}
        tString={tString}
        handleInviteStaff={handleInviteStaff}
      />

      {/* Role Update Modal */}
      <StaffRoleUpdateModal
        isOpen={isRoleModalOpen}
        onClose={onRoleModalClose}
        selectedStaff={selectedStaff}
        newRole={newRole}
        setNewRole={setNewRole}
        roleUpdateLoading={roleUpdateLoading}
        getRoleLabel={getRoleLabel}
        getRoleDescription={getRoleDescription}
        tString={tString}
        handleRoleUpdateSubmit={handleRoleUpdateSubmit}
      />
    </div>
  );
}
