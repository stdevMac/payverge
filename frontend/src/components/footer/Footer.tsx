"use client";
import React, { useState, useEffect } from "react";
import Image from "next/image";
import Link from "next/link";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export const Footer = () => {
    const { locale } = useSimpleLocale();
    const [currentLocale, setCurrentLocale] = useState(locale);
    
    // Update translations when locale changes
    useEffect(() => {
        setCurrentLocale(locale);
    }, [locale]);
    
    // Translation helper
    const tString = (key: string): string => {
        const fullKey = `footer.${key}`;
        const result = getTranslation(fullKey, currentLocale);
        return Array.isArray(result) ? result[0] || key : result as string;
    };
    const currentYear = new Date().getFullYear();

    const scrollToTop = () => {
        window.scrollTo({
            top: 0,
            behavior: "smooth",
        });
    };

    return (
        <footer className="bg-gray-50 dark:bg-gray-900 w-full relative transition-colors duration-200">
            {/* Back to Top Button */}
            <button
                onClick={scrollToTop}
                className="absolute -top-6 right-8 bg-primary hover:bg-primary/90 text-white p-3 rounded-full shadow-lg transform hover:scale-110 transition-all duration-300 group"
                aria-label={tString('backToTop')}
            >
                <svg
                    xmlns="http://www.w3.org/2000/svg"
                    className="h-6 w-6 transform transition-transform group-hover:-translate-y-1 duration-300"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                >
                    <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M5 10l7-7m0 0l7 7m-7-7v18"
                    />
                </svg>
            </button>

            <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
                {/* Main Footer Content */}
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8 py-12 border-b border-gray-200 dark:border-gray-700">
                    {/* Logo Section */}
                    <div className="flex flex-col items-center lg:items-start space-y-4">
                        <Image
                            src="/footer-icons/Web3BoilerplateLogo-01.svg"
                            alt="logo"
                            width={160}
                            height={80}
                            className="w-auto h-auto max-w-[160px]"
                        />
                        <p className="text-sm text-gray-600 dark:text-gray-300 text-center lg:text-left">
                            {tString('tagline')}
                        </p>
                    </div>

                    {/* About Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{tString('about')}</h2>
                        <ul className="space-y-2 text-sm text-gray-600 dark:text-gray-300">
                            <li>
                                <Link
                                    href="/mission"
                                    className="hover:text-primary transition-colors"
                                >
                                    {tString('aboutLinks.mission')}
                                </Link>
                            </li>
                            <li>
                                <Link
                                    href="/how-it-works"
                                    className="hover:text-primary transition-colors"
                                >
                                    {tString('aboutLinks.howItWorks')}
                                </Link>
                            </li>
                        </ul>
                    </div>

                    {/* Resources Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{tString('resources')}</h2>
                        <ul className="space-y-2 text-sm text-gray-600 dark:text-gray-300">
                            <li>
                                <a
                                    href="https://docs.yourapp.com"
                                    className="hover:text-primary transition-colors"
                                >
                                    {tString('resourceLinks.documentation')}
                                </a>
                            </li>
                            <li>
                                <Link
                                    href="/terms"
                                    className="hover:text-primary transition-colors"
                                >
                                    {tString('resourceLinks.terms')}
                                </Link>
                            </li>
                            <li>
                                <Link
                                    href="/privacy"
                                    className="hover:text-primary transition-colors"
                                >
                                    {tString('resourceLinks.privacy')}
                                </Link>
                            </li>
                        </ul>
                    </div>

                    {/* Contact Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{tString('contact')}</h2>
                        <div className="space-y-3 text-sm text-gray-600 dark:text-gray-300 flex flex-col lg:items-start items-center">
                            <div className="flex items-center space-x-2">
                                <Image
                                    src="/footer-icons/icons8mail114429-c29d.svg"
                                    alt="mail"
                                    width={18}
                                    height={18}
                                />
                                <span>
                  <a href="mailto:info@blockvantage.tech" className="underline">
                    info@blockvantage.tech
                  </a>
                </span>
                            </div>
                        </div>
                    </div>
                </div>
                {/* Copyright */}
                <div className="py-6 text-center text-sm text-gray-600 dark:text-gray-300">
                    <p>
                        &copy; {currentYear} {tString('companyBy')}
                        <span className="block lg:inline"> {tString('allRightsReserved')}</span>
                    </p>
                </div>
            </div>
        </footer>
    );
};
