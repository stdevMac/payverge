"use client";

import React from "react";
import { Card, CardBody } from "@nextui-org/react";
import { useTranslation } from "@/i18n/useTranslation";

interface CoreValue {
  title: string;
  description: string;
  icon: string;
}

export default function MissionPage() {
  const { t } = useTranslation();
  
  // Define fallback values in case translations fail
  const fallbackValues: CoreValue[] = [
    {
      title: "Real Ownership",
      description: "Every token represents genuine ownership in our vehicle fleet, backed by real assets and secured through blockchain technology.",
      icon: "🔐"
    },
    {
      title: "Transparency",
      description: "Full visibility into fleet performance, revenue distribution, and operational metrics through our blockchain-based platform.",
      icon: "📊"
    },
    {
      title: "Innovation",
      description: "Bridging traditional car rental with blockchain technology to create a new paradigm of fractional vehicle ownership.",
      icon: "💡"
    },
    {
      title: "Accessibility",
      description: "Making car fleet investment accessible to everyone, regardless of their investment capacity.",
      icon: "🌍"
    },
    {
      title: "Sustainability",
      description: "Promoting efficient vehicle utilization and responsible asset management for long-term value creation.",
      icon: "🌱"
    },
    {
      title: "Community",
      description: "Building a community of forward-thinking investors who believe in the future of tokenized real-world assets.",
      icon: "🤝"
    }
  ];
  
  // Get values from translations with proper type safety
  const translatedValues = t('mission.values.items');
  const values: CoreValue[] = Array.isArray(translatedValues) ? translatedValues : fallbackValues;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
      {/* Hero Section */}
      <Card className="mb-16 bg-gradient-to-r from-primary to-primary/80">
        <CardBody className="text-center py-12">
          <h1 className="text-4xl font-bold text-white mb-6">
            {t('mission.title')}
          </h1>
          <p className="text-white/90 max-w-3xl mx-auto text-lg leading-relaxed">
            {t('mission.subtitle')}
          </p>
        </CardBody>
      </Card>

      <div className="space-y-16">
        {/* Vision Section */}
        <section>
          <h2 className="text-3xl font-bold text-gray-900 mb-6 text-center">
            {t('mission.vision.title')}
          </h2>
          <div className="grid md:grid-cols-2 gap-8">
            <Card>
              <CardBody className="p-8">
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {t('mission.vision.democratizing.title')}
                </h3>
                <p className="text-gray-600 leading-relaxed">
                  {t('mission.vision.democratizing.description')}
                </p>
              </CardBody>
            </Card>
            <Card>
              <CardBody className="p-8">
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {t('mission.vision.bridging.title')}
                </h3>
                <p className="text-gray-600 leading-relaxed">
                  {t('mission.vision.bridging.description')}
                </p>
              </CardBody>
            </Card>
          </div>
        </section>

        {/* What Sets Us Apart */}
        <section>
          <h2 className="text-3xl font-bold text-gray-900 mb-6 text-center">
            {t('mission.difference.title')}
          </h2>
          <div className="space-y-6">
            <Card>
              <CardBody className="p-8">
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {t('mission.difference.ownership.title')}
                </h3>
                <p className="text-gray-600 leading-relaxed">
                  {t('mission.difference.ownership.description')}
                </p>
              </CardBody>
            </Card>
            <Card>
              <CardBody className="p-8">
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {t('mission.difference.transparency.title')}
                </h3>
                <p className="text-gray-600 leading-relaxed">
                  {t('mission.difference.transparency.description')}
                </p>
              </CardBody>
            </Card>
          </div>
        </section>

        {/* Core Values */}
        <section>
          <h2 className="text-3xl font-bold text-gray-900 mb-8 text-center">
            {t('mission.values.title')}
          </h2>
          <div className="max-w-5xl mx-auto">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {values.map((value: CoreValue, index: number) => (
                <Card key={index}>
                  <CardBody className="p-6 text-center">
                    <div className="text-4xl mb-4">{value.icon}</div>
                    <h3 className="text-xl font-semibold text-primary mb-3">
                      {value.title}
                    </h3>
                    <p className="text-gray-600">
                      {value.description}
                    </p>
                  </CardBody>
                </Card>
              ))}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
