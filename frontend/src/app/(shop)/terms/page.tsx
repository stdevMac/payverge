"use client";

import React from "react";
import { Card, CardBody } from "@nextui-org/react";
import { useTranslation } from "@/i18n/useTranslation";

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
  const { t } = useTranslation();
  
  // Helper function to safely handle array translations
  const getTranslatedArray = (key: string): string[] => {
    const value = t(key);
    return Array.isArray(value) ? value : [];
  };
  return (
    <div className="max-w-4xl mx-auto py-8 px-4">
      <h1 className="text-3xl font-bold mb-8 text-center">
        {t('terms.title')}
      </h1>

      <div className="space-y-8">
        {/* Acceptance of Terms */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {t('terms.sections.acceptance.title')}
            </h2>
            <p className="text-gray-700 mb-4">
              {t('terms.sections.acceptance.content')}
            </p>
          </CardBody>
        </Card>

        {/* Definitions */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('terms.sections.definitions.title')}</h2>
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
            <h2 className="text-xl font-semibold mb-4">{t('terms.sections.eligibility.title')}</h2>
            <p className="text-gray-700 mb-4">
              {t('terms.sections.eligibility.intro')}
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
            <h2 className="text-xl font-semibold mb-4">{t('terms.sections.investment.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">{t('terms.sections.investment.structure.title')}</h3>
              <p className="text-gray-700">
                {t('terms.sections.investment.structure.content')}
              </p>

              <h3 className="text-lg font-medium">{t('terms.sections.investment.period.title')}</h3>
              <p className="text-gray-700">
                {t('terms.sections.investment.period.content')}
              </p>
            </div>
          </CardBody>
        </Card>

        {/* Fees and Payments */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('terms.sections.fees.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">
                {t('terms.sections.fees.setup.title')}
              </h3>
              <p className="text-gray-700">
                {t('terms.sections.fees.setup.content')}
              </p>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('terms.sections.fees.setup.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">
                {t('terms.sections.fees.deposit.title')}
              </h3>
              <p className="text-gray-700">
                {t('terms.sections.fees.deposit.content')}
              </p>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('terms.sections.fees.deposit.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">{t('terms.sections.fees.charges.title')}</h3>
              <p className="text-gray-700">
                {t('terms.sections.fees.charges.content')}
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
              {t('terms.sections.income.title')}
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
              {t('terms.sections.vehicle.title')}
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
              {t('terms.sections.termination.title')}
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
              {t('terms.sections.risks.title')}
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
            <h2 className="text-xl font-semibold mb-4">{t('terms.sections.law.title')}</h2>
            <p className="text-gray-700">
              {t('terms.sections.law.content')}
            </p>
          </CardBody>
        </Card>

        {/* Contact Information */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">
              {t('terms.sections.contact.title')}
            </h2>
            <p className="text-gray-700 mb-4">
              {t('terms.sections.contact.intro')}
            </p>
            <div className="text-gray-700 space-y-2">
              <p>
                <strong>{t('terms.sections.contact.company')}</strong>
              </p>
              <p>{t('terms.sections.contact.address1')}</p>
              <p>{t('terms.sections.contact.address2')}</p>
              <p>{t('terms.sections.contact.phone')}</p>
              <p>{t('terms.sections.contact.email')}</p>
            </div>
          </CardBody>
        </Card>

        <div className="text-sm text-gray-500 mt-8">
          {t('terms.sections.lastUpdated')}
        </div>
      </div>
    </div>
  );
}
