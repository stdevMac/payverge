"use client";
import React from "react";
import Image from "next/image";
import Link from "next/link";
import {useTranslation} from "@/i18n/useTranslation";

export const Footer = () => {
    const {t} = useTranslation();
    const currentYear = new Date().getFullYear();

    const scrollToTop = () => {
        window.scrollTo({
            top: 0,
            behavior: "smooth",
        });
    };

    return (
        <footer className="bg-gray-50 w-full relative">
            {/* Back to Top Button */}
            <button
                onClick={scrollToTop}
                className="absolute -top-6 right-8 bg-primary hover:bg-primary/90 text-white p-3 rounded-full shadow-lg transform hover:scale-110 transition-all duration-300 group"
                aria-label={t('footer.backToTop')}
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
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8 py-12 border-b border-gray-200">
                    {/* Logo Section */}
                    <div className="flex flex-col items-center lg:items-start space-y-4">
                        <Image
                            src="/footer-icons/Web3BoilerplateLogo-01.svg"
                            alt="logo"
                            width={160}
                            height={80}
                            className="w-auto h-auto max-w-[160px]"
                        />
                        <p className="text-sm text-gray-600 text-center lg:text-left">
                            {t('footer.tagline')}
                        </p>
                    </div>

                    {/* About Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{t('footer.about')}</h2>
                        <ul className="space-y-2 text-sm text-gray-600">
                            <li>
                                <Link
                                    href="/mission"
                                    className="hover:text-primary transition-colors"
                                >
                                    {t('footer.aboutLinks.mission')}
                                </Link>
                            </li>
                            <li>
                                <Link
                                    href="/how-it-works"
                                    className="hover:text-primary transition-colors"
                                >
                                    {t('footer.aboutLinks.howItWorks')}
                                </Link>
                            </li>
                        </ul>
                    </div>

                    {/* Resources Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{t('footer.resources')}</h2>
                        <ul className="space-y-2 text-sm text-gray-600">
                            <li>
                                <a
                                    href="https://docs.yourapp.com"
                                    className="hover:text-primary transition-colors"
                                >
                                    {t('footer.resourceLinks.documentation')}
                                </a>
                            </li>
                            <li>
                                <Link
                                    href="/terms"
                                    className="hover:text-primary transition-colors"
                                >
                                    {t('footer.resourceLinks.terms')}
                                </Link>
                            </li>
                            <li>
                                <Link
                                    href="/privacy"
                                    className="hover:text-primary transition-colors"
                                >
                                    {t('footer.resourceLinks.privacy')}
                                </Link>
                            </li>
                        </ul>
                    </div>

                    {/* Contact Section */}
                    <div className="flex flex-col items-center lg:items-start">
                        <h2 className="text-primary font-bold text-lg mb-4">{t('footer.contact')}</h2>
                        <div className="space-y-3 text-sm text-gray-600 flex flex-col lg:items-start items-center">
                            <div className="flex items-center space-x-2">
                                <Image
                                    src="/footer-icons/icons8mail114429-c29d.svg"
                                    alt="mail"
                                    width={18}
                                    height={18}
                                />
                                <span>
                  <a href="mailto:info@yourapp.com" className="underline">
                    info@yourapp.com
                  </a>
                </span>
                            </div>
                            <div className="flex items-center space-x-2">
                                <Image
                                    src="/footer-icons/icons8phone14429-i8k.svg"
                                    alt="phone"
                                    width={18}
                                    height={18}
                                />
                                <span>
                  <a href="tel:+971585637448" className="underline">
                    +971 58 563 7448
                  </a>
                </span>
                            </div>
                            <div className="flex items-center space-x-2">
                                <Image
                                    src="/footer-icons/icons8mappin114429-66c5.svg"
                                    alt="mappin"
                                    width={18}
                                    height={18}
                                />
                                <span className="text-center lg:text-left">
                  <a
                      href="https://maps.app.goo.gl/RXJUHySPEvTpZgCz6"
                      className="underline"
                  >
                    5 Stars Business Center, <br/>
                    4th Floor, office 75, Dubai, <br/>
                    UAE
                  </a>
                </span>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Social Links */}
                <div className="py-8 border-b border-gray-200">
                    <div className="flex justify-center space-x-6">
                        <a
                            href={`https://wa.me/+971581529469?text=Hi%20Token%20Fleet%20Team!%20I%20need%20some%20assistance%20regarding%20the%20platform.%20Could%20you%20please%20help%20me%20with%20my%20inquiry%3F`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/whatsapp.png"
                                alt="WhatsApp"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://t.me/yourapp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/telegram.png"
                                alt="Telegram"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://www.facebook.com/profile.php?id=61568084092915"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/facebook.png"
                                alt="Facebook"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://x.com/yourapp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/x.png"
                                alt="Twitter"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://www.youtube.com/@YourApp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/youtube.png"
                                alt="YouTube"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://www.instagram.com/yourapp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/instagram.png"
                                alt="Instagram"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://www.linkedin.com/company/yourapp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/linkedin.png"
                                alt="LinkedIn"
                                width={20}
                                height={20}
                            />
                        </a>
                        <a
                            href="https://www.tiktok.com/@yourapp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="transform hover:rotate-12 transition-all duration-300 ease-in-out hover:scale-110"
                        >
                            <Image
                                src="/footer-icons/tiktok.png"
                                alt="TikTok"
                                width={20}
                                height={20}
                            />
                        </a>
                    </div>
                </div>

                {/* Copyright */}
                <div className="py-6 text-center text-sm text-gray-600">
                    <p>
                        &copy; {currentYear} Token Fleet, {t('footer.companyBy')}
                        <span className="block lg:inline"> {t('footer.allRightsReserved')}</span>
                    </p>
                </div>
            </div>
        </footer>
    );
};
