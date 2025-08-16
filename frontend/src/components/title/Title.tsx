"use client";
import { titleFont } from "@/config/font/font";
import { useTranslation } from "@/i18n/useTranslation";

interface Props {
  title: string | { key: string; defaultValue?: string };
  subtitle?: string | { key: string; defaultValue?: string };
  className?: string;
}

export const Title = ({ title, subtitle, className = "" }: Props) => {
  const clientClassName = `${titleFont} antialiased text-4xl font-semibold mt-5 mb-2`;
  const { t } = useTranslation();
  
  // Handle translation for title
  const translatedTitle = typeof title === 'string' 
    ? title 
    : t(title.key, {}, title.defaultValue);
  
  // Handle translation for subtitle
  const translatedSubtitle = subtitle 
    ? (typeof subtitle === 'string' 
        ? subtitle 
        : t(subtitle.key, {}, subtitle.defaultValue)) 
    : null;

  return (
    <div className={`${className} mt-3`}>
      <h1 className={clientClassName}>{translatedTitle}</h1>
      {translatedSubtitle && <h3 className="text-xl mb-10">{translatedSubtitle}</h3>}
    </div>
  );
};
