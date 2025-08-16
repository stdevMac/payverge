import React from "react";
import { Button, Card, CardBody, CardHeader, Divider } from "@nextui-org/react";
import { useTranslation } from "@/i18n/TranslationContext";

interface LanguageSelectorProps {
  compact?: boolean;
  className?: string;
}

export const LanguageSelector: React.FC<LanguageSelectorProps> = ({ 
  compact = false,
  className = ""
}) => {
  const { t, language, changeLanguage } = useTranslation();

  const handleLanguageChange = (lang: string) => {
    changeLanguage(lang as any);
  };
  
  // This effect will run whenever the language changes
  React.useEffect(() => {
    console.log('Current language is now:', language);
  }, [language]);

  if (compact) {
    return (
      <div className={`flex gap-2 ${className}`}>
        <Button
          size="sm"
          variant={language === "en" ? "solid" : "light"}
          onPress={() => handleLanguageChange("en")}
          className={language === "en" ? "bg-primary text-white" : ""}
        >
          English
        </Button>
        <Button
          size="sm"
          variant={language === "es" ? "solid" : "light"}
          onPress={() => handleLanguageChange("es")}
          className={language === "es" ? "bg-primary text-white" : ""}
        >
          Español
        </Button>
      </div>
    );
  }

  return (
    <Card className={className}>
      <CardHeader className="pb-0 pt-2 px-4 flex-col items-start">
        <h4 className="font-bold text-large">{t("profile.onboarding.languageSelector.title")}</h4>
      </CardHeader>
      <Divider className="my-2" />
      <CardBody className="py-2 flex gap-2 justify-center">
        <Button
          size="md"
          variant={language === "en" ? "solid" : "light"}
          onPress={() => handleLanguageChange("en")}
          className={language === "en" ? "bg-primary text-white" : ""}
        >
          English
        </Button>
        <Button
          size="md"
          variant={language === "es" ? "solid" : "light"}
          onPress={() => handleLanguageChange("es")}
          className={language === "es" ? "bg-primary text-white" : ""}
        >
          Español
        </Button>
      </CardBody>
    </Card>
  );
};
