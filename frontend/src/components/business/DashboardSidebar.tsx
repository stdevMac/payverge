'use client';

import React from 'react';
import { Button } from '@nextui-org/react';
import { 
  BarChart3, 
  Menu, 
  Users, 
  Receipt, 
  Settings, 
  ChefHat, 
  Coffee, 
  UserCheck, 
  Wallet,
  CreditCard,
  X,
  Home,
  TrendingUp,
  QrCode,
  Utensils,
  FileText,
  Link
} from 'lucide-react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Business } from '@/api/business';

interface DashboardSidebarProps {
  business: Business;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
}

export default function DashboardSidebar({ 
  business, 
  activeTab, 
  setActiveTab, 
  sidebarOpen, 
  setSidebarOpen 
}: DashboardSidebarProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = React.useState(locale);
  
  React.useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const menuItems = [
    {
      key: 'overview',
      label: tString('tabs.overview'),
      icon: Home,
      description: tString('tabs.overviewDesc')
    },
    {
      key: 'analytics',
      label: tString('tabs.analytics'),
      icon: TrendingUp,
      description: tString('tabs.analyticsDesc')
    },
    {
      key: 'menu',
      label: tString('tabs.menu'),
      icon: Utensils,
      description: tString('tabs.menuDesc')
    },
    {
      key: 'tables',
      label: tString('tabs.tables'),
      icon: QrCode,
      description: tString('tabs.tablesDesc')
    },
    {
      key: 'bills',
      label: tString('tabs.bills'),
      icon: FileText,
      description: tString('tabs.billsDesc')
    },
    {
      key: 'staff',
      label: tString('tabs.staff'),
      icon: Users,
      description: tString('tabs.staffDesc')
    },
    {
      key: 'kitchen',
      label: tString('tabs.kitchen'),
      icon: ChefHat,
      description: tString('tabs.kitchenDesc')
    },
    {
      key: 'counter',
      label: tString('tabs.counter'),
      icon: UserCheck,
      description: tString('tabs.counterDesc')
    },
    {
      key: 'blockchain',
      label: tString('tabs.blockchain'),
      icon: Link,
      description: tString('tabs.blockchainDesc')
    },
    {
      key: 'subscriptions',
      label: tString('tabs.subscriptions'),
      icon: CreditCard,
      description: tString('tabs.subscriptionsDesc')
    },
    {
      key: 'settings',
      label: tString('tabs.settings'),
      icon: Settings,
      description: tString('tabs.settingsDesc')
    }
  ];

  // Add counter tab (always show for now since counter_enabled doesn't exist in Business interface)
  menuItems.splice(-1, 0, {
    key: 'counter',
    label: tString('tabs.counter'),
    icon: UserCheck,
    description: tString('tabs.counterDesc')
  });

  const handleTabClick = (tabKey: string) => {
    setActiveTab(tabKey);
    setSidebarOpen(false); // Close sidebar on mobile after selection
  };

  return (
    <>
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div 
          className="fixed inset-0 bg-black bg-opacity-50 z-40 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <div className={`
        fixed lg:static inset-y-0 left-0 z-50 w-80 bg-white border-r border-gray-200 
        transform transition-transform duration-300 ease-in-out lg:transform-none
        ${sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
      `}>
        <div className="flex flex-col h-full">
          {/* Header */}
          <div className="flex items-center justify-between p-6 border-b border-gray-200">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
                <span className="text-white font-bold text-lg">
                  {business?.name?.charAt(0)?.toUpperCase() || 'B'}
                </span>
              </div>
              <div>
                <h2 className="font-semibold text-gray-900 truncate max-w-[180px]">
                  {business?.name}
                </h2>
                <p className="text-sm text-gray-500">
                  {tString('dashboard.title')}
                </p>
              </div>
            </div>
            <Button
              isIconOnly
              variant="light"
              size="sm"
              className="lg:hidden"
              onPress={() => setSidebarOpen(false)}
            >
              <X className="w-4 h-4" />
            </Button>
          </div>

          {/* Navigation */}
          <nav className="flex-1 overflow-y-auto p-4">
            <div className="space-y-2">
              {menuItems.map((item) => {
                const Icon = item.icon;
                const isActive = activeTab === item.key;
                
                return (
                  <Button
                    key={item.key}
                    variant={isActive ? "flat" : "light"}
                    color={isActive ? "primary" : "default"}
                    className={`
                      w-full justify-start h-auto p-4 text-left
                      ${isActive 
                        ? 'bg-blue-50 text-blue-700 border border-blue-200' 
                        : 'text-gray-700 hover:bg-gray-50'
                      }
                    `}
                    onPress={() => handleTabClick(item.key)}
                  >
                    <div className="flex items-start gap-3 w-full">
                      <Icon className={`w-5 h-5 mt-0.5 ${isActive ? 'text-blue-600' : 'text-gray-500'}`} />
                      <div className="flex-1 min-w-0">
                        <div className={`font-medium ${isActive ? 'text-blue-700' : 'text-gray-900'}`}>
                          {item.label}
                        </div>
                        <div className={`text-xs mt-1 ${isActive ? 'text-blue-600' : 'text-gray-500'}`}>
                          {item.description}
                        </div>
                      </div>
                    </div>
                  </Button>
                );
              })}
            </div>
          </nav>

          {/* Footer */}
          <div className="p-4 border-t border-gray-200">
            <div className="text-xs text-gray-500 text-center">
              {tString('dashboard.version')} 1.0.0
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
