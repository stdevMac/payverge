"use client";

import React, { useState, useEffect } from "react";
import { Card, CardBody } from "@nextui-org/react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

type StringArray = string[];

interface TermsTranslations {
  title: string;
  sections: {
    acceptance: {
      title: string;
      content: string;
    };
    definitions: {
      title: string;
      items: string[] | string;
    };
    eligibility: {
      title: string;
      intro: string;
      requirements: string[] | string;
    };
    investment: {
      title: string;
      structure: {
        title: string;
        content: string;
      };
      period: {
        title: string;
        content: string;
      };
    };
    fees: {
      title: string;
      setup: {
        title: string;
        content: string;
        items: string[] | string;
      };
      deposit: {
        title: string;
        content: string;
        items: string[] | string;
      };
      charges: {
        title: string;
        content: string;
        items: string[] | string;
      };
    };
    income: {
      title: string;
      items: string[] | string;
    };
    vehicle: {
      title: string;
      items: string[] | string;
    };
    termination: {
      title: string;
      items: string[] | string;
    };
    risks: {
      title: string;
      items: string[] | string;
    };
    law: {
      title: string;
      content: string;
    };
    contact: {
      title: string;
      intro: string;
      company: string;
      address1: string;
      address2: string;
      phone: string;
      email: string;
    };
    lastUpdated: string;
  };
}

export default function TermsPage() {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `terms.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  // Helper function to safely handle array translations
  const getTranslatedArray = (key: string): string[] => {
    const value = tString(key);
    return Array.isArray(value) ? value : [];
  };
  return (
    <div className="max-w-4xl mx-auto py-8 px-4">
      <h1 className="text-3xl font-bold mb-8 text-center">
        {tString('title')}
      </h1>

      <div className="space-y-8">
        {/* Acceptance of Terms */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.acceptance.title')}
            </h2>
            <p className="text-gray-700 mb-4">
              {tString('sections.acceptance.content')}
            </p>
          </CardBody>
        </Card>

        {/* Definitions */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.definitions.title')}</h2>
            <div className="space-y-3 text-gray-700">
              {getTranslatedArray('terms.sections.definitions.items').map((item: string, index: number) => (
                <p key={index}>{item}</p>
              ))}
            </div>
          </CardBody>
        </Card>

        {/* Eligibility */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.eligibility.title')}</h2>
            <p className="text-gray-700 mb-4">
              {tString('sections.eligibility.intro')}
            </p>
            <ul className="list-disc list-inside text-gray-700 space-y-2">
              {getTranslatedArray('terms.sections.eligibility.requirements').map((item: string, index: number) => (
                <li key={index}>{item}</li>
              ))}
            </ul>
          </CardBody>
        </Card>

        {/* Investment Terms */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.investment.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">{tString('sections.investment.structure.title')}</h3>
              <p className="text-gray-700">
                {tString('sections.investment.structure.content')}
              </p>

              <h3 className="text-lg font-medium">{tString('sections.investment.period.title')}</h3>
              <p className="text-gray-700">
                {tString('sections.investment.period.content')}
              </p>
            </div>
          </CardBody>
        </Card>

        {/* Fees and Payments */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.fees.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">
                {tString('sections.fees.setup.title')}
              </h3>
              <p className="text-gray-700">
                {tString('sections.fees.setup.content')}
              </p>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('terms.sections.fees.setup.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">
                {tString('sections.fees.deposit.title')}
              </h3>
              <p className="text-gray-700">
                {tString('sections.fees.deposit.content')}
              </p>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('terms.sections.fees.deposit.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">{tString('sections.fees.charges.title')}</h3>
              <p className="text-gray-700">
                {tString('sections.fees.charges.content')}
              </p>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('terms.sections.fees.charges.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
            </div>
          </CardBody>
        </Card>

        {/* Income Distribution */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.income.title')}
            </h2>
            <div className="space-y-4">
              {getTranslatedArray('terms.sections.income.items').map((item: string, index: number) => (
                <p key={index} className="text-gray-700">{item}</p>
              ))}
            </div>
          </CardBody>
        </Card>

        {/* Vehicle Management */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.vehicle.title')}
            </h2>
            {getTranslatedArray('terms.sections.vehicle.items').map((item: string, index: number) => (
              <p key={index} className="text-gray-700">{item}</p>
            ))}
          </CardBody>
        </Card>

        {/* Term and Termination */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.termination.title')}
            </h2>
            <div className="space-y-4">
              {getTranslatedArray('terms.sections.termination.items').map((item: string, index: number) => (
                <p key={index} className="text-gray-700">{item}</p>
              ))}
            </div>
          </CardBody>
        </Card>

        {/* Risks and Disclaimers */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.risks.title')}
            </h2>
            <div className="space-y-4">
              {getTranslatedArray('terms.sections.risks.items').map((item: string, index: number) => (
                <p key={index} className="text-gray-700">{item}</p>
              ))}
            </div>
          </CardBody>
        </Card>

        {/* Governing Law */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.law.title')}</h2>
            <p className="text-gray-700">
              {tString('sections.law.content')}
            </p>
          </CardBody>
        </Card>

        {/* Contact Information */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {tString('sections.contact.title')}
            </h2>
            <p className="text-gray-700 mb-4">
              {tString('sections.contact.intro')}
            </p>
            <div className="text-gray-700 space-y-2">
              <p>
                <strong>{tString('sections.contact.company')}</strong>
              </p>
              <p>{tString('sections.contact.address1')}</p>
              <p>{tString('sections.contact.address2')}</p>
              <p>{tString('sections.contact.phone')}</p>
              <p>{tString('sections.contact.email')}</p>
            </div>
          </CardBody>
        </Card>

        <div className="text-sm text-gray-500 mt-8">
          {tString('sections.lastUpdated')}
        </div>
      </div>
    </div>
  );
}
