'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Chip,
  Divider,
} from '@nextui-org/react';
import { UserPlus, Building, Mail, User, CheckCircle } from 'lucide-react';
import { toast } from 'react-hot-toast';
import { useRouter, useSearchParams } from 'next/navigation';
import * as StaffAPI from '../../api/staff';

interface AcceptInvitationProps {
  token?: string;
}

const roleLabels = {
  manager: 'Manager',
  server: 'Server',
  host: 'Host',
  kitchen: 'Kitchen',
};

const roleColors = {
  manager: 'primary',
  server: 'success',
  host: 'warning',
  kitchen: 'secondary',
} as const;

export default function AcceptInvitation({ token: propToken }: AcceptInvitationProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = propToken || searchParams.get('token');

  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);
  const [invitationData, setInvitationData] = useState<any>(null);
  const [step, setStep] = useState<'loading' | 'form' | 'success'>('loading');

  useEffect(() => {
    if (!token) {
      toast.error('Invalid invitation link');
      router.push('/staff/login');
      return;
    }

    // For now, we'll skip the invitation validation step since we don't have a backend endpoint for it
    // In a real implementation, you'd validate the token first
    setStep('form');
  }, [token, router]);

  const handleAcceptInvitation = async () => {
    if (!name.trim()) {
      toast.error('Please enter your full name');
      return;
    }

    if (!token) {
      toast.error('Invalid invitation token');
      return;
    }

    try {
      setLoading(true);
      const data = await StaffAPI.acceptInvitation({
        token,
        name: name.trim(),
      });
      
      setInvitationData(data);
      setStep('success');
      toast.success('Account created successfully!');
    } catch (error: any) {
      console.error('Error accepting invitation:', error);
      toast.error(error.message || 'Failed to accept invitation');
    } finally {
      setLoading(false);
    }
  };

  const handleContinueToLogin = () => {
    router.push('/staff/login');
  };

  if (step === 'loading') {
    return (
      <div className="min-h-screen bg-gradient-to-br from-slate-50 to-blue-50 flex items-center justify-center p-4">
        <Card className="backdrop-blur-xl bg-white/80 border border-gray-200 shadow-xl">
          <CardBody className="p-8 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full mx-auto mb-4"></div>
            <p className="text-gray-600">Validating invitation...</p>
          </CardBody>
        </Card>
      </div>
    );
  }

  if (step === 'success') {
    return (
      <div className="min-h-screen bg-gradient-to-br from-slate-50 to-blue-50 flex items-center justify-center p-4">
        {/* Animated background elements */}
        <div className="absolute inset-0 overflow-hidden">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-blue-50 rounded-full blur-3xl opacity-70 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-80 h-80 bg-purple-50 rounded-full blur-3xl opacity-70 animate-pulse" style={{ animationDelay: '1s' }}></div>
          <div className="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 w-60 h-60 bg-gray-50 rounded-full blur-3xl opacity-50 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>

        <div className="relative w-full max-w-md">
          <Card className="backdrop-blur-xl bg-white/80 border border-gray-200 shadow-xl">
            <CardBody className="p-8 text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 bg-gradient-to-br from-green-500 to-emerald-600 rounded-2xl shadow-lg">
                  <CheckCircle className="w-12 h-12 text-white" />
                </div>
              </div>

              <div>
                <h1 className="text-2xl font-light tracking-wide text-gray-900 mb-2">
                  Welcome to the Team!
                </h1>
                <p className="text-gray-600">
                  Your staff account has been created successfully.
                </p>
              </div>

              {invitationData && (
                <div className="bg-gray-50 p-4 rounded-xl space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-600">Business:</span>
                    <span className="font-medium text-gray-900">{invitationData.business_name || 'Restaurant'}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-600">Role:</span>
                    <Chip
                      color={roleColors[invitationData.role as keyof typeof roleColors] || 'default'}
                      variant="flat"
                      size="sm"
                    >
                      {roleLabels[invitationData.role as keyof typeof roleLabels] || invitationData.role}
                    </Chip>
                  </div>
                </div>
              )}

              <div className="space-y-3">
                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-gray-900 text-white font-medium"
                  onPress={handleContinueToLogin}
                >
                  Continue to Login
                </Button>
              </div>

              <div className="text-center">
                <p className="text-sm text-gray-500">
                  You can now log in using your email address and login codes.
                </p>
              </div>
            </CardBody>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-50 to-blue-50 flex items-center justify-center p-4">
      {/* Animated background elements */}
      <div className="absolute inset-0 overflow-hidden">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-blue-50 rounded-full blur-3xl opacity-70 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-80 h-80 bg-purple-50 rounded-full blur-3xl opacity-70 animate-pulse" style={{ animationDelay: '1s' }}></div>
        <div className="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 w-60 h-60 bg-gray-50 rounded-full blur-3xl opacity-50 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      <div className="relative w-full max-w-md">
        <Card className="backdrop-blur-xl bg-white/80 border border-gray-200 shadow-xl">
          <CardHeader className="text-center pb-2">
            <div className="w-full">
              <div className="flex justify-center mb-4">
                <div className="p-3 bg-gradient-to-br from-blue-500 to-purple-600 rounded-2xl shadow-lg">
                  <UserPlus className="w-8 h-8 text-white" />
                </div>
              </div>
              <h1 className="text-2xl font-light tracking-wide text-gray-900">
                Join the Team
              </h1>
              <p className="text-gray-600 mt-2">
                You&apos;ve been invited to join as a staff member
              </p>
            </div>
          </CardHeader>

          <CardBody className="pt-2">
            <div className="space-y-6">
              {/* Invitation Info */}
              <div className="bg-blue-50 p-4 rounded-xl space-y-3">
                <div className="flex items-center space-x-2">
                  <Building className="w-4 h-4 text-blue-600" />
                  <span className="text-sm font-medium text-blue-800">Restaurant Invitation</span>
                </div>
                <p className="text-sm text-blue-700">
                  You&apos;ve been invited to join the staff team. Complete your registration below to get started.
                </p>
              </div>

              {/* Name Input */}
              <Input
                type="text"
                label="Full Name"
                placeholder="Enter your full name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                startContent={<User className="w-4 h-4 text-gray-400" />}
                variant="bordered"
                size="lg"
                classNames={{
                  input: "text-base",
                  inputWrapper: "border-gray-200 hover:border-gray-300 focus-within:border-blue-500",
                }}
                onKeyPress={(e) => {
                  if (e.key === 'Enter') {
                    handleAcceptInvitation();
                  }
                }}
              />

              {/* Accept Button */}
              <Button
                color="primary"
                size="lg"
                className="w-full bg-gray-900 text-white font-medium"
                onPress={handleAcceptInvitation}
                isLoading={loading}
                isDisabled={!name.trim()}
              >
                {loading ? 'Creating Account...' : 'Accept Invitation'}
              </Button>

              {/* Info */}
              <div className="bg-gray-50 p-4 rounded-xl">
                <h4 className="font-medium text-gray-900 mb-2">What happens next?</h4>
                <ul className="text-sm text-gray-600 space-y-1">
                  <li>• Your staff account will be created</li>
                  <li>• You&apos;ll be able to log in with your email address and login codes</li>
                  <li>• Access will be based on your assigned role</li>
                  <li>• No crypto wallet required</li>
                </ul>
              </div>
            </div>

            <Divider className="my-6" />

            <div className="text-center">
              <p className="text-xs text-gray-400">
                Secure staff onboarding powered by Payverge
              </p>
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}
