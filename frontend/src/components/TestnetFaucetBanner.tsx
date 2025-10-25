"use client";
import { Card, CardBody } from "@nextui-org/react";
import { useState, useEffect } from 'react';
import { useAccount, useChainId } from 'wagmi';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface Props {
  onFaucetClick: () => void;
}

export const TestnetFaucetBanner = ({ onFaucetClick }: Props) => {
  // All hooks must be called before any conditional logic
  const [isHovered, setIsHovered] = useState(false);
  const [currentIndex, setCurrentIndex] = useState(0);
  const [prevIndex, setPrevIndex] = useState(0);
  const { address, isConnected } = useAccount();
  const chainId = useChainId();
  const { locale } = useSimpleLocale();
  
  // Translation helper function
  const t = (key: string) => getTranslation(key, locale) as string;

  const messages = [
    {
      text: "🚀 Testing on Base Sepolia?",
      highlight: "Get FREE testnet tokens!"
    },
    {
      text: "💰 Need ETH & USDC for testing?",
      highlight: "Claim your tokens now!"
    },
    {
      text: "⚡ Try Payverge risk-free",
      highlight: "Get testnet funds instantly!"
    }
  ];

  useEffect(() => {
    const interval = setInterval(() => {
      setPrevIndex(currentIndex);
      setCurrentIndex((prevIndex) => (prevIndex + 1) % messages.length);
    }, 4000);

    return () => clearInterval(interval);
  }, [currentIndex, messages.length]);

  const renderMessage = (index: number, isLeaving: boolean) => {
    const messageToShow = messages[index % messages.length];
    return (
      <div
        key={`${index}-${isLeaving ? "leaving" : "entering"}-${messageToShow.text}`}
        className={`absolute w-full h-full flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-2 pointer-events-none ${
          isLeaving ? "animate-slideToRight" : "animate-slideFromLeft"
        }`}
      >
        <span className="text-center sm:text-right text-white font-semibold text-sm sm:text-base whitespace-nowrap leading-none">
          {messageToShow.text}
        </span>
        <span className="text-white font-bold text-sm sm:text-base whitespace-nowrap leading-none">
          {messageToShow.highlight}
        </span>
      </div>
    );
  };

  // Base Sepolia chain ID
  const BASE_SEPOLIA_CHAIN_ID = 84532;

  // Only show on Base Sepolia testnet
  if (chainId !== BASE_SEPOLIA_CHAIN_ID) {
    return null;
  }

  return (
    <div className="w-full bg-gradient-to-r from-blue-600 via-purple-600 to-indigo-600 relative overflow-hidden">
      {/* Animated background elements */}
      <div className="absolute inset-0 bg-[radial-gradient(circle,rgba(255,255,255,0.1)_1px,transparent_1px)] bg-[length:20px_20px] animate-pulse" />
      <div className="absolute top-0 left-1/4 w-32 h-32 bg-white/5 rounded-full blur-xl animate-pulse" />
      <div className="absolute bottom-0 right-1/4 w-24 h-24 bg-white/5 rounded-full blur-xl animate-pulse" style={{ animationDelay: '1s' }} />
      
      <div className="max-w-6xl mx-auto px-4 py-3">
        <div
          className={`relative cursor-pointer transition-all duration-300 rounded-xl overflow-hidden ${
            isHovered ? 'transform scale-[1.02]' : ''
          }`}
          onClick={onFaucetClick}
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
        >
          {/* Shine effect */}
          <div className={`absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent transform -skew-x-12 transition-transform duration-1000 ${
            isHovered ? 'translate-x-full' : '-translate-x-full'
          }`} />
          
          <div className="relative bg-white/10 backdrop-blur-sm border border-white/20 rounded-xl p-4">
            <div className="flex items-center justify-between">
              {/* Left side - Icon and message */}
              <div className="flex items-center gap-4">
                <div className="flex-shrink-0 w-12 h-12 bg-white/20 rounded-full flex items-center justify-center">
                  <div className="text-2xl">🚰</div>
                </div>
                <div className="text-left">
                  <div className="text-white font-semibold text-lg leading-tight">
                    {t('faucet.banner.needTokens')}
                  </div>
                  <div className="text-white/80 text-sm font-light">
                    {t('faucet.banner.description')}
                  </div>
                </div>
              </div>
              
              {/* Right side - CTA and network indicator */}
              <div className="flex items-center gap-4">
                <div className="hidden sm:flex items-center gap-2 text-white/70 text-xs">
                  <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
                  <span>{t('faucet.banner.networkIndicator')}</span>
                </div>
                <div className="bg-white/20 hover:bg-white/30 transition-colors duration-200 rounded-lg px-4 py-2 border border-white/30">
                  <span className="text-white font-medium text-sm">{t('faucet.banner.claimButton')}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
