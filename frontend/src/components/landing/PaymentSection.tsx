'use client';

import { useRouter } from "next/navigation";
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Button } from "@/components/ui/Button";
import { useRegistrationFee, formatUsdcAmount } from '@/contracts/hooks';

export default function PaymentSection() {
  const router = useRouter();
  const { locale } = useSimpleLocale();

  // Smart contract data
  const { data: registrationFee } = useRegistrationFee();

  const t = (key: string) => {
    const result = getTranslation(`landing.${key}`, locale);
    return Array.isArray(result) ? result : result as string;
  };

  const tString = (key: string): string => {
    const result = getTranslation(`landing.${key}`, locale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  // Helper function to get subscription options based on registration fee
  const getSubscriptionOptions = () => {
    const yearlyFee = registrationFee && typeof registrationFee === 'bigint'
      ? Number(formatUsdcAmount(registrationFee))
      : 120; // Fallback value

    return [
      {
        months: 1,
        amount: (yearlyFee / 12).toFixed(2),
        description: tString('payment.options.1month') || '1 Month Access',
      },
      {
        months: 3,
        amount: (yearlyFee / 4).toFixed(2),
        description: tString('payment.options.3months') || '3 Months Access',
      },
      {
        months: 6,
        amount: (yearlyFee / 2).toFixed(2),
        description: tString('payment.options.6months') || '6 Months Access',
        popular: true,
      },
      {
        months: 12,
        amount: yearlyFee.toFixed(2),
        description: tString('payment.options.12months') || '1 Year Access',
      },
    ];
  };

  return (
    <section className="py-16 lg:py-20 bg-white">
      <div className="container mx-auto px-6">
        <div className="max-w-4xl mx-auto text-center mb-16">
          <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8">
            <div className="w-2 h-2 bg-blue-500 rounded-full mr-2 animate-pulse"></div>
            {t('payment.badge')}
          </div>
          <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
            {t('payment.title')}
          </h2>
          <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
            {t('payment.subtitle')}
          </p>
        </div>

        <div className="max-w-4xl mx-auto">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-12">
            {getSubscriptionOptions().map((option) => (
              <div
                key={option.months}
                className={`relative bg-white p-6 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-md transition-all duration-200 ${
                  option.popular ? 'ring-2 ring-gray-900' : ''
                }`}
              >
                {option.popular && (
                  <div className="absolute -top-2 left-1/2 transform -translate-x-1/2">
                    <span className="bg-gray-900 text-white px-3 py-1 rounded-full text-xs font-medium">
                      {tString('payment.popular')}
                    </span>
                  </div>
                )}
                <div className="text-2xl font-light text-gray-900 mb-2 tracking-wide">
                  ${option.amount}
                </div>
                <div className="text-sm text-gray-600 mb-3 font-light">
                  {option.description}
                </div>
              </div>
            ))}
          </div>

          <div className="text-center">
            <p className="text-gray-600 mb-8 font-light leading-relaxed max-w-2xl mx-auto">
              {t('payment.description')}
            </p>
            <Button
              size="large"
              onClick={() => router.push('/business/register')}
            >
              {tString('payment.cta')}
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}
