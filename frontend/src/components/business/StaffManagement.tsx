'use client';

import React, { useState, useEffect } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Input,
  Select,
  SelectItem,
  Spinner,
} from '@nextui-org/react';
import { UserPlus, Mail, Trash2, Users, Clock, CheckCircle, Plus, RefreshCw, Edit, Search, X, Filter } from 'lucide-react';
import { toast } from 'react-hot-toast';
import * as StaffAPI from '../../api/staff';

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
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.staffManagement.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

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
      <div className="flex justify-center items-center p-8">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-3xl font-bold text-default-900">{tString('title')}</h2>
          <p className="text-default-600 mt-1">{tString('subtitle')}</p>
        </div>
        <Button
          color="primary"
          size="lg"
          startContent={<Plus className="w-5 h-5" />}
          onPress={handleInviteModalOpen}
          className="w-full sm:w-auto"
        >
          {tString('buttons.inviteStaff')}
        </Button>
      </div>

      {/* Search and Filter */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Search className="w-5 h-5 text-primary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('search.title')}</h3>
          </div>
        </CardHeader>
        <CardBody className="p-6">
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
                  <Chip size="sm" variant="flat" color="primary">
                    {tString('search.active')}
                  </Chip>
                )}
              </div>
            </div>
          )}
        </CardBody>
      </Card>

      {/* Staff Overview Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Card className="border border-gray-200">
          <CardBody className="p-6">
            <div className="flex items-center space-x-3">
              <div className="p-2 bg-blue-50 rounded-lg">
                <Users className="w-6 h-6 text-blue-600" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-gray-900">{staff.length}</p>
                <p className="text-sm text-gray-600">{tString('overview.activeStaff')}</p>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border border-gray-200">
          <CardBody className="p-6">
            <div className="flex items-center space-x-3">
              <div className="p-2 bg-orange-50 rounded-lg">
                <Clock className="w-6 h-6 text-orange-600" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-gray-900">
                  {invitations.filter(inv => (inv.status || 'pending') === 'pending').length}
                </p>
                <p className="text-sm text-gray-600">{tString('overview.pendingInvitations')}</p>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border border-gray-200">
          <CardBody className="p-6">
            <div className="flex items-center space-x-3">
              <div className="p-2 bg-green-50 rounded-lg">
                <CheckCircle className="w-6 h-6 text-green-600" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-gray-900">
                  {staff.filter(s => s.last_login_at && 
                    new Date(s.last_login_at) > new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
                  ).length}
                </p>
                <p className="text-sm text-gray-600">{tString('overview.activeThisWeek')}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Active Staff Table */}
      <Card className="border border-gray-200">
        <CardHeader className="pb-3">
          <h3 className="text-lg font-medium text-gray-900">{tString('table.activeStaffTitle')}</h3>
        </CardHeader>
        <CardBody className="pt-0">
          {filteredStaff.length === 0 ? (
            <div className="text-center py-8">
              <Users className="w-12 h-12 text-gray-400 mx-auto mb-4" />
              {staff.length === 0 ? (
                <>
                  <p className="text-gray-600">{tString('table.noStaffYet')}</p>
                  <p className="text-sm text-gray-500 mt-1">{tString('table.inviteFirstStaff')}</p>
                </>
              ) : (
                <>
                  <p className="text-gray-600">{tString('table.noStaffFound')}</p>
                  <p className="text-sm text-gray-500 mt-1">{tString('table.adjustSearch')}</p>
                  <Button
                    variant="flat"
                    size="sm"
                    startContent={<X className="w-4 h-4" />}
                    onPress={clearSearch}
                    className="mt-3"
                  >
                    {tString('buttons.clearSearch')}
                  </Button>
                </>
              )}
            </div>
          ) : (
            <Table aria-label="Staff members table">
              <TableHeader>
                <TableColumn>{tString('table.columns.name')}</TableColumn>
                <TableColumn>{tString('table.columns.email')}</TableColumn>
                <TableColumn>{tString('table.columns.role')}</TableColumn>
                <TableColumn>{tString('table.columns.lastLogin')}</TableColumn>
                <TableColumn>{tString('table.columns.joined')}</TableColumn>
                <TableColumn>{tString('table.columns.actions')}</TableColumn>
              </TableHeader>
              <TableBody>
                {filteredStaff.map((member) => (
                  <TableRow key={member.id}>
                    <TableCell>
                      <div className="font-medium text-gray-900">{member.name}</div>
                    </TableCell>
                    <TableCell>
                      <div className="text-gray-600">{member.email}</div>
                    </TableCell>
                    <TableCell>
                      <Chip
                        color={roleColors[member.role] as any}
                        variant="flat"
                        size="sm"
                      >
                        {roleLabels[member.role]}
                      </Chip>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm text-gray-600">
                        {member.last_login_at 
                          ? formatDate(member.last_login_at)
                          : tString('table.never')
                        }
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm text-gray-600">
                        {formatDate(member.created_at)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center space-x-2">
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          color="primary"
                          onPress={() => handleUpdateRole(member)}
                          title={tString('table.updateRoleTooltip')}
                        >
                          <Edit className="w-4 h-4" />
                        </Button>
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          color="danger"
                          onPress={() => handleRemoveStaff(member.id, member.name)}
                          title={tString('table.removeStaffTooltip')}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardBody>
      </Card>

      {/* Pending Invitations */}
      {(invitations.length > 0 || (searchQuery || roleFilter !== 'all')) && (
        <Card className="border border-gray-200">
          <CardHeader className="pb-3">
            <h3 className="text-lg font-medium text-gray-900">{tString('invitations.title')}</h3>
          </CardHeader>
          <CardBody className="pt-0">
            {filteredInvitations.length === 0 ? (
              <div className="text-center py-8">
                <Mail className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                {invitations.length === 0 ? (
                  <>
                    <p className="text-gray-600">{tString('invitations.noPending')}</p>
                    <p className="text-sm text-gray-500 mt-1">{tString('invitations.allAcceptedOrExpired')}</p>
                  </>
                ) : (
                  <>
                    <p className="text-gray-600">{tString('invitations.noFound')}</p>
                    <p className="text-sm text-gray-500 mt-1">{tString('invitations.adjustSearch')}</p>
                    <Button
                      variant="flat"
                      size="sm"
                      startContent={<X className="w-4 h-4" />}
                      onPress={clearSearch}
                      className="mt-3"
                    >
                      {tString('buttons.clearSearch')}
                    </Button>
                  </>
                )}
              </div>
            ) : (
              <Table aria-label="Pending invitations table">
              <TableHeader>
                <TableColumn>{tString('invitations.columns.name')}</TableColumn>
                <TableColumn>{tString('invitations.columns.email')}</TableColumn>
                <TableColumn>{tString('invitations.columns.role')}</TableColumn>
                <TableColumn>{tString('invitations.columns.status')}</TableColumn>
                <TableColumn>{tString('invitations.columns.expires')}</TableColumn>
                <TableColumn>{tString('invitations.columns.sent')}</TableColumn>
                <TableColumn>{tString('invitations.columns.actions')}</TableColumn>
              </TableHeader>
              <TableBody>
                {filteredInvitations.map((invitation) => (
                  <TableRow key={invitation.id}>
                    <TableCell>
                      <div className="font-medium text-gray-900">{invitation.name}</div>
                    </TableCell>
                    <TableCell>
                      <div className="text-gray-600">{invitation.email}</div>
                    </TableCell>
                    <TableCell>
                      <Chip
                        color={roleColors[invitation.role] as any}
                        variant="flat"
                        size="sm"
                      >
                        {roleLabels[invitation.role]}
                      </Chip>
                    </TableCell>
                    <TableCell>
                      <Chip
                        color={isInvitationExpired(invitation.expires_at) ? 'danger' : statusColors[invitation.status || 'pending'] as any}
                        variant="flat"
                        size="sm"
                      >
                        {isInvitationExpired(invitation.expires_at) ? tString('invitations.status.expired') : (invitation.status || tString('invitations.status.pending'))}
                      </Chip>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm text-gray-600">
                        {formatDate(invitation.expires_at)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm text-gray-600">
                        {formatDate(invitation.created_at)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center space-x-2">
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          color="primary"
                          onPress={() => handleResendInvitation(invitation.id, invitation.name)}
                          isDisabled={isInvitationExpired(invitation.expires_at)}
                          title={isInvitationExpired(invitation.expires_at) ? tString('invitations.cannotResendExpired') : tString('invitations.resendTooltip')}
                        >
                          <RefreshCw className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            )}
          </CardBody>
        </Card>
      )}

      {/* Invite Staff Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="lg">
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center space-x-2">
              <Mail className="w-5 h-5 text-blue-600" />
              <span>{tString('modals.invite.title')}</span>
            </div>
          </ModalHeader>
          <ModalBody className="space-y-4">
            <Input
              label={tString('modals.invite.fullName')}
              placeholder={tString('modals.invite.fullNamePlaceholder')}
              value={inviteForm.name}
              onChange={(e) => setInviteForm({ ...inviteForm, name: e.target.value })}
              isRequired
            />
            
            <Input
              label={tString('modals.invite.email')}
              placeholder={tString('modals.invite.emailPlaceholder')}
              type="email"
              value={inviteForm.email}
              onChange={(e) => setInviteForm({ ...inviteForm, email: e.target.value })}
              isRequired
            />
            
            <Select
              key={`invite-role-${inviteForm.role}`}
              label={tString('modals.invite.role')}
              placeholder={tString('modals.invite.rolePlaceholder')}
              defaultSelectedKeys={[inviteForm.role]}
              disallowEmptySelection
              onSelectionChange={(keys) => {
                const selectedKey = Array.from(keys)[0];
                if (selectedKey) {
                  setInviteForm({ ...inviteForm, role: selectedKey as Staff['role'] });
                }
              }}
              isRequired
            >
              {Object.entries(roleLabels).map(([value, label]) => (
                <SelectItem key={value} value={value} textValue={label}>
                  <div>
                    <div className="font-medium">{label}</div>
                    <div className="text-sm text-gray-500">
                      {roleDescriptions[value as keyof typeof roleDescriptions]}
                    </div>
                  </div>
                </SelectItem>
              ))}
            </Select>

            <div className="bg-blue-50 p-4 rounded-lg">
              <p className="text-sm text-blue-800">
                <strong>{tString('modals.invite.noteTitle')}</strong> {tString('modals.invite.noteDescription')}
              </p>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
              {tString('modals.invite.cancel')}
            </Button>
            <Button
              color="primary"
              onPress={handleInviteStaff}
              isLoading={inviteLoading}
              className="bg-gray-900 text-white"
            >
              {tString('modals.invite.sendInvitation')}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Update Role Modal */}
      <Modal isOpen={isRoleModalOpen} onClose={onRoleModalClose} size="md">
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center space-x-2">
              <Edit className="w-5 h-5 text-blue-600" />
              <span>{tString('modals.updateRole.title')}</span>
            </div>
          </ModalHeader>
          <ModalBody className="space-y-4">
            {selectedStaff && (
              <>
                <div className="text-sm text-gray-600">
                  {tString('modals.updateRole.updatingFor')} <strong>{selectedStaff.name}</strong> ({selectedStaff.email})
                </div>
                
                <Select
                  key={`role-select-${selectedStaff.id}-${newRole}`}
                  label={tString('modals.updateRole.newRole')}
                  placeholder={tString('modals.updateRole.selectRole')}
                  defaultSelectedKeys={[newRole]}
                  disallowEmptySelection
                  onSelectionChange={(keys) => {
                    const selectedKey = Array.from(keys)[0];
                    if (selectedKey) {
                      setNewRole(selectedKey as Staff['role']);
                    }
                  }}
                  isRequired
                >
                  {Object.entries(roleLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value} textValue={label}>
                      <div>
                        <div className="font-medium">{label}</div>
                        <div className="text-sm text-gray-500">
                          {roleDescriptions[value as keyof typeof roleDescriptions]}
                        </div>
                      </div>
                    </SelectItem>
                  ))}
                </Select>

                <div className="bg-amber-50 p-4 rounded-lg">
                  <p className="text-sm text-amber-800">
                    <strong>{tString('modals.updateRole.noteTitle')}</strong> {tString('modals.updateRole.noteDescription')}
                  </p>
                </div>
              </>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onRoleModalClose}>
              {tString('buttons.cancel')}
            </Button>
            <Button
              color="primary"
              onPress={handleRoleUpdateSubmit}
              isLoading={roleUpdateLoading}
              className="bg-gray-900 text-white"
            >
              {tString('buttons.updateRole')}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
