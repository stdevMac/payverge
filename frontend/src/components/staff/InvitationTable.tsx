'use client';

import React from 'react';
import { Button, Table, TableHeader, TableColumn, TableBody, TableRow, TableCell, Chip } from '@nextui-org/react';
import { Mail, RefreshCw } from 'lucide-react';
import * as StaffAPI from '../../api/staff';

type PendingInvitation = StaffAPI.StaffInvitation;
type Staff = StaffAPI.StaffMember;

interface InvitationTableProps {
  invitations: PendingInvitation[];
  getRoleLabel: (role: Staff['role']) => string;
  tString: (key: string) => string;
  formatDate: (dateString: string) => string;
  isInvitationExpired: (expiresAt: string) => boolean;
  handleResendInvitation: (invitationId: number, staffName: string) => void;
}

const statusColors = {
  pending: 'warning',
  accepted: 'success',
  expired: 'danger',
  revoked: 'default',
} as const;

const roleColors = {
  manager: 'primary',
  server: 'success',
  host: 'warning',
  kitchen: 'secondary',
} as const;

export default function InvitationTable({
  invitations,
  getRoleLabel,
  tString,
  formatDate,
  isInvitationExpired,
  handleResendInvitation
}: InvitationTableProps) {
  if (invitations.length === 0) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <Mail className="w-8 h-8 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString('invitations.noPending')}</h3>
          <p className="text-gray-600 font-light text-sm">{tString('invitations.allAcceptedOrExpired')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
      <div className="border-b border-gray-200 p-6">
        <div className="flex items-center gap-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <Mail className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('invitations.title')}</h2>
            <p className="text-gray-600 font-light text-sm">Track pending staff invitations</p>
          </div>
        </div>
      </div>
      <div className="p-6">
        <Table aria-label="Pending invitations table">
          <TableHeader>
            <TableColumn>{tString('invitations.columns.name')}</TableColumn>
            <TableColumn>{tString('invitations.columns.email')}</TableColumn>
            <TableColumn>{tString('invitations.columns.role')}</TableColumn>
            <TableColumn>{tString('invitations.columns.status')}</TableColumn>
            <TableColumn>{tString('invitations.columns.expires')}</TableColumn>
            <TableColumn>{tString('invitations.columns.actions')}</TableColumn>
          </TableHeader>
          <TableBody>
            {invitations.map((invitation) => {
              const expired = isInvitationExpired(invitation.expires_at);
              const status = expired ? 'expired' : (invitation.status || 'pending');
              
              return (
                <TableRow key={invitation.id}>
                  <TableCell>
                    <div className="flex flex-col">
                      <p className="font-medium text-gray-900">{invitation.name}</p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <span className="text-gray-600">{invitation.email}</span>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="sm"
                      variant="flat"
                      color={roleColors[invitation.role]}
                    >
                      {getRoleLabel(invitation.role)}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="sm"
                      variant="flat"
                      color={statusColors[status as keyof typeof statusColors]}
                    >
                      {tString(`invitations.status.${status}`)}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <span className={`text-sm ${expired ? 'text-red-600' : 'text-gray-600'}`}>
                      {formatDate(invitation.expires_at)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      {(status === 'pending' || status === 'expired') && (
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          onPress={() => handleResendInvitation(invitation.id, invitation.name)}
                          className="text-gray-600 hover:text-blue-600"
                        >
                          <RefreshCw className="w-4 h-4" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
