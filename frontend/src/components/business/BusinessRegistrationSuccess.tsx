"use client";
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Card, CardBody, Button } from '@nextui-org/react';
import { Check, ArrowRight, Building2, CreditCard, QrCode, BarChart3, Users, Sparkles } from 'lucide-react';
import Link from 'next/link';

interface BusinessRegistrationSuccessProps {
  businessName: string;
  businessId: string;
  transactionHash?: string;
}

export default function BusinessRegistrationSuccess({ 
  businessName, 
  businessId, 
  transactionHash 
}: BusinessRegistrationSuccessProps) {
  const router = useRouter();
  const [isNavigating, setIsNavigating] = useState(false);

  const handleGoToDashboard = () => {
    setIsNavigating(true);
    router.push(`/business/${businessId}/dashboard?welcome=true`);
  };

  const features = [
    {
      icon: QrCode,
      title: "QR Code Menus",
      description: "Create digital menus with QR codes for contactless dining"
    },
    {
      icon: CreditCard,
      title: "Crypto Payments",
      description: "Accept USDC payments instantly with zero chargebacks"
    },
    {
      icon: BarChart3,
      title: "Real-time Analytics",
      description: "Track sales, popular items, and customer insights"
    },
    {
      icon: Users,
      title: "Staff Management",
      description: "Invite team members and manage permissions"
    }
  ];

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white relative overflow-hidden">
      {/* Clean, minimal background */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
      </div>

      <div className="max-w-4xl mx-auto px-6 py-12 relative">
        {/* Success Header */}
        <div className="text-center mb-16">
          <div className="w-24 h-24 bg-green-50 border-2 border-green-200 rounded-full flex items-center justify-center mx-auto mb-8 shadow-lg">
            <Check className="text-green-600" size={48} />
          </div>
          
          <div className="inline-flex items-center px-4 py-2 bg-green-50 border border-green-200 rounded-full text-sm font-medium text-green-700 mb-8 shadow-sm">
            <div className="w-2 h-2 bg-green-500 rounded-full mr-2"></div>
            Registration Complete
          </div>

          <h1 className="text-5xl lg:text-6xl font-medium text-gray-900 mb-6 leading-tight tracking-tight">
            Welcome to
            <span className="text-gray-900 font-semibold block">
              Payverge
            </span>
          </h1>
          
          <p className="text-xl font-light text-gray-600 leading-relaxed tracking-wide max-w-2xl mx-auto mb-4">
            <strong className="font-medium text-gray-900">{businessName}</strong> is now registered on the blockchain and ready to accept crypto payments!
          </p>

          {transactionHash && (
            <p className="text-sm text-gray-500 font-mono bg-gray-50 px-4 py-2 rounded-lg inline-block">
              Transaction: {transactionHash.slice(0, 10)}...{transactionHash.slice(-8)}
            </p>
          )}
        </div>

        {/* What You Can Do Section */}
        <Card className="shadow-xl border-0 bg-white/80 backdrop-blur-sm mb-12">
          <CardBody className="p-12">
            <div className="text-center mb-12">
              <div className="w-16 h-16 bg-gray-50 border-2 border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6">
                <Sparkles className="text-gray-700" size={32} />
              </div>
              <h2 className="text-3xl font-medium text-gray-900 mb-4 tracking-tight">What You Can Do Now</h2>
              <p className="text-lg font-light text-gray-600 leading-relaxed tracking-wide max-w-2xl mx-auto">
                Your business is ready to revolutionize how you accept payments
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-8 mb-12">
              {features.map((feature, index) => {
                const Icon = feature.icon;
                return (
                  <div key={index} className="group">
                    <div className="bg-gray-50 p-8 rounded-2xl border-2 border-gray-100 hover:border-gray-200 transition-all duration-300 hover:shadow-md h-full">
                      <div className="w-12 h-12 bg-gray-100 rounded-xl flex items-center justify-center mb-6 group-hover:bg-gray-200 transition-colors">
                        <Icon size={24} className="text-gray-700" />
                      </div>
                      <h3 className="text-xl font-semibold text-gray-900 mb-3">{feature.title}</h3>
                      <p className="text-gray-600 font-light leading-relaxed">{feature.description}</p>
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Next Steps */}
            <div className="bg-gradient-to-r from-gray-50 to-gray-100 p-8 rounded-2xl border border-gray-200">
              <h3 className="text-2xl font-semibold text-gray-900 mb-6 text-center">Ready to Get Started?</h3>
              <div className="flex flex-col sm:flex-row gap-4 justify-center items-center">
                <Button
                  size="lg"
                  onPress={handleGoToDashboard}
                  isLoading={isNavigating}
                  startContent={!isNavigating && <Building2 size={20} />}
                  className="h-14 px-8 text-base font-semibold bg-gray-900 hover:bg-gray-800 text-white shadow-lg hover:shadow-xl transition-all duration-300"
                >
                  {isNavigating ? 'Opening Dashboard...' : 'Go to Dashboard'}
                </Button>
                
                <Link href="/how-it-works">
                  <Button
                    variant="bordered"
                    size="lg"
                    endContent={<ArrowRight size={18} />}
                    className="h-14 px-8 text-base font-medium border-2 border-gray-300 hover:border-gray-400 hover:bg-gray-50 transition-all duration-300"
                  >
                    Learn More
                  </Button>
                </Link>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Footer Message */}
        <div className="text-center">
          <p className="text-gray-500 font-light">
            Need help getting started? Check out our{' '}
            <Link href="/how-it-works" className="text-gray-700 hover:text-gray-900 font-medium underline">
              documentation
            </Link>{' '}
            or contact support.
          </p>
        </div>
      </div>
    </div>
  );
}
