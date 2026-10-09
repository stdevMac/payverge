import React from "react";

interface SectionHeaderProps {
  title: string;
  subtitle?: string;
  badge?: string;
  designSettings: {
    primary_color: string;
    corner_radius?: string;
  };
  centered?: boolean;
  as?: "h1" | "h2" | "h3";
}

const SectionHeader = ({
  title,
  subtitle,
  badge,
  designSettings,
  centered = true,
  as: Heading = "h2",
}: SectionHeaderProps) => {
  const align = centered ? "items-center text-center" : "items-start text-start";

  return (
    <header className={`mb-10 md:mb-14 flex flex-col ${align} gap-4`}>
      {badge && (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full border border-gray-200 bg-white/60 backdrop-blur-sm text-xs font-semibold uppercase tracking-[0.2em] text-gray-600">
          <span
            aria-hidden
            className="w-1.5 h-1.5 rounded-full"
            style={{ backgroundColor: designSettings.primary_color }}
          />
          {badge}
        </span>
      )}
      <Heading className="font-title text-3xl md:text-4xl lg:text-5xl text-gray-900 tracking-tight max-w-3xl leading-tight">
        {title}
      </Heading>
      {subtitle && (
        <p className="text-base md:text-lg text-gray-500 leading-relaxed max-w-2xl">
          {subtitle}
        </p>
      )}
    </header>
  );
};

export default SectionHeader;
