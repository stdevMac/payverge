'use client';

import React from 'react';
import { Users, Clock, CheckCircle } from 'lucide-react';
import * as StaffAPI from '../../api/staff';

type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

interface StaffOverviewCardsProps {
  staff: Staff[];
  invitations: PendingInvitation[];
  tString: (key: string) => string;
}

export default function StaffOverviewCards({ staff, invitations, tString }: StaffOverviewCardsProps) {
  const pendingInvitations = invitations.filter(inv => (inv.status || 'pending') === 'pending');
  const acceptedInvitations = invitations.filter(inv => inv.status === 'accepted');

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-6">
      {/* Active Staff */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
        <div className="flex items-center space-x-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <Users className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <p className="text-2xl font-light text-gray-900 tracking-wide">{staff.length}</p>
            <p className="text-gray-600 font-light text-sm">{tString('overview.activeStaff')}</p>
          </div>
        </div>
      </div>

      {/* Pending Invitations */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
        <div className="flex items-center space-x-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <Clock className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <p className="text-2xl font-light text-gray-900 tracking-wide">{pendingInvitations.length}</p>
            <p className="text-gray-600 font-light text-sm">{tString('overview.pendingInvitations')}</p>
          </div>
        </div>
      </div>

      {/* Accepted This Month */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
        <div className="flex items-center space-x-3">
          <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
            <CheckCircle className="w-6 h-6 text-gray-700" />
          </div>
          <div>
            <p className="text-2xl font-light text-gray-900 tracking-wide">{acceptedInvitations.length}</p>
            <p className="text-gray-600 font-light text-sm">{tString('overview.activeThisWeek')}</p>
          </div>
        </div>
      </div>
    </div>
  );
}
