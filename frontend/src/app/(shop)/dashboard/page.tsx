"use client";
import { useState, useEffect, useCallback } from "react";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { useAuth } from "@/hooks/useAuth";
import { getCookie } from "@/config/aws-s3/cookie-management/store.helpers";
import { isTokenValid, decodeJwt } from "@/utils/jwt";
import { getUserProfile } from "@/api/users/profile";
import { Business, getMyBusinesses } from "@/api/business";
import { UserInterface as User } from "@/interface/users/users-interface";
import { Copy, Check } from "lucide-react";
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export default function Dashboard() {
  const { isConnected } = useAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `dashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [authChecked, setAuthChecked] = useState(false);
  const [copiedAddresses, setCopiedAddresses] = useState<Record<string, boolean>>({});

  const retryLoadUser = async () => {
    (window as any).__dashboardLoadStart = Date.now();
    setError(null);
    setLoading(true);
    setAuthChecked(false);
    
    // Try to fetch user data directly
    await fetchUserDataDirectly();
  };

  const loadBusinesses = useCallback(async () => {
    try {
      setLoading(true);
      const data = await getMyBusinesses();
      setBusinesses(data);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchUserDataDirectly = useCallback(async () => {
    try {
      const token = getCookie("session_token");
      
      if (!token || !isTokenValid(token)) {
        setError(tString('errors.signIn'));
        setLoading(false);
        setAuthChecked(true);
        return;
      }

      // Get address from token
      const tokenData = decodeJwt(token);
      
      if (!tokenData.address) {
        setError(tString('errors.invalidToken'));
        setLoading(false);
        setAuthChecked(true);
        return;
      }

      // Ensure address is lowercase to match backend format
      const address = tokenData.address.toLowerCase();
      
      // Fetch user profile directly
      const userData = await getUserProfile(address);
      
      if (userData) {
        // Set user in store
        userData.role = tokenData.role || 'user';
        useUserStore.getState().setUser(userData);
        
        setError(null);
        setAuthChecked(true);
        loadBusinesses();
      } else {
        // Fallback: If API returns null, create a temporary user profile from token
        const tempUser: User = {
          username: "",
          email: "",
          address: address,
          role: tokenData.role || 'user',
          joined_at: new Date().toISOString(),
          language_selected: "en",
          referral_code: "",
          referrer: "",
          referees: {},
          notifications: [],
        };
        useUserStore.getState().setUser(tempUser);
        setError(null);
        setAuthChecked(true);
        loadBusinesses();
      }
    } catch (error: any) {
      setError(`Failed to load user data: ${error}`);
      setLoading(false);
      setAuthChecked(true);
    }
  }, [loadBusinesses]);



  // Copy address function
  const copyToClipboard = async (text: string, addressType: string, businessId: number) => {
    try {
      await navigator.clipboard.writeText(text);
      const key = `${businessId}-${addressType}`;
      setCopiedAddresses(prev => ({ ...prev, [key]: true }));
      
      // Reset the copied state after 2 seconds
      setTimeout(() => {
        setCopiedAddresses(prev => ({ ...prev, [key]: false }));
      }, 2000);
    } catch (err) {
      console.error('Failed to copy text: ', err);
    }
  };

  // Simplified authentication check
  useEffect(() => {
    // Mark when we started loading
    if (!(window as any).__dashboardLoadStart) {
      (window as any).__dashboardLoadStart = Date.now();
    }
    
    // If we have a valid token but no user, try direct fetch immediately
    const token = getCookie("session_token");
    if (isConnected && token && isTokenValid(token) && !user) {
      fetchUserDataDirectly();
      return;
    }
    
    const timer = setTimeout(async () => {

      if (!isConnected) {
        setError(tString('errors.connectWallet'));
        setLoading(false);
        setAuthChecked(true);
        return;
      }

      // Check if we have a valid session token
      const token = getCookie("session_token");
      if (!token || !isTokenValid(token)) {
        setError(tString('errors.signIn'));
        setLoading(false);
        setAuthChecked(true);
        return;
      }

      // If we have a valid token but no user yet, try to fetch directly
      if (!user) {
        // Check if this is the first time or if we've been waiting too long
        const waitTime = Date.now() - (window as any).__dashboardLoadStart || 0;
        if (waitTime > 5000) { // 5 seconds timeout, then try direct fetch
          await fetchUserDataDirectly();
          return;
        }
        
        setError(tString('errors.loadingUserData'));
        setLoading(true);
        setAuthChecked(false);
        return;
      }

      // All good, load businesses
      setError(null);
      setAuthChecked(true);
      loadBusinesses();
    }, 500); // Give UserProvider 500ms to initialize

    return () => clearTimeout(timer);
  }, [isConnected, user, fetchUserDataDirectly]); // eslint-disable-line react-hooks/exhaustive-deps

  // Reload when user changes
  useEffect(() => {
    if (user && authChecked && !error) {
      loadBusinesses();
    }
  }, [user?.address, authChecked, error]); // eslint-disable-line react-hooks/exhaustive-deps



  if (loading) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center">
            <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
              <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
            </div>
            <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
            
            {/* Action buttons */}
            <div className="mt-6 space-y-3">
              {error === tString('errors.loadingUserData') && (
                <button
                  onClick={retryLoadUser}
                  className="px-4 py-2 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
                >
                  {tString('authentication.retryLoadingUser')}
                </button>
              )}
              
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Show authentication error
  if (error && (error.includes("sign in") || error.includes("connect your wallet"))) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center max-w-md mx-auto">
            <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
              <svg className="w-8 h-8 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
            </div>
            <h2 className="text-2xl font-light text-gray-900 mb-4 tracking-wide">{tString('authentication.title')}</h2>
            <p className="text-gray-600 font-light tracking-wide mb-8">{error}</p>
            
            {!isConnected ? (
              <div className="space-y-4">
                <p className="text-sm text-gray-500">{tString('authentication.connectMessage')}</p>
                <w3m-button />
              </div>
            ) : (
              <div className="space-y-4">
                <p className="text-sm text-gray-500">{tString('authentication.signMessage')}</p>
                <button
                  onClick={() => window.location.reload()}
                  className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
                >
                  {tString('authentication.tryAgain')}
                </button>
              </div>
            )}
            
            <div className="mt-8">
              <a 
                href="/"
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← {tString('authentication.backToHome')}
              </a>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      <div className="relative z-10 max-w-6xl mx-auto px-6 py-16">
        {/* Header */}
        <section className="mb-16">
          <div className="text-center mb-8">
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-6 hover:bg-gray-100 transition-colors duration-200">
              <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
              {tString('badge')}
            </div>
            
            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              {tString('title')}
            </h1>
            <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl mx-auto mb-8">
              {tString('subtitle')}
            </p>
            
            {businesses.length === 0 && (
              <Link
                href="/business/register"
                className="inline-block bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
              >
                {tString('businesses.create')}
              </Link>
            )}
          </div>
        </section>

        {/* Error Message */}
        {error && (
          <section className="mb-8">
            <div className="bg-red-50 border border-red-200 rounded-2xl p-6 shadow-sm">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 bg-red-100 rounded-xl flex items-center justify-center">
                  <svg className="h-6 w-6 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                  </svg>
                </div>
                <div>
                  <h3 className="text-lg font-medium text-red-900 tracking-wide">{tString('errors.error')}</h3>
                  <p className="text-red-700 font-light">{error}</p>
                </div>
              </div>
            </div>
          </section>
        )}

        {/* Businesses Grid */}
        <section>
          {businesses.length === 0 ? (
            <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
              <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
                <svg className="h-10 w-10 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                </svg>
              </div>
              <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">{tString('businesses.noneTitle')}</h3>
              <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
                {tString('businesses.noneDescription')}
              </p>
              <Link
                href="/business/register"
                className="inline-block bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
              >
                {tString('businesses.createFirst')}
              </Link>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
              {businesses.map((business) => (
                <div key={business.id} className="group bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1">
                  <div className="flex items-center justify-between mb-6">
                    <h3 className="text-xl font-light text-gray-900 tracking-wide">{business.name}</h3>
                    <div className={`px-3 py-1 rounded-xl text-xs font-medium tracking-wide ${
                      business.is_active 
                        ? 'bg-green-100 text-green-700 border border-green-200' 
                        : 'bg-gray-100 text-gray-700 border border-gray-200'
                    }`}>
                      {business.is_active ? tString('businesses.active') : tString('businesses.inactive')}
                    </div>
                  </div>
                  
                  <div className="space-y-4 mb-8">
                    <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-medium text-gray-900 tracking-wide">{tString('businesses.settlementAddress')}</span>
                        <button
                          onClick={() => copyToClipboard(business.settlement_address, 'settlement', business.id)}
                          className="p-1 hover:bg-gray-200 rounded-md transition-colors duration-200 group"
                          title={tString('businesses.copySettlement')}
                        >
                          {copiedAddresses[`${business.id}-settlement`] ? (
                            <Check className="w-4 h-4 text-green-600" />
                          ) : (
                            <Copy className="w-4 h-4 text-gray-500 group-hover:text-gray-700" />
                          )}
                        </button>
                      </div>
                      <span className="font-mono text-xs text-gray-600">{business.settlement_address.slice(0, 10)}...{business.settlement_address.slice(-8)}</span>
                    </div>
                    <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-medium text-gray-900 tracking-wide">{tString('businesses.tippingAddress')}</span>
                        <button
                          onClick={() => copyToClipboard(business.tipping_address, 'tipping', business.id)}
                          className="p-1 hover:bg-gray-200 rounded-md transition-colors duration-200 group"
                          title={tString('businesses.copyTipping')}
                        >
                          {copiedAddresses[`${business.id}-tipping`] ? (
                            <Check className="w-4 h-4 text-green-600" />
                          ) : (
                            <Copy className="w-4 h-4 text-gray-500 group-hover:text-gray-700" />
                          )}
                        </button>
                      </div>
                      <span className="font-mono text-xs text-gray-600">{business.tipping_address.slice(0, 10)}...{business.tipping_address.slice(-8)}</span>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <span className="text-sm font-medium text-gray-900 tracking-wide block mb-1">{tString('businesses.taxRate')}</span>
                        <span className="text-gray-600 font-light">{business.tax_rate}%</span>
                      </div>
                      <div>
                        <span className="text-sm font-medium text-gray-900 tracking-wide block mb-1">{tString('businesses.serviceFee')}</span>
                        <span className="text-gray-600 font-light">{business.service_fee_rate}%</span>
                      </div>
                    </div>
                  </div>

                  <div className="flex gap-3">
                    <a 
                      href={`/business/${business.id}/dashboard`}
                      className="flex-1 bg-gray-900 text-white px-6 py-3 rounded-xl text-sm font-medium hover:bg-gray-800 transition-all duration-200 text-center tracking-wide group-hover:shadow-lg"
                    >
                      {tString('businesses.manage')}
                    </a>
                </div>
              </div>
            ))}
          </div>
        )}
        </section>

      </div>
    </div>
  );
}
