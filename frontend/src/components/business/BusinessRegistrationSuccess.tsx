"use client";
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Card, CardBody, Button } from '@nextui-org/react';
import { Check, ArrowRight, Building2, CreditCard, QrCode, BarChart3, Users, Sparkles } from 'lucide-react';
import Link from 'next/link';
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

interface BusinessRegistrationSuccessProps {
  businessName: string;
  businessId: string;
  transactionHash?: string;
  variant?: 'registration' | 'dashboard'; // New prop to handle different contexts
  onClose?: () => void; // Optional close handler for dashboard context
}

export default function BusinessRegistrationSuccess({ 
  businessName, 
  businessId, 
  transactionHash,
  variant = 'registration',
  onClose
}: BusinessRegistrationSuccessProps) {
  // URL for booking a call with founder
  const FOUNDER_CALL_URL = "https://calendar.app.google/Etej2zEcndeK8LgU7";
  
  const router = useRouter();
  const [isNavigating, setIsNavigating] = useState(false);
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper - same pattern as dashboard
  const tString = (key: string): string => {
    const fullKey = `businessRegistrationSuccess.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const handleGoToDashboard = () => {
    if (variant === 'dashboard' && onClose) {
      // If we're already in dashboard context, just close the welcome screen
      onClose();
    } else {
      // If we're in registration context, navigate to dashboard
      setIsNavigating(true);
      router.push(`/business/${businessId}/dashboard?welcome=true`);
    }
  };

  const features = [
    {
      icon: QrCode,
      title: tString("whatYouCanDo.features.qrMenus.title"),
      description: tString("whatYouCanDo.features.qrMenus.description")
    },
    {
      icon: CreditCard,
      title: tString("whatYouCanDo.features.cryptoPayments.title"),
      description: tString("whatYouCanDo.features.cryptoPayments.description")
    },
    {
      icon: BarChart3,
      title: tString("whatYouCanDo.features.analytics.title"),
      description: tString("whatYouCanDo.features.analytics.description")
    },
    {
      icon: Users,
      title: tString("whatYouCanDo.features.staffManagement.title"),
      description: tString("whatYouCanDo.features.staffManagement.description")
    }
  ];

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white relative overflow-hidden">
      {/* Clean, minimal background */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
      </div>

      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-8 sm:py-12 lg:py-16 relative">
        {/* Success Header */}
        <div className="text-center mb-20 lg:mb-24">
          <div className="flex flex-col items-center space-y-6">
            <div className="w-20 h-20 sm:w-24 sm:h-24 bg-green-50 border-2 border-green-200 rounded-full flex items-center justify-center shadow-lg">
              <Check className="text-green-600" size={40} />
            </div>
            
            <div className="inline-flex items-center px-4 py-2 bg-green-50 border border-green-200 rounded-full text-sm font-medium text-green-700 shadow-sm">
              <div className="w-2 h-2 bg-green-500 rounded-full mr-2"></div>
              {tString("header.badge")}
            </div>

            <div className="space-y-4">
              <h1 className="text-4xl sm:text-5xl lg:text-6xl font-medium text-gray-900 leading-tight tracking-tight">
                {tString("header.title")}
                <span className="text-gray-900 font-semibold block mt-2">
                  Payverge
                </span>
              </h1>
              
              <p className="text-lg sm:text-xl font-light text-gray-600 leading-relaxed max-w-3xl mx-auto px-4">
                <strong className="font-medium text-gray-900">{businessName}</strong> {tString("header.subtitle")}
              </p>

              {transactionHash && (
                <div className="mt-6">
                  <p className="text-sm text-gray-500 font-mono bg-gray-50 px-4 py-2 rounded-lg inline-block border">
                    {tString("header.transactionLabel")} {transactionHash.slice(0, 10)}...{transactionHash.slice(-8)}
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* What You Can Do Section */}
        <Card className="shadow-xl border-0 bg-white/80 backdrop-blur-sm mb-16 lg:mb-20">
          <CardBody className="p-8 sm:p-12 lg:p-16">
            <div className="text-center mb-16">
              <div className="flex flex-col items-center space-y-6">
                <div className="w-16 h-16 bg-gray-50 border-2 border-gray-200 rounded-2xl flex items-center justify-center shadow-sm">
                  <Sparkles className="text-gray-700" size={32} />
                </div>
                <div className="space-y-4">
                  <h2 className="text-2xl sm:text-3xl lg:text-4xl font-medium text-gray-900 tracking-tight">
                    {tString("whatYouCanDo.title")}
                  </h2>
                  <p className="text-lg font-light text-gray-600 leading-relaxed max-w-3xl mx-auto px-4">
                    {tString("whatYouCanDo.subtitle")}
                  </p>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6 lg:gap-8 mb-16">
              {features.map((feature, index) => {
                const Icon = feature.icon;
                return (
                  <div key={index} className="group">
                    <div className="bg-gray-50 p-6 sm:p-8 rounded-2xl border-2 border-gray-100 hover:border-gray-200 transition-all duration-300 hover:shadow-md h-full flex flex-col">
                      <div className="flex items-start space-x-4 mb-4">
                        <div className="w-12 h-12 bg-gray-100 rounded-xl flex items-center justify-center group-hover:bg-gray-200 transition-colors flex-shrink-0">
                          <Icon size={24} className="text-gray-700" />
                        </div>
                        <div className="flex-1">
                          <h3 className="text-lg sm:text-xl font-semibold text-gray-900 mb-2 leading-tight">{feature.title}</h3>
                          <p className="text-gray-600 font-light leading-relaxed text-sm sm:text-base">{feature.description}</p>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Next Steps */}
            <div className="bg-gradient-to-r from-gray-50 to-gray-100 p-8 sm:p-10 rounded-2xl border border-gray-200">
              <div className="text-center space-y-8">
                <h3 className="text-xl sm:text-2xl font-semibold text-gray-900">
                  {tString("nextSteps.title")}
                </h3>
                <div className="flex flex-col sm:flex-row gap-4 justify-center items-center max-w-2xl mx-auto">
                  <Button
                    size="lg"
                    onPress={handleGoToDashboard}
                    isLoading={isNavigating}
                    startContent={!isNavigating && <Building2 size={20} />}
                    className="h-14 px-8 text-base font-semibold bg-gray-900 hover:bg-gray-800 text-white shadow-lg hover:shadow-xl transition-all duration-300 w-full sm:w-auto"
                  >
                    {variant === 'dashboard' 
                      ? tString("nextSteps.buttons.startExploring") || "Start Exploring"
                      : isNavigating 
                        ? tString("nextSteps.buttons.openingDashboard") 
                        : tString("nextSteps.buttons.goToDashboard")
                    }
                  </Button>
                  
                  <Link href={FOUNDER_CALL_URL} target="_blank" rel="noopener noreferrer">
                    <Button
                      variant="bordered"
                      size="lg"
                      endContent={<ArrowRight size={18} />}
                      className="h-14 px-8 text-base font-medium border-2 border-gray-300 hover:border-gray-400 hover:bg-gray-50 transition-all duration-300 w-full sm:w-auto"
                    >
                      {tString("nextSteps.buttons.bookCall")}
                    </Button>
                  </Link>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Footer Message */}
        <div className="text-center max-w-2xl mx-auto">
          <p className="text-gray-500 font-light text-sm sm:text-base leading-relaxed px-4">
            {tString("footer.helpText")}{' '}
            <Link href="/how-it-works" className="text-gray-700 hover:text-gray-900 font-medium underline transition-colors">
              {tString("footer.documentation")}
            </Link>{' '}
            {tString("footer.or")} {tString("footer.contactSupport")}.
          </p>
        </div>
      </div>
    </div>
  );
}
