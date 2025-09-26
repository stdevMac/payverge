'use client';

import React, { useState, useEffect } from 'react';
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
import { UserPlus, Mail, Trash2, Users, Clock, CheckCircle, Plus, RefreshCw } from 'lucide-react';
import { toast } from 'react-hot-toast';
import * as StaffAPI from '../../api/staff';

// Use types from API
type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

interface StaffManagementProps {
  businessId: string;
}

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
  const [staff, setStaff] = useState<Staff[]>([]);
  const [invitations, setInvitations] = useState<PendingInvitation[]>([]);
  const [loading, setLoading] = useState(true);
  const [inviteLoading, setInviteLoading] = useState(false);
  
  const { isOpen, onOpen, onClose } = useDisclosure();
  
  // Invite form state
  const [inviteForm, setInviteForm] = useState({
    email: '',
    name: '',
    role: 'server' as Staff['role'],
  });

  // Load staff and invitations
  const loadStaffData = async () => {
    try {
      setLoading(true);
      const data = await StaffAPI.getBusinessStaff(businessId);
      setStaff(data.staff || []);
      setInvitations(data.pending_invitations || []);
    } catch (error) {
      console.error('Error loading staff data:', error);
      toast.error('Failed to load staff data');
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
      toast.error('Please fill in all fields');
      return;
    }

    try {
      setInviteLoading(true);
      await StaffAPI.inviteStaff(businessId, inviteForm);
      
      toast.success('Staff invitation sent successfully!');
      setInviteForm({ email: '', name: '', role: 'server' });
      onClose();
      loadStaffData(); // Reload to show new invitation
    } catch (error: any) {
      console.error('Error inviting staff:', error);
      toast.error(error.message || 'Failed to send invitation');
    } finally {
      setInviteLoading(false);
    }
  };

  // Handle staff removal
  const handleRemoveStaff = async (staffId: number, staffName: string) => {
    if (!confirm(`Are you sure you want to remove ${staffName} from your staff?`)) {
      return;
    }

    try {
      await StaffAPI.removeStaff(businessId, staffId);
      toast.success('Staff member removed successfully');
      loadStaffData(); // Reload staff list
    } catch (error) {
      console.error('Error removing staff:', error);
      toast.error('Failed to remove staff member');
    }
  };

  // Handle resend invitation
  const handleResendInvitation = async (invitationId: number, staffName: string) => {
    if (!confirm(`Resend invitation to ${staffName}? This will generate a new invitation link and extend the expiry date.`)) {
      return;
    }

    try {
      await StaffAPI.resendInvitation(businessId, invitationId);
      toast.success('Invitation resent successfully!');
      loadStaffData(); // Reload to show updated expiry
    } catch (error: any) {
      console.error('Error resending invitation:', error);
      toast.error(error.message || 'Failed to resend invitation');
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
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-2xl font-light tracking-wide text-gray-900">Staff Management</h2>
          <p className="text-gray-600 mt-1">Invite and manage your restaurant staff</p>
        </div>
        <Button
          color="primary"
          startContent={<Plus className="w-4 h-4" />}
          onPress={onOpen}
          className="bg-gray-900 text-white"
        >
          Invite Staff
        </Button>
      </div>

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
                <p className="text-sm text-gray-600">Active Staff</p>
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
                <p className="text-sm text-gray-600">Pending Invitations</p>
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
                <p className="text-sm text-gray-600">Active This Week</p>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Active Staff Table */}
      <Card className="border border-gray-200">
        <CardHeader className="pb-3">
          <h3 className="text-lg font-medium text-gray-900">Active Staff Members</h3>
        </CardHeader>
        <CardBody className="pt-0">
          {staff.length === 0 ? (
            <div className="text-center py-8">
              <Users className="w-12 h-12 text-gray-400 mx-auto mb-4" />
              <p className="text-gray-600">No staff members yet</p>
              <p className="text-sm text-gray-500 mt-1">Invite your first staff member to get started</p>
            </div>
          ) : (
            <Table aria-label="Staff members table">
              <TableHeader>
                <TableColumn>NAME</TableColumn>
                <TableColumn>EMAIL</TableColumn>
                <TableColumn>ROLE</TableColumn>
                <TableColumn>LAST LOGIN</TableColumn>
                <TableColumn>JOINED</TableColumn>
                <TableColumn>ACTIONS</TableColumn>
              </TableHeader>
              <TableBody>
                {staff.map((member) => (
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
                          : 'Never'
                        }
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm text-gray-600">
                        {formatDate(member.created_at)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        color="danger"
                        onPress={() => handleRemoveStaff(member.id, member.name)}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardBody>
      </Card>

      {/* Pending Invitations */}
      {invitations.length > 0 && (
        <Card className="border border-gray-200">
          <CardHeader className="pb-3">
            <h3 className="text-lg font-medium text-gray-900">Pending Invitations</h3>
          </CardHeader>
          <CardBody className="pt-0">
            <Table aria-label="Pending invitations table">
              <TableHeader>
                <TableColumn>NAME</TableColumn>
                <TableColumn>EMAIL</TableColumn>
                <TableColumn>ROLE</TableColumn>
                <TableColumn>STATUS</TableColumn>
                <TableColumn>EXPIRES</TableColumn>
                <TableColumn>SENT</TableColumn>
                <TableColumn>ACTIONS</TableColumn>
              </TableHeader>
              <TableBody>
                {invitations.map((invitation) => (
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
                        {isInvitationExpired(invitation.expires_at) ? 'Expired' : (invitation.status || 'pending')}
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
                          title={isInvitationExpired(invitation.expires_at) ? "Cannot resend expired invitation" : "Resend invitation"}
                        >
                          <RefreshCw className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardBody>
        </Card>
      )}

      {/* Invite Staff Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="lg">
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center space-x-2">
              <Mail className="w-5 h-5 text-blue-600" />
              <span>Invite Staff Member</span>
            </div>
          </ModalHeader>
          <ModalBody className="space-y-4">
            <Input
              label="Full Name"
              placeholder="Enter staff member's full name"
              value={inviteForm.name}
              onChange={(e) => setInviteForm({ ...inviteForm, name: e.target.value })}
              isRequired
            />
            
            <Input
              label="Email Address"
              placeholder="Enter staff member's email"
              type="email"
              value={inviteForm.email}
              onChange={(e) => setInviteForm({ ...inviteForm, email: e.target.value })}
              isRequired
            />
            
            <Select
              label="Role"
              placeholder="Select a role"
              selectedKeys={[inviteForm.role]}
              onSelectionChange={(keys) => {
                const role = Array.from(keys)[0] as Staff['role'];
                setInviteForm({ ...inviteForm, role });
              }}
              isRequired
            >
              {Object.entries(roleLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>
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
                <strong>Note:</strong> The staff member will receive an email invitation with instructions 
                to create their account. They&apos;ll be able to log in using email codes without needing a crypto wallet.
              </p>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
            </Button>
            <Button
              color="primary"
              onPress={handleInviteStaff}
              isLoading={inviteLoading}
              className="bg-gray-900 text-white"
            >
              Send Invitation
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
