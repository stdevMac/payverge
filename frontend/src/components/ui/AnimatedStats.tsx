"use client";
import { useState, useEffect } from "react";

interface AnimatedStatsProps {
  stats: Array<{
    value: string;
    label: string;
    suffix?: string;
  }>;
}

export const AnimatedStats = ({ stats }: AnimatedStatsProps) => {
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    // Trigger animation after component mounts
    const timer = setTimeout(() => setIsVisible(true), 500);
    return () => clearTimeout(timer);
  }, []);

  return (
    <div className="flex flex-wrap justify-center gap-8 mt-12">
      {stats.map((stat, index) => (
        <div
          key={index}
          className={`text-center transform transition-all duration-1000 ${
            isVisible ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'
          }`}
          style={{ transitionDelay: `${index * 200}ms` }}
        >
          <div className="text-2xl lg:text-3xl font-light text-gray-900 mb-1">
            {stat.value}
            {stat.suffix && <span className="text-lg text-gray-600">{stat.suffix}</span>}
          </div>
          <div className="text-sm text-gray-600 font-medium">{stat.label}</div>
        </div>
      ))}
    </div>
  );
};
