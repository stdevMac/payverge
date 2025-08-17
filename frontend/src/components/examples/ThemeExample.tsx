"use client";

import React from 'react';
import { Card, CardBody, CardHeader, Button, Chip } from '@nextui-org/react';
import { useTheme } from '@/context/ThemeContext';
import { ThemeToggle, ThemeDropdown } from '@/components/ui/ThemeToggle';

export function ThemeExample() {
  const { theme, resolvedTheme } = useTheme();

  return (
    <div className="max-w-4xl mx-auto p-6 space-y-6">
      <Card className="bg-white dark:bg-gray-800 transition-colors duration-200">
        <CardHeader>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">
            Theme System Demo
          </h2>
        </CardHeader>
        <CardBody className="space-y-4">
          <div className="flex items-center gap-4">
            <span className="text-gray-700 dark:text-gray-300">Current Theme:</span>
            <Chip color="primary" variant="flat">
              {theme} ({resolvedTheme})
            </Chip>
          </div>
          
          <div className="flex items-center gap-4">
            <span className="text-gray-700 dark:text-gray-300">Quick Toggle:</span>
            <ThemeToggle />
          </div>
          
          <div className="space-y-2">
            <span className="text-gray-700 dark:text-gray-300">Theme Options:</span>
            <ThemeDropdown />
          </div>
        </CardBody>
      </Card>

      <Card className="bg-gradient-to-r from-blue-50 to-indigo-50 dark:from-gray-800 dark:to-gray-700 transition-colors duration-200">
        <CardHeader>
          <h3 className="text-xl font-semibold text-gray-900 dark:text-white">
            Theme-Aware Components
          </h3>
        </CardHeader>
        <CardBody className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Button color="primary" variant="solid">
              Primary Button
            </Button>
            <Button color="secondary" variant="flat">
              Secondary Button
            </Button>
            <Button color="success" variant="bordered">
              Success Button
            </Button>
            <Button color="warning" variant="light">
              Warning Button
            </Button>
          </div>
          
          <div className="p-4 rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-600">
            <p className="text-gray-700 dark:text-gray-300">
              This content automatically adapts to the current theme. The background, 
              text colors, and borders all change seamlessly when switching between 
              light and dark modes.
            </p>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
