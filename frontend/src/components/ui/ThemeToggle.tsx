"use client";

import React from 'react';
import { Button } from '@nextui-org/react';
import { useTheme } from '@/context/ThemeContext';
import { 
  HiSun, 
  HiMoon, 
} from 'react-icons/hi';

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  const themes = [
    { value: 'light', icon: HiSun, label: 'Light' },
    { value: 'dark', icon: HiMoon, label: 'Dark' },
  ] as const;

  const currentTheme = themes.find(t => t.value === theme) || themes[0];

  const cycleTheme = () => {
    const currentIndex = themes.findIndex(t => t.value === theme);
    const nextIndex = (currentIndex + 1) % themes.length;
    setTheme(themes[nextIndex].value);
  };

  return (
    <Button
      isIconOnly
      variant="ghost"
      size="sm"
      onClick={cycleTheme}
      className="transition-all duration-200 hover:scale-105"
      aria-label={`Switch to ${themes[(themes.findIndex(t => t.value === theme) + 1) % themes.length].label} theme`}
    >
      <currentTheme.icon className="h-5 w-5" />
    </Button>
  );
}

export function ThemeDropdown() {
  const { theme, setTheme } = useTheme();

  const themes = [
    { value: 'light', icon: HiSun, label: 'Light' },
    { value: 'dark', icon: HiMoon, label: 'Dark' },
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
            onClick={() => setTheme(themeOption.value)}
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
