'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Divider,
} from '@nextui-org/react';
import { Mail, Key, ArrowRight, Users } from 'lucide-react';
import { toast } from 'react-hot-toast';
import * as StaffAPI from '../../api/staff';

interface StaffLoginProps {
  onLoginSuccess?: (staffData: any) => void;
}

export default function StaffLogin({ onLoginSuccess }: StaffLoginProps) {
  const [step, setStep] = useState<'email' | 'code'>('email');
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [loading, setLoading] = useState(false);
  const [codeLoading, setCodeLoading] = useState(false);

  // Request login code
  const handleRequestCode = async () => {
    if (!email) {
      toast.error('Please enter your email address');
      return;
    }

    try {
      setLoading(true);
      await StaffAPI.requestLoginCode({ email });
      toast.success('Login code sent to your email!');
      setStep('code');
    } catch (error: any) {
      console.error('Error requesting login code:', error);
      toast.error(error.message || 'Failed to send login code');
    } finally {
      setLoading(false);
    }
  };

  // Verify login code
  const handleVerifyCode = async () => {
    if (!code) {
      toast.error('Please enter the login code');
      return;
    }

    try {
      setCodeLoading(true);
      const data = await StaffAPI.verifyLoginCode({ email, code });
      
      // Store staff session data
      localStorage.setItem('staff_token', data.token);
      localStorage.setItem('staff_data', JSON.stringify(data.staff));
      
      toast.success('Login successful!');
      
      if (onLoginSuccess) {
        onLoginSuccess(data.staff);
      }
    } catch (error: any) {
      console.error('Error verifying code:', error);
      toast.error(error.message || 'Invalid login code');
    } finally {
      setCodeLoading(false);
    }
  };

  const handleBackToEmail = () => {
    setStep('email');
    setCode('');
  };

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
                  <Users className="w-8 h-8 text-white" />
                </div>
              </div>
              <h1 className="text-2xl font-light tracking-wide text-gray-900">
                Staff Login
              </h1>
              <p className="text-gray-600 mt-2">
                {step === 'email' 
                  ? 'Enter your work email to receive a login code'
                  : 'Enter the 6-digit code sent to your email'
                }
              </p>
            </div>
          </CardHeader>

          <CardBody className="pt-2">
            {step === 'email' ? (
              <div className="space-y-6">
                <Input
                  type="email"
                  label="Work Email"
                  placeholder="Enter your work email address"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  startContent={<Mail className="w-4 h-4 text-gray-400" />}
                  variant="bordered"
                  size="lg"
                  classNames={{
                    input: "text-base",
                    inputWrapper: "border-gray-200 hover:border-gray-300 focus-within:border-blue-500",
                  }}
                  onKeyPress={(e) => {
                    if (e.key === 'Enter') {
                      handleRequestCode();
                    }
                  }}
                />

                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-gray-900 text-white font-medium"
                  onPress={handleRequestCode}
                  isLoading={loading}
                  endContent={!loading && <ArrowRight className="w-4 h-4" />}
                >
                  {loading ? 'Sending Code...' : 'Send Login Code'}
                </Button>

                <div className="text-center">
                  <p className="text-sm text-gray-500">
                    Don&apos;t have an account? Contact your manager for an invitation.
                  </p>
                </div>
              </div>
            ) : (
              <div className="space-y-6">
                <div className="text-center">
                  <div className="inline-flex items-center space-x-2 bg-blue-50 px-4 py-2 rounded-full">
                    <Mail className="w-4 h-4 text-blue-600" />
                    <span className="text-sm text-blue-800 font-medium">{email}</span>
                  </div>
                </div>

                <Input
                  type="text"
                  label="Login Code"
                  placeholder="Enter 6-digit code"
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  startContent={<Key className="w-4 h-4 text-gray-400" />}
                  variant="bordered"
                  size="lg"
                  classNames={{
                    input: "text-base text-center tracking-widest font-mono",
                    inputWrapper: "border-gray-200 hover:border-gray-300 focus-within:border-blue-500",
                  }}
                  onKeyPress={(e) => {
                    if (e.key === 'Enter') {
                      handleVerifyCode();
                    }
                  }}
                />

                <div className="space-y-3">
                  <Button
                    color="primary"
                    size="lg"
                    className="w-full bg-gray-900 text-white font-medium"
                    onPress={handleVerifyCode}
                    isLoading={codeLoading}
                    isDisabled={code.length !== 6}
                  >
                    {codeLoading ? 'Verifying...' : 'Verify & Login'}
                  </Button>

                  <div className="flex space-x-2">
                    <Button
                      variant="light"
                      size="lg"
                      className="flex-1"
                      onPress={handleBackToEmail}
                    >
                      Back
                    </Button>
                    <Button
                      variant="light"
                      size="lg"
                      className="flex-1"
                      onPress={handleRequestCode}
                      isLoading={loading}
                    >
                      Resend Code
                    </Button>
                  </div>
                </div>

                <div className="text-center">
                  <p className="text-sm text-gray-500">
                    Code expires in 10 minutes. Check your spam folder if you don&apos;t see it.
                  </p>
                </div>
              </div>
            )}

            <Divider className="my-6" />

            <div className="text-center">
              <p className="text-xs text-gray-400">
                Secure staff access powered by Payverge
              </p>
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}
