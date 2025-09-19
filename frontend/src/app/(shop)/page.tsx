"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslation } from "@/i18n/useTranslation";

export default function Home() {
  const [email, setEmail] = useState("");
  const [businessName, setBusinessName] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showSuccessModal, setShowSuccessModal] = useState(false);
  const { t } = useTranslation();
  const router = useRouter();

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
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>

        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            {/* Floating badge */}
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-8 hover:bg-gray-100 transition-colors duration-200">
              <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
              Now live and ready to use
            </div>

            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              Crypto payments for <br />
              <span className="italic font-light bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 bg-clip-text text-transparent tracking-wider">modern hospitality</span>
            </h1>
            <p className="text-lg lg:text-xl text-gray-600 mb-12 font-light leading-relaxed max-w-2xl mx-auto">
              Accept USDC payments and receive instant settlements with complete transparency
            </p>
            <div className="flex flex-col sm:flex-row gap-4 justify-center items-center">
              <button
                onClick={() => router.push('/business/register')}
                className="group bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 relative overflow-hidden tracking-wide"
              >
                <span className="relative z-10 tracking-wide">Start Your Business</span>
                <div className="absolute inset-0 bg-gradient-to-r from-gray-800 to-gray-900 opacity-0 group-hover:opacity-100 transition-opacity duration-200"></div>
              </button>
              <button
                onClick={() => router.push('/scan')}
                className="group border border-gray-300 text-gray-700 px-8 py-3 text-base font-medium hover:border-gray-400 hover:text-gray-900 transition-all duration-200 tracking-wide"
              >
                Scan QR Code
              </button>
            </div>

            {/* Quick Access Links */}
            <div className="mt-8 flex flex-wrap justify-center gap-6 text-sm text-gray-500">
              <button
                onClick={() => document.getElementById('how-it-works')?.scrollIntoView({ behavior: 'smooth' })}
                className="hover:text-gray-700 transition-colors underline decoration-dotted underline-offset-4"
              >
                How it works
              </button>
              <button
                onClick={() => document.getElementById('updates')?.scrollIntoView({ behavior: 'smooth' })}
                className="hover:text-gray-700 transition-colors underline decoration-dotted underline-offset-4"
              >
                Get Updates
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* Problem → Solution Section */}
      <section className="py-20 bg-gradient-to-b from-white via-gray-50/30 to-white relative overflow-hidden">
        {/* Subtle background pattern */}
        <div className="absolute inset-0 opacity-[0.02]">
          <div className="absolute top-20 left-20 w-72 h-72 bg-gray-900 rounded-full blur-3xl"></div>
          <div className="absolute bottom-20 right-20 w-96 h-96 bg-gray-600 rounded-full blur-3xl"></div>
        </div>

        <div className="container mx-auto px-6 relative">
          <div className="max-w-6xl mx-auto">
            {/* Header */}
            <div className="text-center mb-20">
              <div className="inline-flex items-center gap-2 bg-gray-100 px-4 py-2 rounded-full text-sm text-gray-600 mb-8">
                <div className="w-2 h-2 bg-gray-400 rounded-full animate-pulse"></div>
                The current state of payments
              </div>
              <h2 className="text-4xl lg:text-5xl font-light text-gray-900 mb-6 tracking-wide">
                From broken to <span className="italic font-light tracking-wider">beautiful</span>
              </h2>
            </div>

            {/* Comparison Layout */}
            <div className="grid lg:grid-cols-2 gap-16 items-start">
              {/* Traditional Payments - Problem */}
              <div className="relative">
                <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-md transition-shadow duration-300">
                  <div className="space-y-8">
                    <div className="space-y-4">
                      <h3 className="text-2xl lg:text-3xl font-light text-gray-900 leading-tight">
                        The problem with <br />
                        <span className="italic">traditional payments</span>
                      </h3>
                      <p className="text-lg text-gray-600 leading-relaxed">
                        Restaurants lose money to high processing fees, delayed settlements, and complex payment processes that frustrate customers.
                      </p>
                    </div>

                    <div className="space-y-6">
                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-red-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">High processing fees</h4>
                          <p className="text-gray-600">Credit card fees eat into already thin margins</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-red-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">Delayed settlements</h4>
                          <p className="text-gray-600">Wait days for payment processing and bank transfers</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-red-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">Complex payment processes</h4>
                          <p className="text-gray-600">Manual calculations and multiple payment methods create friction</p>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              {/* Payverge Solution */}
              <div className="relative">
                {/* Subtle glow effect */}
                <div className="absolute inset-0 bg-gradient-to-r from-green-500/5 to-blue-500/5 rounded-2xl blur-xl"></div>

                <div className="relative bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-md transition-shadow duration-300">
                  <div className="space-y-8">
                    <div className="space-y-4">
                      <h3 className="text-2xl lg:text-3xl font-light text-gray-900 leading-tight">
                        The <span className="italic">Payverge</span> <br />
                        solution
                      </h3>
                      <p className="text-lg text-gray-600 leading-relaxed">
                        Instant USDC settlements, transparent fees, and streamlined payment processes that delight your customers.
                      </p>
                    </div>

                    <div className="space-y-6">
                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-green-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">Instant settlements</h4>
                          <p className="text-gray-600">Receive USDC payments immediately with blockchain transparency</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-green-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">Lower fees</h4>
                          <p className="text-gray-600">Transparent 2% platform fee with no hidden charges</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-full bg-green-50 flex items-center justify-center flex-shrink-0 mt-1">
                          <svg className="w-4 h-4 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                          </svg>
                        </div>
                        <div>
                          <h4 className="font-medium text-gray-900 mb-1">Streamlined payments</h4>
                          <p className="text-gray-600">Simple, fast payment processing with automatic calculations</p>
                        </div>
                      </div>
                    </div>

                  </div>
                </div>
              </div>
            </div>

            {/* Call to Action */}
            <div className="pt-4 text-center">
              <button
                onClick={() => router.push('/business/register')}
                className="inline-flex items-center gap-2 bg-gray-900 text-white px-6 py-3 rounded-lg font-medium hover:bg-gray-800 transition-colors group"
              >
                Get started today
                <svg className="w-4 h-4 transition-transform group-hover:translate-x-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* Key Features Section */}
      <section className="py-16 bg-white relative">
        {/* Subtle grid pattern */}
        <div className="absolute inset-0 opacity-[0.02]" style={{ backgroundImage: 'radial-gradient(circle at 1px 1px, rgba(0,0,0,0.15) 1px, transparent 0)', backgroundSize: '20px 20px' }}></div>

        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center mb-20">
            <div className="inline-flex items-center gap-2 bg-blue-50 px-4 py-2 rounded-full text-sm text-blue-700 mb-8">
              <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M11.3 1.046A1 1 0 0112 2v5h4a1 1 0 01.82 1.573l-7 10A1 1 0 018 18v-5H4a1 1 0 01-.82-1.573l7-10a1 1 0 011.12-.38z" clipRule="evenodd" />
              </svg>
              Core features
            </div>
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Built for modern hospitality
            </h2>
            <p className="text-lg text-gray-600 font-light leading-relaxed">
              Everything you need to accept crypto payments and manage your business
            </p>
          </div>

          <div className="max-w-6xl mx-auto">
            <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-8">
              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Digital Menus</h3>
                <p className="text-gray-600 leading-relaxed">Guests scan QR codes to browse sleek online menus with real-time availability</p>
              </div>

              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">USDC Payments</h3>
                <p className="text-gray-600 leading-relaxed">Fast, secure cryptocurrency payments handled seamlessly onchain</p>
              </div>

              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Crypto Tipping</h3>
                <p className="text-gray-600 leading-relaxed">Diners add tips in USDC, routed directly to your designated tipping wallet</p>
              </div>

              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Instant Settlement</h3>
                <p className="text-gray-600 leading-relaxed">Receive funds in your wallet immediately, minus a flat 2% fee</p>
              </div>

              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Onchain Transparency</h3>
                <p className="text-gray-600 leading-relaxed">Immutable logs for all transactions with complete transparency</p>
              </div>

              <div className="group text-center p-6 rounded-xl hover:bg-gray-50/50 transition-all duration-300 hover:scale-105">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Global Access</h3>
                <p className="text-gray-600 leading-relaxed">Accept payments from anyone with a crypto wallet, anywhere in the world</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* How It Works Section */}
      <section id="how-it-works" className="py-16 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-20">
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              How it works
            </h2>
            <p className="text-lg text-gray-600 font-light leading-relaxed">
              Simple, secure, and seamless crypto payments in three steps
            </p>
          </div>

          <div className="max-w-5xl mx-auto">
            <div className="grid lg:grid-cols-3 gap-16">
              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">1</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Guest Orders</h3>
                <p className="text-gray-600 leading-relaxed">
                  Diners scan your QR code, browse the menu, and place their order with items automatically added to their bill
                </p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">2</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Pay with USDC</h3>
                <p className="text-gray-600 leading-relaxed">
                  Customers pay their bills directly in USDC with optional tips, all processed instantly and securely
                </p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">3</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Instant Settlement</h3>
                <p className="text-gray-600 leading-relaxed">
                  Funds arrive in your wallet immediately with complete transparency. All transactions are recorded onchain
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Who It's For Section */}
      <section className="py-16 bg-white">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center mb-20">
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Built for hospitality
            </h2>
            <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl mx-auto">
              Whether you run a small café or a high-end restaurant, Payverge gives you modern payments without the friction
            </p>
          </div>

          <div className="max-w-4xl mx-auto">
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-8">
              <div className="text-center">
                <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                  </svg>
                </div>
                <h3 className="text-sm font-medium text-gray-900">Restaurants</h3>
              </div>
              <div className="text-center">
                <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
                  </svg>
                </div>
                <h3 className="text-sm font-medium text-gray-900">Bars & Lounges</h3>
              </div>
              <div className="text-center">
                <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M10.5 3L12 2l1.5 1H21l-1 6H4l-1-6h7.5z" />
                  </svg>
                </div>
                <h3 className="text-sm font-medium text-gray-900">Coffee Shops</h3>
              </div>
              <div className="text-center">
                <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                  </svg>
                </div>
                <h3 className="text-sm font-medium text-gray-900">Hotels</h3>
              </div>
              <div className="text-center">
                <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 4V2a1 1 0 011-1h8a1 1 0 011 1v2m-9 0h10m-10 0a2 2 0 00-2 2v14a2 2 0 002 2h10a2 2 0 002-2V6a2 2 0 00-2-2M9 12l2 2 4-4" />
                  </svg>
                </div>
                <h3 className="text-sm font-medium text-gray-900">Event Venues</h3>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Updates Section */}
      <section id="updates" className="py-16 bg-gray-900 text-white">
        <div className="container mx-auto px-6">
          <div className="max-w-3xl mx-auto text-center">
            <h2 className="text-3xl lg:text-4xl font-light mb-6 tracking-wide">
              Stay updated on Payverge
            </h2>
            <p className="text-lg text-gray-300 mb-12 font-light leading-relaxed">
              Get updates on new features, improvements, and crypto payment insights
            </p>

            <form onSubmit={handleWaitlistSubmit} className="max-w-sm mx-auto space-y-4">
              <input
                type="email"
                placeholder="Enter your email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="w-full px-4 py-3 bg-white text-gray-900 placeholder-gray-500 border-0 focus:outline-none focus:ring-1 focus:ring-gray-300"
              />
              <input
                type="text"
                placeholder="Business name (optional)"
                value={businessName}
                onChange={(e) => setBusinessName(e.target.value)}
                className="w-full px-4 py-3 bg-white text-gray-900 placeholder-gray-500 border-0 focus:outline-none focus:ring-1 focus:ring-gray-300"
              />
              <button
                type="submit"
                disabled={isSubmitting}
                className="w-full bg-white text-gray-900 px-6 py-3 font-medium hover:bg-gray-100 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isSubmitting ? "Joining..." : "Get Updates"}
              </button>
            </form>
          </div>
        </div>
      </section>

      {/* Trust & Vision Section */}
      <section className="py-16 bg-white">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center">
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              Why Payverge?
            </h2>
            <p className="text-lg text-gray-600 font-light leading-relaxed mb-16">
              Built for the future of hospitality payments
            </p>

            <div className="grid md:grid-cols-3 gap-12 mb-16">
              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Built on Ethereum & USDC</h3>
                <p className="text-gray-600 leading-relaxed">Transparent, global, and trusted infrastructure you can rely on</p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">Hospitality-First Design</h3>
                <p className="text-gray-600 leading-relaxed">Menus, bills, tips, and accountability designed specifically for hospitality</p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
                  <svg className="w-6 h-6 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3">No Upfront Fees</h3>
                <p className="text-gray-600 leading-relaxed">Just 2% per transaction. Simple and transparent pricing</p>
              </div>
            </div>

            <div className="bg-gray-50 p-8 max-w-2xl mx-auto">
              <p className="text-gray-600 leading-relaxed">
                Payverge is a crypto-native hospitality payment platform built to replace
                outdated card processors with instant, transparent USDC payments
              </p>
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
