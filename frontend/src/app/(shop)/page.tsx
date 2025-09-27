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
  const [message, setMessage] = useState("");
  const [showSuccessModal, setShowSuccessModal] = useState(false);
  const { t } = useTranslation();
  const router = useRouter();

  // Dynamic content for typewriter effect - dream-focused
  const dreamWords = [
    "instantly",
    "effortlessly",
    "globally",
    "fairly",
    "seamlessly",
    "borderlessly"
  ];


  const handleWaitlistSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);

    try {
      const { subscribeToWaitlist } = await import('@/api/subscribers/getSubscribers');
      const result = await subscribeToWaitlist(email, businessName, message);

      if (result.success) {
        setEmail("");
        setBusinessName("");
        setMessage("");
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
    const tweetText = encodeURIComponent("Just signed up for updates from https://payverge.io, the future of crypto payments for hospitality! 🚀 Excited to see what they're building.");
    const tweetUrl = `https://twitter.com/intent/tweet?text=${tweetText}`;
    window.open(tweetUrl, '_blank');
  };

  return (
    <>
      <style jsx>{`
        @keyframes fade-in {
          from {
            opacity: 0;
            transform: translateY(20px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }
        .animate-fade-in {
          animation: fade-in 0.8s ease-out;
        }
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
                Coming soon 
              </div>

              <h1 className="text-5xl lg:text-7xl font-medium text-gray-900 mb-8 leading-tight tracking-tight">
                From the table to your wallet,<br />
                <TypewriterText
                  words={dreamWords}
                  className="bg-gradient-to-r from-blue-600 via-purple-600 to-indigo-600 bg-clip-text text-transparent font-semibold italic"
                />
              </h1>
              <p className="text-lg font-light leading-relaxed max-w-2xl mx-auto text-gray-600 mb-12 tracking-wide">
                Payverge makes dining payments borderless, fair, and effortless. Guests scan, pay in USDC,
                and you&apos;re paid instantly. No banks. No friction.
              </p>
              {/* Not yet to be deployed! */}
              <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-12">
              <Button size="large" onClick={() => router.push('/business/register')}>
                Register Business
              </Button>
              <Button variant="secondary" size="large" onClick={() => router.push('/dashboard')}>
                Access Dashboard
              </Button>
            </div>
              <div className="text-center">
                <Button size="large" onClick={() => document.getElementById('waitlist')?.scrollIntoView({ behavior: 'smooth' })}>
                  Become an Early Partner
                </Button>
              </div>
            </div>
          </div>
        </section>

        {/* Vision Section */}
        <section id="vision" className="py-16 lg:py-20 bg-gray-50">
          <div className="w-full">
            <div className="container mx-auto px-6">
              <div className="max-w-4xl mx-auto text-center mb-16">
                <div className="inline-flex items-center px-4 py-2 bg-white border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 shadow-sm">
                  <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '0.5s' }}></div>
                  Why Payverge
                </div>
                <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                  Hospitality deserves <span className="italic font-light tracking-wider">better</span>
                </h2>
                <div className="text-lg font-light leading-relaxed text-gray-600 tracking-wide text-left max-w-3xl mx-auto space-y-4">
                  <p>
                    For too long, restaurants, cafés, and bars have been chained to slow, expensive, and outdated payment systems.
                    Fees eat into margins. Settlements take days. Tips get lost in the process. Guests are forced into clunky card
                    machines and awkward bill splits.
                  </p>
                  <p>
                    At Payverge, we believe payments should be as effortless as the meal itself. We&apos;re not building just another
                    payment processor, <strong>we&apos;re building a new standard.</strong>
                  </p>
                </div>
              </div>

              {/* Key Benefits - Fast. Fair. Global. */}
              <div className="text-center mb-12">
                <h3 className="text-2xl lg:text-3xl font-light text-gray-900 mb-8 tracking-wide">
                  Fast. Fair. Global.
                </h3>
              </div>

              <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto mb-12">
                <div className="group bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-blue-300">
                  <div className="w-12 h-12 bg-blue-50 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-blue-100 transition-colors duration-300">
                    <svg className="w-6 h-6 text-blue-600 group-hover:text-blue-700 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Instant Settlement</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Get paid in USDC the moment your guests finish their meal.</p>
                </div>

                <div className="group bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-purple-300">
                  <div className="w-12 h-12 bg-purple-50 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-purple-100 transition-colors duration-300">
                    <svg className="w-6 h-6 text-purple-600 group-hover:text-purple-700 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 715.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 919.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Smart Bill Splitting</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Guests can split payments automatically. No coordination needed.</p>
                </div>

                <div className="group bg-white p-8 rounded-2xl shadow-sm border border-gray-200 text-center hover:shadow-lg hover:-translate-y-1 transition-all duration-300 hover:border-indigo-300">
                  <div className="w-12 h-12 bg-indigo-50 rounded-full flex items-center justify-center mx-auto mb-6 group-hover:bg-indigo-100 transition-colors duration-300">
                    <svg className="w-6 h-6 text-indigo-600 group-hover:text-indigo-700 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Global Payments</h3>
                  <p className="text-gray-600 font-light leading-relaxed">Accept guests from anywhere in the world as if they were local.</p>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* How It Works Section */}
        <section id="how-it-works" className="py-16 lg:py-20 bg-white">
          <div className="container mx-auto px-6">
            <div className="max-w-4xl mx-auto text-center mb-16">
              <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '1s' }}></div>
                How It Works
              </div>
              <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                Payments as easy as <span className="italic font-light tracking-wider">dining</span>
              </h2>
              <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
                Three simple steps to transform your payment experience
              </p>
            </div>

            <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto mb-12">
              {/* Step 1: Scan */}
              <div className="group bg-gray-50 p-8 rounded-2xl text-center relative hover:bg-white hover:shadow-md transition-all duration-300">
                <div className="absolute -top-4 left-1/2 transform -translate-x-1/2 w-8 h-8 bg-gray-800 text-white rounded-full flex items-center justify-center text-sm font-light group-hover:bg-gray-900 transition-colors duration-300">
                  1
                </div>
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 mt-4 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-700 group-hover:text-gray-800 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v1m6 11h2m-6 0h-2v4m0-11v3m0 0h.01M12 12h4.01M16 20h4M4 12h4m12 0h.01M5 8h2a1 1 0 001-1V5a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1zm12 0h2a1 1 0 001-1V5a1 1 0 00-1-1h-2a1 1 0 00-1 1v2a1 1 0 001 1zM5 20h2a1 1 0 001-1v-2a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1z" />
                  </svg>
                </div>
                <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Scan</h3>
                <p className="text-gray-600 font-light leading-relaxed">Guest scans a QR code at their table.</p>
              </div>

              {/* Step 2: Pay */}
              <div className="group bg-gray-50 p-8 rounded-2xl text-center relative hover:bg-white hover:shadow-md transition-all duration-300">
                <div className="absolute -top-4 left-1/2 transform -translate-x-1/2 w-8 h-8 bg-gray-800 text-white rounded-full flex items-center justify-center text-sm font-light group-hover:bg-gray-900 transition-colors duration-300">
                  2
                </div>
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 mt-4 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-700 group-hover:text-gray-800 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5a2 2 0 11-4 0 2 2 0 014 0z" />
                  </svg>
                </div>
                <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Pay or Split</h3>
                <p className="text-gray-600 font-light leading-relaxed">Choose to pay the full amount or split it automatically with friends.</p>
              </div>

              {/* Step 3: Done */}
              <div className="group bg-gray-50 p-8 rounded-2xl text-center relative hover:bg-white hover:shadow-md transition-all duration-300">
                <div className="absolute -top-4 left-1/2 transform -translate-x-1/2 w-8 h-8 bg-gray-800 text-white rounded-full flex items-center justify-center text-sm font-light group-hover:bg-gray-900 transition-colors duration-300">
                  3
                </div>
                <div className="w-12 h-12 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6 mt-4 group-hover:bg-gray-200 transition-colors duration-300">
                  <svg className="w-6 h-6 text-gray-700 group-hover:text-gray-800 transition-colors duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                </div>
                <h3 className="text-xl font-light text-gray-900 mb-3 tracking-wide">Done</h3>
                <p className="text-gray-600 font-light leading-relaxed">Money flows directly to your wallet, tips included.</p>
              </div>
            </div>
          </div>
        </section>

        {/* Roadmap / Coming Soon Section */}
        <section className="py-16 lg:py-20 bg-gray-50">
          <div className="w-full">
            <div className="container mx-auto px-6">
              <div className="max-w-4xl mx-auto text-center mb-16">
                <div className="inline-flex items-center px-4 py-2 bg-white border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 shadow-sm">
                  <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '1.5s' }}></div>
                  Coming Soon
                </div>
                <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                  We&apos;re just getting <span className="italic font-light tracking-wider">started</span>
                </h2>
                <div className="text-lg font-light leading-relaxed text-gray-600 tracking-wide text-left max-w-3xl mx-auto space-y-4">
                  <p>
                    Payverge will roll out with restaurants, cafés, and bars worldwide. We&apos;re working closely with early
                    partners to shape the future of hospitality payments.
                  </p>
                </div>
              </div>

              <div className="text-center">
                <Button size="large" onClick={() => document.getElementById('waitlist')?.scrollIntoView({ behavior: 'smooth' })}>
                  Become an Early Partner
                </Button>
              </div>
            </div>
          </div>
        </section>

        {/* Complete Platform Section */}
        <section className="py-16 lg:py-20 bg-white">
          <div className="container mx-auto px-6">
            <div className="max-w-4xl mx-auto text-center mb-16">
              <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '2s' }}></div>
                Complete platform
              </div>
              <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                Beyond payments, a complete <span className="italic font-light tracking-wider">restaurant tech</span>
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
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-900 mb-3 tracking-wide">Instant QR Codes</h3>
                <p className="text-gray-600 text-sm font-light leading-relaxed">
                  Generate unique QR codes for each table with automatic bill creation and payment tracking
                </p>
              </div>
            </div>

            <div className="text-center">
              <p className="text-sm text-gray-500 mb-6 font-light tracking-wide">All features included • Annual subscription • 2% per transaction</p>
              {/* <Button size="large" onClick={() => router.push('/business/register')}>
                Start Your Business
              </Button> */}
            </div>
          </div>
        </section>

        {/* Social Proof Section */}
        <section className="py-16 lg:py-20 bg-gray-50">
          <div className="w-full">
            <div className="container mx-auto px-6">
              <div className="max-w-4xl mx-auto text-center mb-16">
                <div className="inline-flex items-center px-4 py-2 bg-white border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 shadow-sm">
                  <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '2.5s' }}></div>
                  Trusted by hospitality
                </div>
                <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                  Built for <span className="italic font-light tracking-wider">modern restaurants</span>
                </h2>
                <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
                  From coffee shops to fine dining, crypto payments for every hospitality business
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
          </div>
        </section>

        {/* Final CTA Section */}
        <section id="waitlist" className="py-16 lg:py-20 bg-white">
          <div className="container mx-auto px-6">
            <div className="max-w-4xl mx-auto text-center mb-16">
              <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse" style={{ animationDelay: '3s' }}></div>
                Join the Movement
              </div>
              <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
                The future of dining runs on <span className="italic font-light tracking-wider">Payverge</span>
              </h2>
              <p className="text-lg font-light leading-relaxed text-gray-600 tracking-wide">
                Start accepting instant USDC payments, reduce fees, and give your guests a seamless experience
              </p>
            </div>

            {/* <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-8">
              <Button size="large" onClick={() => router.push('/business/register')}>
                Register Your Business
              </Button>
              <Button variant="secondary" size="large" onClick={() => router.push('/dashboard')}>
                Access Dashboard
              </Button>
            </div> */}

            {/* Pricing info */}
            <div className="text-center mb-16">
              <p className="text-sm text-gray-600 mb-2">
                One-time setup fee • Annual subscription • 2% transaction fee
              </p>
              {/* <button
                onClick={() => document.getElementById('updates')?.scrollIntoView({ behavior: 'smooth' })}
                className="text-gray-500 hover:text-gray-700 transition-colors underline text-sm"
              >
                Become an Early Partner for Special Pricing
              </button> */}
            </div>

            <div id="updates" className="max-w-2xl mx-auto">
              <div className="bg-gray-50 p-8 rounded-2xl text-center">
                <h3 className="text-xl font-medium text-gray-900 mb-4 tracking-wide">Become an Early Partner</h3>
                <p className="text-gray-600 font-light mb-6 leading-relaxed">
                  Get special pricing, priority support, and help shape the future of restaurant payments.
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
                  <input
                    type="text"
                    placeholder="Message (optional)"
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    className="w-full px-4 py-3 bg-white text-gray-900 placeholder-gray-500 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent font-light"
                  />
                  <Button
                    type="submit"
                    disabled={isSubmitting}
                    className="w-full"
                  >
                    {isSubmitting ? "Joining..." : "Join us!"}
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
                  Welcome to the Early Partner Program! 🚀
                </h3>
                <p className="text-gray-600 mb-6 leading-relaxed">
                  Thanks for joining! We&apos;ll contact you with special pricing details and early access to Payverge.
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

        {/* Footer */}
        <footer className="py-12 bg-gray-50 border-t border-gray-200">
          <div className="container mx-auto px-6">
            <div className="max-w-4xl mx-auto">
              <div className="flex flex-col md:flex-row justify-between items-center mb-8">
                <div className="mb-4 md:mb-0">
                  <h3 className="text-lg font-medium text-gray-900 tracking-wide">Payverge</h3>
                  <p className="text-sm text-gray-600 font-light">Modern payments for hospitality</p>
                </div>

                <div className="flex gap-6 text-sm">
                  <a href="#" className="text-gray-600 hover:text-gray-900 transition-colors">About</a>
                  <a href="#" className="text-gray-600 hover:text-gray-900 transition-colors">Contact</a>
                  <a href="#" className="text-gray-600 hover:text-gray-900 transition-colors">Privacy</a>
                  <a href="#" className="text-gray-600 hover:text-gray-900 transition-colors">Terms</a>
                </div>
              </div>

              <div className="text-center pt-8 border-t border-gray-200">
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
