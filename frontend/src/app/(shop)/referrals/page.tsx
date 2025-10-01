'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAccount } from 'wagmi';
import { useAppKit } from '@reown/appkit/react';
import { TypewriterText } from '@/components/ui/TypewriterText';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { getReferralStats } from '@/api/referrals';
import { 
  useTotalReferrers,
  useReferrer,
  useBasicReferrerFee,
  usePremiumReferrerFee,
  formatUsdcAmount
} from '@/contracts/hooks';

export default function ReferralsPage() {
  const [stats, setStats] = useState<any>(null);
  const [isLoading, setIsLoading] = useState(true);
  const router = useRouter();
  const { address, isConnected } = useAccount();
  const { open } = useAppKit();

  const dreamWords = [
    "effortlessly",
    "profitably", 
    "globally",
    "sustainably",
    "automatically"
  ];

  // Blockchain hooks
  const { data: totalReferrers } = useTotalReferrers();
  const { data: userReferrer } = useReferrer(address || '0x0');
  const { data: basicFee } = useBasicReferrerFee();
  const { data: premiumFee } = usePremiumReferrerFee();

  useEffect(() => {
    const fetchStats = async () => {
      try {
        const data = await getReferralStats();
        setStats(data);
      } catch (error) {
        console.error('Failed to fetch referral stats:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchStats();
  }, []);

  return (
    <>
      <style jsx>{`
        @keyframes fade-in {
          from { opacity: 0; transform: translateY(20px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .animate-fade-in { animation: fade-in 0.8s ease-out; }
      `}</style>
      
      <div className="min-h-screen bg-white">
        {/* Hero Section */}
        <section className="py-20 lg:py-32 bg-gradient-to-b from-gray-50 to-white relative overflow-hidden">
          {/* Clean, minimal background */}
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
            <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
          </div>

          <div className="container mx-auto px-6 relative">
            <div className="max-w-4xl mx-auto text-center animate-fade-in">
              <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 hover:bg-white hover:shadow-sm transition-all duration-300">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse"></div>
                Earn with every referral 💰
              </div>

              <h1 className="text-5xl lg:text-7xl font-medium text-gray-900 mb-8 leading-tight tracking-tight">
                Build your income,<br />
                <TypewriterText
                  words={dreamWords}
                  className="bg-gradient-to-r from-blue-600 via-purple-600 to-indigo-600 bg-clip-text text-transparent font-semibold italic"
                />
              </h1>
              
              <p className="text-lg font-light leading-relaxed max-w-2xl mx-auto text-gray-600 mb-12 tracking-wide">
                Join Payverge&apos;s referral program and earn commission for every restaurant you bring to our platform. 
                Help businesses go borderless while building your passive income stream.
              </p>

              <div className="flex flex-col sm:flex-row gap-4 justify-center mb-16">
                {!isConnected ? (
                  <Button
                    onClick={() => open()}
                    className="px-8 py-4 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800 hover:shadow-lg transform hover:scale-105 transition-all duration-300"
                  >
                    Connect Wallet to Start
                  </Button>
                ) : userReferrer ? (
                  <>
                    <Button
                      onClick={() => router.push('/referrals/dashboard')}
                      className="px-8 py-4 bg-green-600 text-white font-medium rounded-xl hover:bg-green-700 hover:shadow-lg transform hover:scale-105 transition-all duration-300"
                    >
                      View My Dashboard
                    </Button>
                    <Button
                      onClick={() => router.push('/referrals/buy')}
                      variant="secondary"
                      className="px-8 py-4 border-2 border-gray-300 text-gray-700 font-medium rounded-xl hover:border-gray-500 hover:text-gray-900 transition-all duration-300"
                    >
                      Upgrade Tier
                    </Button>
                  </>
                ) : (
                  <>
                    <Button
                      onClick={() => router.push('/referrals/buy')}
                      className="px-8 py-4 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800 hover:shadow-lg transform hover:scale-105 transition-all duration-300"
                    >
                      Start Earning Today
                    </Button>
                    <Button
                      onClick={() => router.push('/referrals/dashboard')}
                      variant="secondary"
                      className="px-8 py-4 border-2 border-gray-300 text-gray-700 font-medium rounded-xl hover:border-gray-500 hover:text-gray-900 transition-all duration-300"
                    >
                      Check My Account
                    </Button>
                  </>
                )}
              </div>

              {/* Stats */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-6 max-w-2xl mx-auto">
                <div className="text-center">
                  <div className="text-2xl font-bold text-gray-900">
                    {totalReferrers ? totalReferrers.toString() : stats?.total_referrers || '0'}
                  </div>
                  <div className="text-sm text-gray-600">Active Referrers</div>
                </div>
                <div className="text-center">
                  <div className="text-2xl font-bold text-gray-900">
                    {stats?.total_referrals || '0'}
                  </div>
                  <div className="text-sm text-gray-600">Businesses Referred</div>
                </div>
                <div className="text-center">
                  <div className="text-2xl font-bold text-gray-900">
                    ${stats ? (parseFloat(stats.total_commissions || '0') / 1000000).toFixed(0) : '0'}
                  </div>
                  <div className="text-sm text-gray-600">Total Paid Out</div>
                </div>
                <div className="text-center">
                  <div className="text-2xl font-bold text-gray-900">15%</div>
                  <div className="text-sm text-gray-600">Max Commission</div>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* How It Works */}
        <section className="py-20 bg-white">
          <div className="container mx-auto px-6">
            <div className="max-w-6xl mx-auto">
              <div className="text-center mb-16">
                <h2 className="text-4xl lg:text-5xl font-medium text-gray-900 mb-6 tracking-tight">How It Works</h2>
                <p className="text-lg font-light text-gray-600 max-w-2xl mx-auto">Simple steps to start earning with Payverge referrals</p>
              </div>

              <div className="grid md:grid-cols-3 gap-12">
                <div className="text-center group">
                  <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-100 transition-colors">
                    <span className="text-2xl font-semibold text-gray-700">1</span>
                  </div>
                  <h3 className="text-xl font-semibold text-gray-900 mb-4">Choose Your Tier</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Select Basic (10% commission) or Premium (15% commission) referrer status</p>
                </div>

                <div className="text-center group">
                  <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-100 transition-colors">
                    <span className="text-2xl font-semibold text-gray-700">2</span>
                  </div>
                  <h3 className="text-xl font-semibold text-gray-900 mb-4">Share Your Code</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Get your unique referral code and share it with restaurant owners</p>
                </div>

                <div className="text-center group">
                  <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-100 transition-colors">
                    <span className="text-2xl font-semibold text-gray-700">3</span>
                  </div>
                  <h3 className="text-xl font-semibold text-gray-900 mb-4">Earn Commissions</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Earn USDC commissions automatically when businesses register with your code</p>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Pricing Tiers */}
        <section className="py-20 bg-gray-50">
          <div className="container mx-auto px-6">
            <div className="max-w-6xl mx-auto">
              <div className="text-center mb-16">
                <h2 className="text-4xl lg:text-5xl font-medium text-gray-900 mb-6 tracking-tight">Choose Your Referrer Tier</h2>
                <p className="text-lg font-light text-gray-600 max-w-2xl mx-auto">Start earning with the tier that fits your goals</p>
              </div>

              <div className="grid md:grid-cols-2 gap-8 max-w-4xl mx-auto">
                {/* Basic Tier */}
                <div className="bg-white rounded-3xl p-8 border border-gray-200 hover:shadow-lg transition-all duration-300 group">
                  <div className="text-center mb-8">
                    <h3 className="text-2xl font-semibold text-gray-900 mb-3">Basic Referrer</h3>
                    <div className="text-5xl font-medium text-gray-900 mb-2">$10</div>
                    <p className="text-gray-500 font-light">One-time registration fee</p>
                  </div>
                  
                  <div className="space-y-4 mb-8">
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">10% commission on referrals</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">10% discount for businesses</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">Custom referral code</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">Real-time dashboard</span>
                    </div>
                  </div>
                  
                  <Button 
                    onClick={() => router.push('/referrals/buy?tier=basic')}
                    className="w-full py-3 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800 transition-colors"
                  >
                    Get Started - Basic
                  </Button>
                </div>

                {/* Premium Tier */}
                <div className="bg-white rounded-3xl p-8 border-2 border-gray-900 hover:shadow-lg transition-all duration-300 group relative">
                  <div className="absolute -top-3 left-1/2 transform -translate-x-1/2">
                    <Badge className="bg-gray-900 text-white px-4 py-1 text-sm font-medium rounded-full">
                      RECOMMENDED
                    </Badge>
                  </div>
                  
                  <div className="text-center mb-8">
                    <h3 className="text-2xl font-semibold text-gray-900 mb-3">Premium Referrer</h3>
                    <div className="text-5xl font-medium text-gray-900 mb-2">$25</div>
                    <p className="text-gray-500 font-light">One-time registration fee</p>
                  </div>
                  
                  <div className="space-y-4 mb-8">
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">15% commission on referrals</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">15% discount for businesses</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">Custom referral code</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">Priority support</span>
                    </div>
                    <div className="flex items-center">
                      <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-3"></div>
                      <span className="text-gray-700 font-light">Advanced analytics</span>
                    </div>
                  </div>
                  
                  <Button 
                    onClick={() => router.push('/referrals/buy?tier=premium')}
                    className="w-full py-3 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800 transition-colors"
                  >
                    Get Started - Premium
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Benefits */}
        <section className="py-20 bg-white">
          <div className="container mx-auto px-6">
            <div className="max-w-6xl mx-auto">
              <div className="text-center mb-16">
                <h2 className="text-4xl lg:text-5xl font-medium text-gray-900 mb-6 tracking-tight">Why Choose Payverge Referrals?</h2>
                <p className="text-lg font-light text-gray-600 max-w-2xl mx-auto">The benefits that make us different</p>
              </div>

              <div className="grid md:grid-cols-2 gap-16">
                <div>
                  <div className="space-y-10">
                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">💰</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Instant USDC Payments</h3>
                        <p className="text-gray-600 font-light leading-relaxed">Get paid instantly in USDC when businesses register. No waiting, no delays.</p>
                      </div>
                    </div>

                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">🌍</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Global Reach</h3>
                        <p className="text-gray-600 font-light leading-relaxed">Refer businesses from anywhere in the world. No geographic restrictions.</p>
                      </div>
                    </div>

                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">📊</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Real-time Tracking</h3>
                        <p className="text-gray-600 font-light leading-relaxed">Monitor your referrals and earnings in real-time with our comprehensive dashboard.</p>
                      </div>
                    </div>
                  </div>
                </div>

                <div>
                  <div className="space-y-10">
                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">🔒</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Blockchain Security</h3>
                        <p className="text-gray-600 font-light leading-relaxed">All transactions are secured by blockchain technology. Transparent and immutable.</p>
                      </div>
                    </div>

                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">🚀</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Growing Market</h3>
                        <p className="text-gray-600 font-light leading-relaxed">Join the future of payments. The crypto payments market is exploding.</p>
                      </div>
                    </div>

                    <div className="flex group">
                      <div className="flex-shrink-0 w-14 h-14 bg-gray-50 rounded-2xl flex items-center justify-center mr-6 group-hover:bg-gray-100 transition-colors">
                        <span className="text-2xl">🤝</span>
                      </div>
                      <div>
                        <h3 className="text-xl font-semibold text-gray-900 mb-3">Win-Win Model</h3>
                        <p className="text-gray-600 font-light leading-relaxed">Businesses save money with discounts while you earn commissions. Everyone wins.</p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* CTA Section */}
        <section className="py-20 bg-gray-50">
          <div className="container mx-auto px-6">
            <div className="max-w-4xl mx-auto text-center">
              <h2 className="text-4xl lg:text-5xl font-medium text-gray-900 mb-6 tracking-tight">Ready to Start Earning?</h2>
              <p className="text-lg font-light text-gray-600 mb-12 max-w-2xl mx-auto leading-relaxed">
                Join hundreds of referrers already earning with Payverge. 
                Start building your passive income stream today.
              </p>
              
              <div className="flex flex-col sm:flex-row gap-4 justify-center">
                <Button
                  onClick={() => router.push('/referrals/buy')}
                  className="px-8 py-4 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800 transform hover:scale-105 transition-all duration-300"
                >
                  Become a Referrer
                </Button>
                <Button
                  onClick={() => router.push('/referrals/dashboard')}
                  variant="secondary"
                  className="px-8 py-4 border-2 border-gray-300 text-gray-700 font-medium rounded-xl hover:border-gray-500 hover:text-gray-900 transition-all duration-300"
                >
                  Access Dashboard
                </Button>
              </div>
            </div>
          </div>
        </section>

        {/* Footer */}
        <footer className="bg-gray-900 text-white py-16">
          <div className="container mx-auto px-6">
            <div className="max-w-6xl mx-auto">
              <div className="grid grid-cols-1 md:grid-cols-4 gap-8 mb-12">
                <div className="col-span-1 md:col-span-2">
                  <h3 className="text-2xl font-semibold mb-4">Payverge</h3>
                  <p className="text-gray-400 font-light leading-relaxed max-w-md">
                    Building the future of borderless payments for hospitality. 
                    Instant, fair, and effortless crypto payments.
                  </p>
                </div>

                <div>
                  <h4 className="font-semibold mb-4">Product</h4>
                  <div className="space-y-3">
                    <a href="/referrals" className="text-gray-400 hover:text-white transition-colors block font-light">Referrals</a>
                    <a href="/referrals/dashboard" className="text-gray-400 hover:text-white transition-colors block font-light">Dashboard</a>
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">Features</a>
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">Pricing</a>
                  </div>
                </div>

                <div>
                  <h4 className="font-semibold mb-4">Company</h4>
                  <div className="space-y-3">
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">About</a>
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">Contact</a>
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">Privacy</a>
                    <a href="#" className="text-gray-400 hover:text-white transition-colors block font-light">Terms</a>
                  </div>
                </div>
              </div>

              <div className="text-center pt-8 border-t border-gray-800">
                <p className="text-sm text-gray-500 font-light">
                  &ldquo;Payverge operates exclusively on USDC. No banks. No borders. Just better payments.&rdquo;
                </p>
              </div>
            </div>
          </div>
        </footer>
      </div>
    </>
  );
}
