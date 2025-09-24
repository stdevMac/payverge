'use client';

import React from 'react';
import { useParams } from 'next/navigation';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Breadcrumbs,
  BreadcrumbItem
} from '@nextui-org/react';
import { ArrowLeft, Receipt, CreditCard } from 'lucide-react';
import Link from 'next/link';
import AlternativePaymentManager from '@/components/business/AlternativePaymentManager';

export default function AlternativePaymentsPage() {
  const params = useParams();
  const businessId = params.businessId as string;
  const billId = params.billId as string;

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Animated Background */}
      <div className="fixed inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-blue-50 rounded-full blur-3xl opacity-70 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-80 h-80 bg-purple-50 rounded-full blur-3xl opacity-70 animate-pulse delay-1000"></div>
        <div className="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 w-96 h-96 bg-gray-50 rounded-full blur-3xl opacity-50 animate-pulse delay-500"></div>
      </div>

      <div className="relative z-10">
        {/* Header */}
        <div className="bg-white/80 backdrop-blur-xl border-b border-gray-200 sticky top-0 z-20">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="flex items-center justify-between h-16">
              <div className="flex items-center space-x-4">
                <Link href={`/business/${businessId}/bills`}>
                  <Button
                    variant="light"
                    size="sm"
                    startContent={<ArrowLeft className="w-4 h-4" />}
                  >
                    Back to Bills
                  </Button>
                </Link>
                
                <Breadcrumbs>
                  <BreadcrumbItem>
                    <Link href={`/business/${businessId}`}>Business</Link>
                  </BreadcrumbItem>
                  <BreadcrumbItem>
                    <Link href={`/business/${businessId}/bills`}>Bills</Link>
                  </BreadcrumbItem>
                  <BreadcrumbItem>Bill #{billId}</BreadcrumbItem>
                  <BreadcrumbItem>Alternative Payments</BreadcrumbItem>
                </Breadcrumbs>
              </div>

              <div className="flex items-center space-x-2">
                <Receipt className="w-5 h-5 text-gray-600" />
                <span className="text-sm font-medium text-gray-700">
                  Payment Management
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* Main Content */}
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
          <div className="mb-8">
            <div className="flex items-center space-x-3 mb-2">
              <CreditCard className="w-8 h-8 text-blue-600" />
              <h1 className="text-3xl font-light text-gray-900 tracking-wide">
                Alternative Payment Management
              </h1>
            </div>
            <p className="text-gray-600 font-light">
              Manage cash, card, and other non-crypto payments for this bill
            </p>
          </div>

          {/* Alternative Payment Manager */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            <div className="lg:col-span-2">
              <AlternativePaymentManager
                billId={billId}
                billTotal={100} // This would come from bill data
                onPaymentMarked={() => {
                  console.log('Payment marked, refreshing data...');
                }}
              />
            </div>

            {/* Sidebar with Instructions */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-lg font-semibold">How It Works</h3>
                </CardHeader>
                <CardBody className="space-y-4">
                  <div className="space-y-3 text-sm">
                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        1
                      </div>
                      <p>Customers can request alternative payments through the guest interface</p>
                    </div>
                    
                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        2
                      </div>
                      <p>You receive payment requests in real-time</p>
                    </div>
                    
                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        3
                      </div>
                      <p>Confirm payments after receiving cash, card, or other payments</p>
                    </div>
                    
                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        4
                      </div>
                      <p>Bill automatically completes when fully paid</p>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-lg font-semibold">Payment Methods</h3>
                </CardHeader>
                <CardBody>
                  <div className="space-y-3">
                    <div className="flex items-center space-x-3">
                      <span className="text-xl">💵</span>
                      <span className="text-sm">Cash payments</span>
                    </div>
                    <div className="flex items-center space-x-3">
                      <span className="text-xl">💳</span>
                      <span className="text-sm">Credit/Debit cards</span>
                    </div>
                    <div className="flex items-center space-x-3">
                      <span className="text-xl">📱</span>
                      <span className="text-sm">Venmo/PayPal</span>
                    </div>
                    <div className="flex items-center space-x-3">
                      <span className="text-xl">🔄</span>
                      <span className="text-sm">Other methods</span>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card className="bg-green-50 border-green-200">
                <CardBody>
                  <div className="text-center">
                    <div className="text-green-600 mb-2">
                      <Receipt className="w-8 h-8 mx-auto" />
                    </div>
                    <h4 className="font-semibold text-green-800 mb-1">Hybrid Payments</h4>
                    <p className="text-sm text-green-700">
                      Customers can mix crypto and traditional payments on the same bill
                    </p>
                  </div>
                </CardBody>
              </Card>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
