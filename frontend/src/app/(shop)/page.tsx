"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslation } from "@/i18n/useTranslation";

// Force dynamic rendering to avoid static generation issues
export const dynamic = 'force-dynamic';
export const revalidate = 0;

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
        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-8">
              <div className="w-2 h-2 bg-green-400 rounded-full"></div>
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
              </button>
              <button
                onClick={() => router.push('/scan')}
                className="group border border-gray-300 text-gray-700 px-8 py-3 text-base font-medium hover:border-gray-400 hover:text-gray-900 transition-all duration-200 tracking-wide"
              >
                Scan QR Code
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* Simple features section */}
      <section className="py-16 bg-gray-50">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto text-center">
            <h2 className="text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">
              How it works
            </h2>
            <div className="grid lg:grid-cols-3 gap-16">
              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">1</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Guest Orders</h3>
                <p className="text-gray-600 leading-relaxed">
                  Diners scan your QR code, browse the menu, and place their order
                </p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">2</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Pay with USDC</h3>
                <p className="text-gray-600 leading-relaxed">
                  Customers pay their bills directly in USDC with optional tips
                </p>
              </div>

              <div className="text-center">
                <div className="w-12 h-12 bg-gray-900 rounded-full flex items-center justify-center mx-auto mb-8">
                  <span className="text-lg font-medium text-white">3</span>
                </div>
                <h3 className="text-xl font-medium text-gray-900 mb-4">Instant Settlement</h3>
                <p className="text-gray-600 leading-relaxed">
                  Funds arrive in your wallet immediately with complete transparency
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Updates Section */}
      <section className="py-16 bg-gray-900 text-white">
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

      {/* Success Modal */}
      {showSuccessModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-lg max-w-md w-full p-8 relative">
            <button
              onClick={() => setShowSuccessModal(false)}
              className="absolute top-4 right-4 text-gray-400 hover:text-gray-600 transition-colors"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>

            <div className="flex justify-center mb-6">
              <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center">
                <svg className="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
            </div>

            <div className="text-center">
              <h3 className="text-xl font-semibold text-gray-900 mb-3">
                Welcome to the future! 🚀
              </h3>
              <p className="text-gray-600 mb-6 leading-relaxed">
                Thanks for subscribing to updates! We&apos;ll keep you informed about new features and improvements.
              </p>

              <div className="border-t pt-6">
                <p className="text-sm text-gray-500 mb-4">
                  Help us spread the word!
                </p>
                <button
                  onClick={handleShareOnX}
                  className="inline-flex items-center gap-2 bg-black text-white px-6 py-3 rounded-lg hover:bg-gray-800 transition-colors font-medium"
                >
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