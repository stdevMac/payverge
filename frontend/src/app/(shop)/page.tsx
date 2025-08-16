"use client";
import { useState } from "react";
import { Title, PrimarySpinner } from "@/components";
import { Selected } from "@/components";
// import { FeaturedFleetCarousel } from "@/components/shared/FeaturedFleetCarousel";
import { useTranslation } from "@/i18n/useTranslation";

export default function Home() {
  const [loading, setLoading] = useState(true);
  const { t } = useTranslation();

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white">
      {/* Hero Section */}
      <section className="relative w-full bg-gradient-to-r from-blue-600 to-purple-600 text-white py-20">
        <div className="container mx-auto px-4 text-center">
          <h1 className="text-4xl md:text-6xl font-bold mb-6">
            {t('homepage.heroTitle', 'Welcome to Web3 Boilerplate')}
          </h1>
          <p className="text-xl md:text-2xl mb-8 opacity-90">
            {t('homepage.heroSubtitle', 'Build the future of decentralized applications')}
          </p>
        </div>
      </section>

      {/* Start Section */}
      <section className="container mx-auto px-2 sm:px-4 -mt-4 relative z-10">
        <div className="bg-white rounded-2xl shadow-lg p-3 sm:p-6 mb-6 sm:mb-8">
          <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-2 sm:gap-6 mb-4">
            <div className="sm:flex-shrink-0">
              <Title
                title={{ key: 'homepage.featuresTitle', defaultValue: 'Platform Features' }}
                subtitle={{ key: 'homepage.featuresSubtitle', defaultValue: 'Everything you need to build Web3 apps' }}
                className="text-left"
              />
            </div>
            {!loading && (
              <div className="w-full sm:w-[320px] sm:flex-shrink-0">
                <Selected />
              </div>
            )}
          </div>

          {loading ? (
            <div className="flex justify-center items-center min-h-[40vh]">
              <PrimarySpinner />
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="p-6 bg-gray-50 rounded-lg">
                <h3 className="text-xl font-semibold mb-3">🔐 Authentication</h3>
                <p className="text-gray-600">Secure SIWE-based authentication with wallet integration</p>
              </div>
              <div className="p-6 bg-gray-50 rounded-lg">
                <h3 className="text-xl font-semibold mb-3">💾 Database</h3>
                <p className="text-gray-600">MongoDB integration with user management</p>
              </div>
              <div className="p-6 bg-gray-50 rounded-lg">
                <h3 className="text-xl font-semibold mb-3">🚀 Ready to Deploy</h3>
                <p className="text-gray-600">Docker containerized with production-ready setup</p>
              </div>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
