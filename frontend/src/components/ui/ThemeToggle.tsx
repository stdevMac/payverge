"use client";

import React from 'react';
import { Button } from '@nextui-org/react';
import { useTheme } from '@/context/ThemeContext';
import { 
  HiSun, 
} from 'react-icons/hi';

export function ThemeToggle() {
  const { theme } = useTheme();

  const themes = [
    { value: 'light', icon: HiSun, label: 'Light' },
  ] as const;

  const currentTheme = themes.find(t => t.value === theme) || themes[0];

  // No cycling needed since we only have light theme
  const handleClick = () => {
    // Theme is always light, no action needed
  };

  return (
    <Button
      isIconOnly
      variant="ghost"
      size="sm"
      onClick={handleClick}
      className="transition-all duration-200 hover:scale-105"
      aria-label="Light theme"
    >
      <currentTheme.icon className="h-5 w-5" />
    </Button>
  );
}

export function ThemeDropdown() {
  const { theme } = useTheme();

  const themes = [
    { value: 'light', icon: HiSun, label: 'Light' },
  ] as const;

  return (
    <div className="flex flex-col gap-1 p-1">
      {themes.map((themeOption) => {
        const Icon = themeOption.icon;
        return (
          <Button
            key={themeOption.value}
            variant={theme === themeOption.value ? "flat" : "light"}
            size="sm"
            onClick={() => {}} // No action needed for light-only theme
            className="justify-start gap-2 w-full"
          >
            <Icon className="h-4 w-4" />
            {themeOption.label}
          </Button>
        );
      })}
    </div>
  );
}
