"use client";

import React from "react";
import Image from "next/image";
import { Card, CardBody } from "@nextui-org/react";
import {
  IoWalletOutline,
  IoCodeSlashOutline,
  IoStatsChartOutline,
  IoShieldCheckmarkOutline,
  IoRocketOutline,
  IoImageOutline,
  IoDocumentTextOutline,
  IoBarChartOutline,
} from "react-icons/io5";
import { useTranslation } from "@/i18n/useTranslation";

export default function HowItWorksPage() {
  const { t } = useTranslation();
  
  // Define steps with icons
  const steps = [
    {
      title: t('howItWorks.steps.0.title'),
      description: t('howItWorks.steps.0.description'),
      details: Array.isArray(t('howItWorks.steps.0.details')) ? t('howItWorks.steps.0.details') : [
        "Support for major Web3 wallets",
        "Secure blockchain transactions",
        "Easy portfolio management"
      ],
      icon: <IoWalletOutline className="w-12 h-12 text-primary" />
    },
    {
      title: t('howItWorks.steps.1.title'),
      description: t('howItWorks.steps.1.description'),
      details: Array.isArray(t('howItWorks.steps.1.details')) ? t('howItWorks.steps.1.details') : [
        "Quick identity verification",
        "Secure document storage",
        "Regulatory compliance"
      ],
      icon: <IoShieldCheckmarkOutline className="w-12 h-12 text-primary" />
    },
    {
      title: t('howItWorks.steps.2.title'),
      description: t('howItWorks.steps.2.description'),
      details: Array.isArray(t('howItWorks.steps.2.details')) ? t('howItWorks.steps.2.details') : [
        "Smart contract integration",
        "Transparent blockchain transactions",
        "Flexible development options"
      ],
      icon: <IoCodeSlashOutline className="w-12 h-12 text-primary" />
    },
    {
      title: t('howItWorks.steps.3.title'),
      description: t('howItWorks.steps.3.description'),
      details: Array.isArray(t('howItWorks.steps.3.details')) ? t('howItWorks.steps.3.details') : [
        "Unique portfolio identifier",
        "Secure ownership record",
        "Easy transfer and management"
      ],
      icon: <IoImageOutline className="w-12 h-12 text-primary" />
    }
  ];
  
  // Define features with icons
  const features = [
    {
      title: t('howItWorks.features.0.title'),
      description: t('howItWorks.features.0.description'),
      icon: <IoDocumentTextOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t('howItWorks.features.1.title'),
      description: t('howItWorks.features.1.description'),
      icon: <IoBarChartOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t('howItWorks.features.2.title'),
      description: t('howItWorks.features.2.description'),
      icon: <IoRocketOutline className="w-8 h-8 text-primary" />
    }
  ];
  
  // Define benefits
  const benefits = [
    {
      title: t('howItWorks.benefits.0.title'),
      description: t('howItWorks.benefits.0.description')
    },
    {
      title: t('howItWorks.benefits.1.title'),
      description: t('howItWorks.benefits.1.description')
    },
    {
      title: t('howItWorks.benefits.2.title'),
      description: t('howItWorks.benefits.2.description')
    }
  ];


  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
      {/* Hero Section */}
      <Card className="mb-16 bg-gradient-to-r from-primary to-primary/80">
        <CardBody className="text-center py-12">
          <h1 className="text-4xl font-bold text-white mb-6">
            {t('howItWorks.title')}
          </h1>
          <p className="text-white/90 max-w-3xl mx-auto text-lg">
            {t('howItWorks.subtitle')}
          </p>
        </CardBody>
      </Card>

      {/* Investment Process */}
      <div className="mb-20">
        <h2 className="text-3xl font-bold text-gray-900 mb-12 text-center">
          {t('howItWorks.investmentJourney')}
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
          {steps.map((step, index) => (
            <Card key={index} className="border border-gray-200 hover:border-primary transition-colors">
              <CardBody className="p-8">
                <div className="flex items-start gap-6">
                  <div className="flex-shrink-0 bg-primary/10 p-3 rounded-lg">
                    {step.icon}
                  </div>
                  <div>
                    <h3 className="text-xl font-semibold text-primary mb-3">
                      {step.title}
                    </h3>
                    <p className="text-gray-600 mb-4">
                      {step.description}
                    </p>
                    <ul className="space-y-2">
                      {Array.isArray(step.details) && step.details.map((detail: string, idx: number) => (
                        <li key={idx} className="flex items-center text-gray-600">
                          <span className="mr-2">•</span>
                          {detail}
                        </li>
                      ))}
                    </ul>
                  </div>
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      </div>

      {/* Platform Features */}
      <section className="mb-20">
        <h2 className="text-3xl font-bold text-gray-900 mb-12 text-center">
          {t('howItWorks.platformFeatures')}
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
          {features.map((feature, index) => (
            <Card key={index} className="border border-gray-200 hover:border-primary transition-colors">
              <CardBody className="p-8 text-center">
                <div className="flex justify-center mb-6">
                  {feature.icon}
                </div>
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {feature.title}
                </h3>
                <p className="text-gray-600">
                  {feature.description}
                </p>
              </CardBody>
            </Card>
          ))}
        </div>
      </section>

      {/* Investment Benefits */}
      <section>
        <h2 className="text-3xl font-bold text-gray-900 mb-12 text-center">
          {t('howItWorks.benefitsOfInvesting')}
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          {benefits.map((benefit: any, index: number) => (
            <Card key={index} className="border border-gray-200 hover:border-primary transition-colors">
              <CardBody className="p-8 text-center">
                <h3 className="text-xl font-semibold text-primary mb-4">
                  {benefit.title}
                </h3>
                <p className="text-gray-600">
                  {benefit.description}
                </p>
              </CardBody>
            </Card>
          ))}
        </div>
      </section>
    </div>
  );
}
