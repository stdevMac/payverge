"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslation } from "@/i18n/useTranslation";
import { TypewriterText } from "@/components/ui/TypewriterText";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";

export default function Home() {
  const [email, setEmail] = useState("");
  const [businessName, setBusinessName] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showSuccessModal, setShowSuccessModal] = useState(false);
  const { t } = useTranslation();
  const router = useRouter();

  // Dynamic content for typewriter effect
  const businessTypes = [
    "Restaurants",
    "Coffee Shops", 
    "Hotels",
    "Bars",
    "Food Trucks",
    "Cafés",
    "Breweries",
    "Bistros"
  ];


  const handleWaitlistSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);

    try {
      const { subscribeToWaitlist } = await import('@/api/subscribers/getSubscribers');
      const result = await subscribeToWaitlist(email, businessName);

      if (result.success) {
        setEmail("");
        setBusinessName("");
        setShowSuccessModal(true);
      } else {
        alert(`Error: ${result.message}`);
      }
    } catch (error: any) {
      console.error('Subscription error:', error);
      alert("Something went wrong. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleShareOnX = () => {
    const tweetText = encodeURIComponent("Just signed up for updates from https://payverge.io - the future of crypto payments for hospitality! 🚀 Excited to see what they're building.");
    const tweetUrl = `https://twitter.com/intent/tweet?text=${tweetText}`;
    window.open(tweetUrl, '_blank');
  };

  return (
    <div className="min-h-screen bg-white">
      {/* Hero Section */}
      <section className="py-16 lg:py-24 bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-gray-50 to-gray-100 rounded-full blur-3xl opacity-20 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-gray-100 rounded-full blur-3xl opacity-15 animate-pulse" style={{ animationDelay: '2s' }}></div>
          
          {/* Floating crypto symbols */}
          <div className="absolute top-20 left-1/4 text-gray-200 text-4xl animate-bounce opacity-30" style={{ animationDelay: '1s', animationDuration: '3s' }}>₿</div>
          <div className="absolute bottom-32 right-1/4 text-gray-200 text-3xl animate-bounce opacity-25" style={{ animationDelay: '2s', animationDuration: '4s' }}>Ξ</div>
          <div className="absolute top-1/3 right-20 text-gray-200 text-2xl animate-bounce opacity-20" style={{ animationDelay: '0.5s', animationDuration: '3.5s' }}>$</div>
          
          {/* Subtle grid pattern */}
          <div className="absolute inset-0 opacity-[0.01]" style={{ backgroundImage: 'radial-gradient(circle at 1px 1px, rgba(0,0,0,0.15) 1px, transparent 0)', backgroundSize: '40px 40px' }}></div>
        </div>

        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            <Badge className="mb-8">Now live and ready to use</Badge>

            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              Crypto payments for <br />
              <TypewriterText 
                words={businessTypes}
                className="italic font-medium bg-gradient-to-r from-gray-800 via-blue-600 to-gray-900 bg-clip-text text-transparent tracking-wider"
              />
            </h1>
            <p className="text-lg font-light leading-relaxed max-w-2xl mx-auto text-gray-600 mb-12 tracking-wide">
              Accept USDC payments with instant settlement, lower fees, and zero chargebacks
            </p>
            
            <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-12">
              <Button size="large" onClick={() => router.push('/business/register')}>
                Start Your Business
              </Button>
              <Button variant="secondary" size="large" onClick={() => router.push('/dashboard')}>
                Access Dashboard
              </Button>
            </div>

            <div className="flex justify-center gap-6">
              <Button variant="ghost" onClick={() => document.getElementById('how-it-works')?.scrollIntoView({ behavior: 'smooth' })}>
                How it works
              </Button>
              <Button variant="ghost" onClick={() => document.getElementById('updates')?.scrollIntoView({ behavior: 'smooth' })}>
                Get Updates
              </Button>
            </div>
          </div>
        </div>
      </section>

      {/* The Problem Section */}
      <section className="py-16 lg:py-20 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge className="mb-8">The current state of payments</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Traditional payments are <span className="italic font-light tracking-wider">broken</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              Restaurants lose money to high fees, wait days for settlements, and deal with complex processes that frustrate customers
            </p>
          </div>

          <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto mb-12">
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="w-12 h-12 bg-red-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">High Fees</h3>
              <p className="text-gray-600 font-light leading-relaxed">3-5% processing fees plus monthly charges eat into thin margins</p>
            </div>
            
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="w-12 h-12 bg-red-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">Slow Settlement</h3>
              <p className="text-gray-600 font-light leading-relaxed">Wait 2-3 business days for payments to reach your account</p>
            </div>
            
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="w-12 h-12 bg-red-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">Chargeback Risk</h3>
              <p className="text-gray-600 font-light leading-relaxed">Disputes and reversals create uncertainty and additional costs</p>
            </div>
          </div>
        </div>
      </section>

      {/* The Solution Section */}
      <section className="py-16 lg:py-20 bg-white">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge variant="primary" className="mb-8">The Payverge solution</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Crypto payments <span className="italic font-light tracking-wider">done right</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              Instant USDC settlements, transparent fees, and zero chargebacks
            </p>
          </div>

          <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto mb-12">
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-green-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-green-600" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">Instant Settlement</h3>
              <p className="text-gray-600 font-light leading-relaxed">USDC hits your wallet immediately—no waiting for bank transfers</p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-blue-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-blue-600" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M4 4a2 2 0 00-2 2v4a2 2 0 002 2V6h10a2 2 0 00-2-2H4zm2 6a2 2 0 012-2h8a2 2 0 012 2v4a2 2 0 01-2 2H8a2 2 0 01-2-2v-4zm6 4a2 2 0 100-4 2 2 0 000 4z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">Lower Fees</h3>
              <p className="text-gray-600 font-light leading-relaxed">Just 2% per transaction with no hidden charges or monthly fees</p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-purple-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-purple-600" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M2.166 4.999A11.954 11.954 0 0010 1.944 11.954 11.954 0 0017.834 5c.11.65.166 1.32.166 2.001 0 5.225-3.34 9.67-8 11.317C5.34 16.67 2 12.225 2 7c0-.682.057-1.35.166-2.001zm11.541 3.708a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-3 tracking-wide">Zero Chargebacks</h3>
              <p className="text-gray-600 font-light leading-relaxed">Crypto payments are final—no disputes or payment reversals</p>
            </div>
          </div>

          <div className="text-center">
            <Button size="large" onClick={() => router.push('/business/register')}>
              Experience the difference
            </Button>
          </div>
        </div>
      </section>

      {/* How It Works Section */}
      <section id="how-it-works" className="py-16 lg:py-20 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge className="mb-8">How it works</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Simple crypto payments in <span className="italic font-light tracking-wider">three steps</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              From QR scan to instant settlement—the future of restaurant payments
            </p>
          </div>

          <div className="grid lg:grid-cols-3 gap-12 max-w-5xl mx-auto">
            <div className="text-center">
              <div className="w-16 h-16 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                <span className="text-xl font-medium text-white">1</span>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-4 tracking-wide">Scan & Order</h3>
              <p className="text-gray-600 font-light leading-relaxed">
                Guests scan your table QR code, browse the digital menu, and add items to their bill. No app downloads required.
              </p>
            </div>

            <div className="text-center">
              <div className="w-16 h-16 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                <span className="text-xl font-medium text-white">2</span>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-4 tracking-wide">Pay with USDC</h3>
              <p className="text-gray-600 font-light leading-relaxed">
                Secure blockchain payment in USDC (stable cryptocurrency). Split bills, add tips, and pay directly from any crypto wallet.
              </p>
            </div>

            <div className="text-center">
              <div className="w-16 h-16 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                <span className="text-xl font-medium text-white">3</span>
              </div>
              <h3 className="text-xl font-medium text-gray-900 mb-4 tracking-wide">Instant Settlement</h3>
              <p className="text-gray-600 font-light leading-relaxed">
                Funds arrive in your wallet immediately—no 2-3 day delays. Every transaction recorded on blockchain for perfect transparency.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Complete Platform Section */}
      <section className="py-16 lg:py-20 bg-white">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge variant="primary" className="mb-8">Complete platform</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Beyond payments—complete <span className="italic font-light tracking-wider">restaurant tech</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              Everything you need to run a modern restaurant, plus crypto payments
            </p>
          </div>

          <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-8 max-w-6xl mx-auto mb-12">
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Beautiful Digital Menus</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Automatically generated menus with photos, real-time pricing, and instant updates across all tables
              </p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Google Reviews Integration</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Automatically request reviews after successful payments to boost your online reputation
              </p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Real-time Analytics</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Track sales, crypto payment trends, and make data-driven decisions for your business
              </p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Staff Management</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Transparent crypto tip distribution and performance analytics for your team
              </p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 17h5l-5 5v-5zM4 19h6v-2H4v2zM4 15h8v-2H4v2zM4 11h8V9H4v2zM4 7h8V5H4v2z" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Order Management</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Kitchen display system and order tracking from menu to payment
              </p>
            </div>
            
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1" />
                </svg>
              </div>
              <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">USDC Loyalty Program</h3>
              <p className="text-gray-600 text-sm font-light leading-relaxed">
                Reward frequent customers with crypto cashback and exclusive offers
              </p>
            </div>
          </div>

          <div className="text-center">
            <p className="text-sm text-gray-500 mb-6 font-light tracking-wide">All features included • No hidden fees • 2% per transaction</p>
            <Button size="large" onClick={() => router.push('/business/register')}>
              Start Your Business
            </Button>
          </div>
        </div>
      </section>

      {/* Social Proof Section */}
      <section className="py-16 lg:py-20 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge className="mb-8">Trusted by hospitality</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Built for <span className="italic font-light tracking-wider">modern restaurants</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              From coffee shops to fine dining—crypto payments for every hospitality business
            </p>
          </div>

          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-8 max-w-4xl mx-auto mb-16">
            <div className="text-center">
              <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm">
                <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                </svg>
              </div>
              <h3 className="text-sm font-medium text-gray-900 tracking-wide">Restaurants</h3>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm">
                <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M10.5 3L12 2l1.5 1H21l-1 6H4l-1-6h7.5z" />
                </svg>
              </div>
              <h3 className="text-sm font-medium text-gray-900 tracking-wide">Coffee Shops</h3>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm">
                <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
                </svg>
              </div>
              <h3 className="text-sm font-medium text-gray-900 tracking-wide">Bars & Lounges</h3>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm">
                <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                </svg>
              </div>
              <h3 className="text-sm font-medium text-gray-900 tracking-wide">Hotels</h3>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm">
                <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 4V2a1 1 0 011-1h8a1 1 0 011 1v2m-9 0h10m-10 0a2 2 0 00-2 2v14a2 2 0 002 2h10a2 2 0 002-2V6a2 2 0 00-2-2M9 12l2 2 4-4" />
                </svg>
              </div>
              <h3 className="text-sm font-medium text-gray-900 tracking-wide">Event Venues</h3>
            </div>
          </div>

          <div className="grid md:grid-cols-4 gap-8 max-w-5xl mx-auto">
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="text-3xl lg:text-4xl font-light text-gray-900 mb-2 tracking-wide">Instant</div>
              <div className="text-sm text-gray-600 font-medium mb-1">Settlement</div>
              <div className="text-xs text-gray-500 font-light">USDC in your wallet immediately</div>
            </div>
            
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="text-3xl lg:text-4xl font-light text-gray-900 mb-2 tracking-wide">2%</div>
              <div className="text-sm text-gray-600 font-medium mb-1">Flat Fee</div>
              <div className="text-xs text-gray-500 font-light">No hidden charges</div>
            </div>
            
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="text-3xl lg:text-4xl font-light text-gray-900 mb-2 tracking-wide">0%</div>
              <div className="text-sm text-gray-600 font-medium mb-1">Chargebacks</div>
              <div className="text-xs text-gray-500 font-light">Crypto payments are final</div>
            </div>
            
            <div className="bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center">
              <div className="text-3xl lg:text-4xl font-light text-gray-900 mb-2 tracking-wide">24/7</div>
              <div className="text-sm text-gray-600 font-medium mb-1">Support</div>
              <div className="text-xs text-gray-500 font-light">Always here to help</div>
            </div>
          </div>
        </div>
      </section>

      {/* Get Started Section */}
      <section id="updates" className="py-16 lg:py-20 bg-white">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-16">
            <Badge variant="primary" className="mb-8">Ready to get started?</Badge>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Join the crypto payments <span className="italic font-light tracking-wider">revolution</span>
            </h2>
            <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
              Set up your restaurant in minutes and start accepting USDC payments today
            </p>
          </div>

          <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-16">
            <Button size="large" onClick={() => router.push('/business/register')}>
              Start Your Business
            </Button>
            <Button variant="secondary" size="large" onClick={() => router.push('/dashboard')}>
              Access Dashboard
            </Button>
          </div>

          <div className="max-w-2xl mx-auto">
            <div className="bg-gray-50 p-8 rounded-2xl text-center">
              <h3 className="text-xl font-medium text-gray-900 mb-4 tracking-wide">Stay updated on crypto payments</h3>
              <p className="text-gray-600 font-light mb-6 leading-relaxed">
                Get insights on crypto adoption in hospitality and new Payverge features
              </p>
              
              <form onSubmit={handleWaitlistSubmit} className="max-w-sm mx-auto space-y-4">
                <input
                  type="email"
                  placeholder="Enter your email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  className="w-full px-4 py-3 bg-white text-gray-900 placeholder-gray-500 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent font-light"
                />
                <input
                  type="text"
                  placeholder="Business name (optional)"
                  value={businessName}
                  onChange={(e) => setBusinessName(e.target.value)}
                  className="w-full px-4 py-3 bg-white text-gray-900 placeholder-gray-500 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent font-light"
                />
                <Button
                  type="submit"
                  disabled={isSubmitting}
                  className="w-full"
                >
                  {isSubmitting ? "Joining..." : "Get Updates"}
                </Button>
              </form>
            </div>
          </div>

          <div className="mt-16 text-center">
            <div className="grid md:grid-cols-3 gap-8 max-w-3xl mx-auto">
              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Built on USDC</h3>
                <p className="text-gray-600 font-light leading-relaxed">Trusted, stable cryptocurrency infrastructure</p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Hospitality-First</h3>
                <p className="text-gray-600 font-light leading-relaxed">Designed specifically for restaurants and hospitality</p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Simple Pricing</h3>
                <p className="text-gray-600 font-light leading-relaxed">Just 2% per transaction with no hidden fees</p>
              </div>
            </div>
          </div>
        </div>
      </section>


      {/* Success Modal */}
      {showSuccessModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-lg max-w-md w-full p-8 relative animate-in fade-in zoom-in duration-200">
            {/* Close button */}
            <button
              onClick={() => setShowSuccessModal(false)}
              className="absolute top-4 right-4 text-gray-400 hover:text-gray-600 transition-colors"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>

            {/* Success icon */}
            <div className="flex justify-center mb-6">
              <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center">
                <svg className="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
            </div>

            {/* Modal content */}
            <div className="text-center">
              <h3 className="text-xl font-semibold text-gray-900 mb-3">
                Welcome to the future! 🚀
              </h3>
              <p className="text-gray-600 mb-6 leading-relaxed">
                Thanks for subscribing to updates! We&apos;ll keep you informed about new features, improvements, and insights about crypto payments in hospitality.
              </p>

              {/* Share section */}
              <div className="border-t pt-6">
                <p className="text-sm text-gray-500 mb-4">
                  Help us spread the word!
                </p>
                <button
                  onClick={handleShareOnX}
                  className="inline-flex items-center gap-2 bg-black text-white px-6 py-3 rounded-lg hover:bg-gray-800 transition-colors font-medium"
                >
                  <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
                  </svg>
                  Share on X
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
