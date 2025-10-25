'use client';

import React, { useState, useEffect } from 'react';
import { Button } from '@nextui-org/react';
import { Building2, MapPin, CreditCard, Percent, DollarSign, Users, ArrowRight, Sparkles, BarChart3, QrCode, Wallet, Receipt } from 'lucide-react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Business } from '@/api/business';

interface BusinessOverviewProps {
  business: Business;
  onNavigateToTab?: (tab: string) => void;
}

export default function BusinessOverview({ business, onNavigateToTab }: BusinessOverviewProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.overview.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  return (
    <div className="min-h-screen bg-white">
      {/* Clean, minimal background matching landing page */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
      </div>

      {/* Hero Section */}
      <section className="py-12 sm:py-16 lg:py-24 relative">
        <div className="container mx-auto px-4 sm:px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            <div className="inline-flex items-center px-3 sm:px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-xs sm:text-sm font-medium text-gray-700 mb-6 sm:mb-8 hover:bg-white hover:shadow-sm transition-all duration-300">
              <Building2 className="w-3 sm:w-4 h-3 sm:h-4 mr-2 text-blue-600" />
              {tString('welcome.badge')}
            </div>
            
            <h1 className="text-3xl sm:text-4xl md:text-5xl lg:text-7xl font-medium text-gray-900 mb-6 sm:mb-8 leading-tight tracking-tight">
              {tString('welcome.title')} <br />
              <span className="bg-gradient-to-r from-blue-600 via-blue-500 to-indigo-600 bg-clip-text text-transparent font-semibold italic">
                {business.name}
              </span>
            </h1>
            
            <p className="text-base sm:text-lg font-light leading-relaxed max-w-2xl mx-auto text-gray-600 mb-8 sm:mb-12 tracking-wide px-4 sm:px-0">
              {tString('welcome.subtitle')}
            </p>
          </div>
        </div>
      </section>

      {/* Stats Section */}
      <section className="pb-12 sm:pb-16 lg:pb-20">
        <div className="container mx-auto px-4 sm:px-6">
          <div className="max-w-4xl mx-auto text-center mb-8 sm:mb-12">
            <div className="inline-flex items-center px-3 sm:px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-xs sm:text-sm font-medium text-gray-700 mb-6 sm:mb-8 shadow-sm">
              <div className="w-2 h-2 bg-gray-400 rounded-full mr-2 animate-pulse" style={{ animationDelay: '0.5s' }}></div>
              {tString('welcome.todayOverview')}
            </div>
          </div>
          
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-6 lg:gap-8 max-w-5xl mx-auto mb-12 sm:mb-16">
            <div className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300">
              <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4 sm:mb-6 group-hover:bg-gray-100 transition-colors duration-300">
                <Receipt className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide">{tString('activeBills')}</h3>
              <p className="text-2xl sm:text-3xl font-medium text-gray-900">0</p>
            </div>
            
            <div className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300">
              <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4 sm:mb-6 group-hover:bg-gray-100 transition-colors duration-300">
                <Users className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide">{tString('tables')}</h3>
              <p className="text-2xl sm:text-3xl font-medium text-gray-900">0</p>
            </div>
            
            <div className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300 sm:col-span-2 md:col-span-1">
              <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4 sm:mb-6 group-hover:bg-gray-100 transition-colors duration-300">
                <DollarSign className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide">{tString('todaysRevenue')}</h3>
              <p className="text-2xl sm:text-3xl font-medium text-gray-900">$0.00</p>
            </div>
          </div>

          {/* Quick Actions Section - same container */}
          <div className="max-w-4xl mx-auto text-center mb-8 sm:mb-12">
            <div className="inline-flex items-center px-3 sm:px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-xs sm:text-sm font-medium text-gray-700 mb-6 sm:mb-8 shadow-sm">
              <Sparkles className="w-3 sm:w-4 h-3 sm:h-4 mr-2 text-gray-600" />
              {tString('quickActions.badge')}
            </div>
            
            <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-4 sm:mb-6 tracking-wide px-4 sm:px-0">
              {tString('quickActions.title')} <span className="italic font-light tracking-wider">{tString('quickActions.titleEmphasis')}</span> {tString('quickActions.titleSuffix')}
            </h2>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 sm:gap-6 lg:gap-8 max-w-5xl mx-auto">
            <div 
              className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300 cursor-pointer"
              onClick={() => onNavigateToTab?.('analytics')}
            >
              <div className="flex items-center justify-between mb-4">
                <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center group-hover:bg-gray-100 transition-colors duration-300">
                  <BarChart3 className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
                </div>
                <ArrowRight className="w-4 sm:w-5 h-4 sm:h-5 text-gray-400 group-hover:text-gray-600 group-hover:translate-x-1 transition-all duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide text-left">{tString('quickActions.viewAnalytics.title')}</h3>
              <p className="text-sm sm:text-base text-gray-600 font-light leading-relaxed text-left">{tString('quickActions.viewAnalytics.description')}</p>
            </div>

            <div 
              className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300 cursor-pointer"
              onClick={() => onNavigateToTab?.('menu')}
            >
              <div className="flex items-center justify-between mb-4">
                <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center group-hover:bg-gray-100 transition-colors duration-300">
                  <QrCode className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
                </div>
                <ArrowRight className="w-4 sm:w-5 h-4 sm:h-5 text-gray-400 group-hover:text-gray-600 group-hover:translate-x-1 transition-all duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide text-left">{tString('quickActions.manageMenu.title')}</h3>
              <p className="text-sm sm:text-base text-gray-600 font-light leading-relaxed text-left">{tString('quickActions.manageMenu.description')}</p>
            </div>

            <div 
              className="group bg-white p-6 sm:p-8 rounded-2xl shadow-sm border border-gray-200 hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-gray-300 cursor-pointer"
              onClick={() => onNavigateToTab?.('blockchain')}
            >
              <div className="flex items-center justify-between mb-4">
                <div className="w-10 sm:w-12 h-10 sm:h-12 bg-gray-50 rounded-full flex items-center justify-center group-hover:bg-gray-100 transition-colors duration-300">
                  <Wallet className="w-5 sm:w-6 h-5 sm:h-6 text-gray-600 group-hover:text-gray-700 transition-colors duration-300" />
                </div>
                <ArrowRight className="w-4 sm:w-5 h-4 sm:h-5 text-gray-400 group-hover:text-gray-600 group-hover:translate-x-1 transition-all duration-300" />
              </div>
              <h3 className="text-lg sm:text-xl font-light text-gray-900 mb-2 sm:mb-3 tracking-wide text-left">{tString('quickActions.claimEarnings.title')}</h3>
              <p className="text-sm sm:text-base text-gray-600 font-light leading-relaxed text-left">{tString('quickActions.claimEarnings.description')}</p>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
