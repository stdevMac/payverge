"use client";

import React from "react";
import { Card, CardBody } from "@nextui-org/react";
import { useTranslation } from "@/i18n/useTranslation";

type StringArray = string[];

interface PrivacyTranslations {
  title: string;
  sections: {
    introduction: {
      title: string;
      content: string;
    };
    information: {
      title: string;
      personal: {
        title: string;
        items: string[] | string;
      };
      technical: {
        title: string;
        items: string[] | string;
      };
    };
    usage: {
      title: string;
      intro: string;
      items: string[] | string;
    };
    sharing: {
      title: string;
      intro: string;
      items: string[] | string;
      note: string;
    };
    security: {
      title: string;
      content: string;
    };
    rights: {
      title: string;
      intro: string;
      items: string[] | string;
    };
    cookies: {
      title: string;
      content: string;
    };
    changes: {
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

const PrivacyPage = () => {
  const { t } = useTranslation();
  
  // Helper function to safely handle array translations
  const getTranslatedArray = (key: string): string[] => {
    const value = t(key);
    return Array.isArray(value) ? value : [];
  };
  return (
    <div className="max-w-4xl mx-auto py-8 px-4">
      <h1 className="text-3xl font-bold mb-8 text-center">{t('privacy.title')}</h1>

      <div className="space-y-8">
        {/* Introduction */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.introduction.title')}</h2>
            <p className="text-gray-700 mb-4">
              {t('privacy.sections.introduction.content')}
            </p>
          </CardBody>
        </Card>

        {/* Information We Collect */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.information.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">{t('privacy.sections.information.personal.title')}</h3>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.information.personal.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">{t('privacy.sections.information.technical.title')}</h3>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.information.technical.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
            </div>
          </CardBody>
        </Card>

        {/* How We Use Your Information */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.usage.title')}</h2>
            <div className="space-y-3 text-gray-700">
              <p>{t('privacy.sections.usage.intro')}</p>
              <ul className="list-disc list-inside space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.usage.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
            </div>
          </CardBody>
        </Card>

        {/* Information Sharing */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.sharing.title')}</h2>
            <div className="space-y-4 text-gray-700">
              <p>{t('privacy.sections.sharing.intro')}</p>
              <ul className="list-disc list-inside space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.sharing.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
              <p>{t('privacy.sections.sharing.note')}</p>
            </div>
          </CardBody>
        </Card>

        {/* Data Security */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.security.title')}</h2>
            <p className="text-gray-700">
              {t('privacy.sections.security.content')}
            </p>
          </CardBody>
        </Card>

        {/* Your Rights */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.rights.title')}</h2>
            <div className="space-y-3 text-gray-700">
              <p>{t('privacy.sections.rights.intro')}</p>
              <ul className="list-disc list-inside space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.rights.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
            </div>
          </CardBody>
        </Card>

        {/* Cookies */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.cookies.title')}</h2>
            <p className="text-gray-700">
              {t('privacy.sections.cookies.content')}
            </p>
          </CardBody>
        </Card>

        {/* Changes to Privacy Policy */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.changes.title')}</h2>
            <p className="text-gray-700">
              {t('privacy.sections.changes.content')}
            </p>
          </CardBody>
        </Card>

        {/* Contact Information */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{t('privacy.sections.contact.title')}</h2>
            <div className="text-gray-700 space-y-2">
              <p>{t('privacy.sections.contact.intro')}</p>
              <p><strong>{t('privacy.sections.contact.company')}</strong></p>
              <p>{t('privacy.sections.contact.address1')}</p>
              <p>{t('privacy.sections.contact.address2')}</p>
              <p>{t('privacy.sections.contact.phone')}</p>
              <p>{t('privacy.sections.contact.email')}</p>
            </div>
          </CardBody>
        </Card>

        <div className="text-sm text-gray-500 mt-8">
          {t('privacy.sections.lastUpdated')}
        </div>
      </div>
    </div>
  );
};

export default PrivacyPage;
