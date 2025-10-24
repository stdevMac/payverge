"use client";

import React, { useState, useEffect } from "react";
import { Card, CardBody } from "@nextui-org/react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

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
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `privacy.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  // Helper function to safely handle array translations
  const getTranslatedArray = (key: string): string[] => {
    const value = getTranslation(key, currentLocale);
    return Array.isArray(value) ? value : [];
  };
  return (
    <div className="max-w-4xl mx-auto py-8 px-4">
      <h1 className="text-3xl font-bold mb-8 text-center">{tString('title')}</h1>

      <div className="space-y-8">
        {/* Introduction */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.introduction.title')}</h2>
            <p className="text-gray-700 mb-4">
              {tString('sections.introduction.content')}
            </p>
          </CardBody>
        </Card>

        {/* Information We Collect */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.information.title')}</h2>
            <div className="space-y-4">
              <h3 className="text-lg font-medium">{tString('sections.information.personal.title')}</h3>
              <ul className="list-disc list-inside text-gray-700 space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.information.personal.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>

              <h3 className="text-lg font-medium">{tString('sections.information.technical.title')}</h3>
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
            <h2 className="text-xl font-semibold mb-4">{tString('sections.usage.title')}</h2>
            <div className="space-y-3 text-gray-700">
              <p>{tString('sections.usage.intro')}</p>
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
            <h2 className="text-xl font-semibold mb-4">{tString('sections.sharing.title')}</h2>
            <div className="space-y-4 text-gray-700">
              <p>{tString('sections.sharing.intro')}</p>
              <ul className="list-disc list-inside space-y-2 ml-4">
                {getTranslatedArray('privacy.sections.sharing.items').map((item: string, index: number) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
              <p>{tString('sections.sharing.note')}</p>
            </div>
          </CardBody>
        </Card>

        {/* Data Security */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.security.title')}</h2>
            <p className="text-gray-700">
              {tString('sections.security.content')}
            </p>
          </CardBody>
        </Card>

        {/* Your Rights */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.rights.title')}</h2>
            <div className="space-y-3 text-gray-700">
              <p>{tString('sections.rights.intro')}</p>
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
            <h2 className="text-xl font-semibold mb-4">{tString('sections.cookies.title')}</h2>
            <p className="text-gray-700">
              {tString('sections.cookies.content')}
            </p>
          </CardBody>
        </Card>

        {/* Changes to Privacy Policy */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.changes.title')}</h2>
            <p className="text-gray-700">
              {tString('sections.changes.content')}
            </p>
          </CardBody>
        </Card>

        {/* Contact Information */}
        <Card>
          <CardBody>
            <h2 className="text-xl font-semibold mb-4">{tString('sections.contact.title')}</h2>
            <div className="text-gray-700 space-y-2">
              <p>{tString('sections.contact.intro')}</p>
              <p><strong>{tString('sections.contact.company')}</strong></p>
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
};

export default PrivacyPage;
