"use client";
import { useState } from "react";
import { Title, PrimarySpinner } from "@/components";
import { Selected } from "@/components";
import { FeaturedFleetCarousel } from "@/components/shared/FeaturedFleetCarousel";
import { useTranslation } from "@/i18n/useTranslation";

export default function Home() {
  const [loading, setLoading] = useState(true);
  const { t } = useTranslation();

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white">
      {/* Hero Section with Carousel, comment if not needed */}
      <section className="relative w-full">
        <FeaturedFleetCarousel />
        <div className="absolute bottom-0 left-0 right-0 h-16 bg-gradient-to-t from-white to-transparent" />
      </section>

      {/* Start Section */}
      <section className="container mx-auto px-2 sm:px-4 -mt-4 relative z-10">
        <div className="bg-white rounded-2xl shadow-lg p-3 sm:p-6 mb-6 sm:mb-8">
          <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-2 sm:gap-6 mb-4">
            <div className="sm:flex-shrink-0">
              <Title
                title={{ key: 'homepage.availableFleets', defaultValue: 'Available Fleets' }}
                subtitle={{ key: 'homepage.fleetSubtitle', defaultValue: 'Choose from our curated collection' }}
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
            <div className="flex justify-center items-center min-h-[40vh]">
              <p className="text-gray-600">We start here :P</p>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
