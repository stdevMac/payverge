"use client";
import { titleFont } from "@/config/font/font";
import { useState, useEffect } from "react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

interface Props {
  title: string | { key: string; defaultValue?: string };
  subtitle?: string | { key: string; defaultValue?: string };
  className?: string;
}

export const Title = ({ title, subtitle, className = "" }: Props) => {
  const clientClassName = `${titleFont} antialiased text-4xl font-semibold mt-5 mb-2`;
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const result = getTranslation(key, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  // Handle translation for title
  const translatedTitle = typeof title === 'string' 
    ? title 
    : tString(title.key) || title.defaultValue || title.key;
  
  // Handle translation for subtitle
  const translatedSubtitle = subtitle 
    ? (typeof subtitle === 'string' 
        ? subtitle 
        : tString(subtitle.key) || subtitle.defaultValue || subtitle.key) 
    : null;

  return (
    <div className={`${className} mt-3`}>
      <h1 className={clientClassName}>{translatedTitle}</h1>
      {translatedSubtitle && <h3 className="text-xl mb-10">{translatedSubtitle}</h3>}
    </div>
  );
};
