'use client';

import { CheckCircle, Zap, Shield, Globe, DollarSign, Clock, Users } from 'lucide-react';

export function Hero() {
  return (
    <div className="bg-gradient-to-br from-primary-50 via-blue-50 to-indigo-100 py-20">
      <div className="container mx-auto px-4">
        <div className="text-center max-w-5xl mx-auto">
          <div className="mb-6">
            <span className="inline-flex items-center px-4 py-2 rounded-full text-sm font-medium bg-primary-100 text-primary-800 mb-4">
              🚀 The Future of Freelance Payments
            </span>
          </div>
          
          <h1 className="text-5xl md:text-7xl font-bold text-gray-900 mb-8 leading-tight">
            Invoice. Pay.{' '}
            <span className="text-transparent bg-clip-text bg-gradient-to-r from-primary-600 to-blue-600">
              Done.
            </span>
            <br />
            <span className="text-3xl md:text-4xl font-semibold text-gray-700">
              Skip the banks, get paid in crypto
            </span>
          </h1>
          
          <p className="text-xl md:text-2xl text-gray-600 mb-12 max-w-3xl mx-auto leading-relaxed">
            The only invoice platform that pays you instantly in USDC. 
            No more waiting days for payments or losing money to high fees.
          </p>

          {/* Key Benefits Grid */}
          <div className="grid md:grid-cols-3 gap-8 mb-16">
            <div className="bg-white rounded-2xl p-8 shadow-xl border border-gray-100 hover:shadow-2xl transition-all duration-300">
              <div className="bg-gradient-to-br from-primary-500 to-primary-600 rounded-2xl w-16 h-16 flex items-center justify-center mx-auto mb-6">
                <Clock className="h-8 w-8 text-white" />
              </div>
              <h3 className="text-xl font-bold text-gray-900 mb-3">
                Instant Settlement
              </h3>
              <p className="text-gray-600 leading-relaxed">
                Money hits your wallet in seconds, not days. No more chasing payments or waiting for bank transfers.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 shadow-xl border border-gray-100 hover:shadow-2xl transition-all duration-300">
              <div className="bg-gradient-to-br from-success-500 to-success-600 rounded-2xl w-16 h-16 flex items-center justify-center mx-auto mb-6">
                <DollarSign className="h-8 w-8 text-white" />
              </div>
              <h3 className="text-xl font-bold text-gray-900 mb-3">
                Ultra-Low Fees
              </h3>
              <p className="text-gray-600 leading-relaxed">
                Just 1% fee (max $10) vs 3-5% for traditional processors. Keep more of what you earn.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 shadow-xl border border-gray-100 hover:shadow-2xl transition-all duration-300">
              <div className="bg-gradient-to-br from-blue-500 to-blue-600 rounded-2xl w-16 h-16 flex items-center justify-center mx-auto mb-6">
                <Globe className="h-8 w-8 text-white" />
              </div>
              <h3 className="text-xl font-bold text-gray-900 mb-3">
                Borderless Payments
              </h3>
              <p className="text-gray-600 leading-relaxed">
                Accept payments from any country without currency conversion fees or banking restrictions.
              </p>
            </div>
          </div>

          {/* Additional Benefits */}
          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6 mb-16">
            <div className="bg-white rounded-2xl p-6 shadow-lg border border-gray-100">
              <div className="flex items-start space-x-3">
                <div className="bg-primary-100 rounded-lg p-2">
                  <Shield className="h-5 w-5 text-primary-600" />
                </div>
                <div>
                  <h4 className="text-base font-semibold text-gray-900 mb-1">Smart Contract Security</h4>
                  <p className="text-sm text-gray-600">Audited smart contracts. No middleman, no custody risk.</p>
                </div>
              </div>
            </div>

            <div className="bg-white rounded-2xl p-6 shadow-lg border border-gray-100">
              <div className="flex items-start space-x-3">
                <div className="bg-success-100 rounded-lg p-2">
                  <Users className="h-5 w-5 text-success-600" />
                </div>
                <div>
                  <h4 className="text-base font-semibold text-gray-900 mb-1">Auto Client Reminders</h4>
                  <p className="text-sm text-gray-600">We chase payments for you. Automated email reminders and follow-ups.</p>
                </div>
              </div>
            </div>

            <div className="bg-white rounded-2xl p-6 shadow-lg border border-gray-100">
              <div className="flex items-start space-x-3">
                <div className="bg-blue-100 rounded-lg p-2">
                  <Zap className="h-5 w-5 text-blue-600" />
                </div>
                <div>
                  <h4 className="text-base font-semibold text-gray-900 mb-1">Real-Time Tracking</h4>
                  <p className="text-sm text-gray-600">Live payment status updates. Know exactly when you get paid.</p>
                </div>
              </div>
            </div>

            <div className="bg-white rounded-2xl p-6 shadow-lg border border-gray-100">
              <div className="flex items-start space-x-3">
                <div className="bg-purple-100 rounded-lg p-2">
                  <Globe className="h-5 w-5 text-purple-600" />
                </div>
                <div>
                  <h4 className="text-base font-semibold text-gray-900 mb-1">Payment Insights</h4>
                  <p className="text-sm text-gray-600">Track earnings, client patterns, and payment trends over time.</p>
                </div>
              </div>
            </div>
          </div>

          {/* Pricing Highlight */}
          <div className="bg-gradient-to-r from-primary-500 to-blue-600 rounded-3xl p-8 text-white shadow-2xl max-w-2xl mx-auto">
            <div className="text-center">
              <div className="flex items-center justify-center space-x-2 mb-4">
                <CheckCircle className="h-6 w-6 text-white" />
                <span className="text-lg font-semibold">Transparent Pricing</span>
              </div>
              <div className="text-4xl font-bold mb-2">
                1% fee (max $10)
              </div>
              <p className="text-blue-100 text-lg mb-4">
                PayPal charges 3.49% + $0.49. Stripe charges 2.9% + $0.30.
              </p>
              <div className="bg-white/20 rounded-xl p-4">
                <p className="text-white font-medium">
                  On a $1000 invoice: You keep $990 (vs $965 with PayPal)
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
