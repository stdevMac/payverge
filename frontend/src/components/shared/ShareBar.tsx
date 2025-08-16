"use client";

import { useState, useEffect, useCallback, useRef } from 'react';
import { Button, Tooltip } from "@nextui-org/react";
import { FaShare } from "react-icons/fa";
import { 
  IoGiftOutline, 
  IoCarSportOutline, 
  IoWalletOutline, 
  IoTrendingUpOutline, 
  IoChevronDownOutline, 
  IoChevronUpOutline 
} from "react-icons/io5";
import { useTranslation } from '@/i18n/useTranslation';
import { motion, AnimatePresence } from 'framer-motion';
import { useAccount } from 'wagmi';
import { useUserStore } from '@/store/useUserStore';
import toast from 'react-hot-toast';
import { useShareBarStore } from '@/store/useShareBarStore';

export const ShareBar = () => {
  const { t } = useTranslation();
  const { address, isConnected } = useAccount();
  const user = useUserStore((state) => state.user);
  const [isVisible, setIsVisible] = useState(true);
  const [lastScrollY, setLastScrollY] = useState(0);
  const [scrollTimeout, setScrollTimeout] = useState<NodeJS.Timeout | null>(null);
  const [showCopiedTooltip, setShowCopiedTooltip] = useState(false);
  const { isExpanded, setIsExpanded, setExpandedHeight } = useShareBarStore();
  const expandedContentRef = useRef<HTMLDivElement>(null);
  
  // Toggle expanded state
  const toggleExpanded = () => {
    const newExpandedState = !isExpanded;
    setIsExpanded(newExpandedState);
    
    // Dispatch a custom event when expanded state changes
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('shareBarExpandedChange', { 
        detail: { isExpanded: newExpandedState } 
      }));
    }
  };

  // Handle scroll events
  const handleScroll = useCallback(() => {
    const currentScrollY = window.scrollY;
    
    // Hide the bar when scrolling
    if (currentScrollY !== lastScrollY) {
      // If the bar is expanded, collapse it
      if (isExpanded) {
        setIsExpanded(false);
        
        // Dispatch event to notify language switcher
        if (typeof window !== 'undefined') {
          window.dispatchEvent(new CustomEvent('shareBarExpandedChange', { 
            detail: { isExpanded: false } 
          }));
        }
      }
      
      setIsVisible(false);
      setLastScrollY(currentScrollY);
      
      // Clear any existing timeout
      if (scrollTimeout) {
        clearTimeout(scrollTimeout);
      }
      
      // Set a new timeout to show the bar after scrolling stops
      const timeout = setTimeout(() => {
        setIsVisible(true);
      }, 2000); // 2 seconds after scrolling stops
      
      setScrollTimeout(timeout as unknown as NodeJS.Timeout);
    }
  }, [lastScrollY, scrollTimeout, isExpanded, setIsExpanded]);

  // Set up scroll event listener
  useEffect(() => {
    window.addEventListener('scroll', handleScroll, { passive: true });
    
    return () => {
      window.removeEventListener('scroll', handleScroll);
      if (scrollTimeout) {
        clearTimeout(scrollTimeout);
      }
    };
  }, [handleScroll, scrollTimeout]);
  
  // If user is not connected, don't show the ShareBar
  if (!isConnected || !address || !user) {
    return null;
  }

  // Function to handle sharing
  const handleShare = () => {
    if (user?.referral_code) {
      const referralLink = `${window.location.origin}/referee/${user.referral_code}`;
      navigator.clipboard
        .writeText(referralLink)
        .then(() => {
          toast.success(t("shared.shareBar.linkCopied"), {
            duration: 3000,
            position: "top-right",
            style: {
              background: "#10B981",
              color: "#fff",
            },
          });
          
          // Also show the tooltip for immediate feedback
          setShowCopiedTooltip(true);
          setTimeout(() => {
            setShowCopiedTooltip(false);
          }, 2000);
        })
        .catch((err) => {
          console.error("Could not copy text: ", err);
          toast.error(t("shared.shareBar.copyError"));
        });
    } else {
      // If no referral code, just share the main site
      navigator.clipboard
        .writeText(window.location.origin)
        .then(() => {
          // Show copied tooltip
          setShowCopiedTooltip(true);
          setTimeout(() => {
            setShowCopiedTooltip(false);
          }, 2000);
        })
        .catch((err) => {
          console.error("Could not copy text: ", err);
        });
    }
  };
  


  // Animation variants for Framer Motion
  const shareBarVariants = {
    hidden: { opacity: 0, y: 50 },
    visible: { 
      opacity: 1, 
      y: 0,
      transition: { 
        duration: 0.5, 
        ease: "easeOut" 
      }
    },
    exit: { 
      opacity: 0, 
      y: 50,
      transition: { 
        duration: 0.3, 
        ease: "easeIn" 
      }
    }
  };

  // Animation variants for the expanded content
  const expandedContentVariants = {
    hidden: { opacity: 0, height: 0 },
    visible: { 
      opacity: 1, 
      height: 'auto',
      transition: { 
        duration: 0.3, 
        ease: "easeOut" 
      }
    },
    exit: { 
      opacity: 0, 
      height: 0,
      transition: { 
        duration: 0.2, 
        ease: "easeIn" 
      }
    }
  };

  return (
    <AnimatePresence>
      {isVisible && (
        <motion.div
          initial="hidden"
          animate="visible"
          exit="exit"
          variants={shareBarVariants}
          className="fixed bottom-0 inset-x-0 mx-auto z-50 w-full sm:w-[70%] max-w-[800px] shadow-lg rounded-t-xl"
        >
          <div 
            className="w-full bg-primary-400 px-4 py-3 rounded-t-xl cursor-pointer" 
            onClick={toggleExpanded}
          >
            {/* Main bar content */}
            <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
              <div className="flex items-center gap-2 w-full sm:w-auto">
                <p className="text-white font-medium text-center sm:text-left text-sm sm:text-base flex-1 sm:flex-initial">
                  {t('shared.shareBar.message')}
                </p>
                <div className="text-white ml-auto sm:ml-2 opacity-70">
                  {isExpanded ? 
                    <IoChevronUpOutline className="text-lg" /> : 
                    <IoChevronDownOutline className="text-lg" />
                  }
                </div>
              </div>
              <Tooltip
                content={t('shared.shareBar.copied')}
                isOpen={showCopiedTooltip}
                placement="top"
                color="success"
              >
                <div onClick={(e) => e.stopPropagation()}>
                  <Button
                    color="primary"
                    variant="solid"
                    onPress={handleShare}
                    className="bg-white text-primary-600 hover:bg-primary-50 min-w-[120px] w-full sm:w-auto"
                    startContent={<FaShare className="text-sm" />}
                    size="sm"
                  >
                    {t('shared.shareBar.buttonText')}
                  </Button>
                </div>
              </Tooltip>
            </div>
            
            {/* Expanded content with referral program benefits */}
            <AnimatePresence>
              {isExpanded && (
                <motion.div
                  initial="hidden"
                  animate="visible"
                  exit="exit"
                  variants={expandedContentVariants}
                  className="mt-3 bg-white rounded-xl border border-primary-100 overflow-hidden max-h-[60vh] sm:max-h-none overflow-y-auto shadow-sm"
                  ref={expandedContentRef}
                  onAnimationComplete={() => {
                    // Measure height after animation completes
                    if (expandedContentRef.current) {
                      const height = expandedContentRef.current.offsetHeight;
                      setExpandedHeight(height + 100); // Add some padding
                    }
                  }}
                >
                  <div className="p-4">
                    <div className="flex items-center gap-2 mb-3">
                      <div className="p-2 rounded-lg bg-primary-100 flex-shrink-0">
                        <IoGiftOutline className="text-lg sm:text-xl text-primary-600" />
                      </div>
                      <div>
                        <span className="font-semibold text-primary-700 text-sm sm:text-base">
                          {t('shared.shareBar.benefitsTitle')}
                        </span>
                      </div>
                    </div>
                    
                    <div className="space-y-4 py-2">
                      <div className="space-y-1 sm:space-y-2">
                        <div className="flex items-center gap-2">
                          <IoCarSportOutline className="text-primary flex-shrink-0" />
                          <span className="font-medium text-sm sm:text-base">
                            {t('shared.shareBar.benefits.fleetSpecific.title')}
                          </span>
                        </div>
                        <p className="text-xs sm:text-sm text-default-500 ml-6">
                          {t('shared.shareBar.benefits.fleetSpecific.description')}
                        </p>
                      </div>
                      <div className="space-y-1 sm:space-y-2">
                        <div className="flex items-center gap-2">
                          <IoWalletOutline className="text-primary flex-shrink-0" />
                          <span className="font-medium text-sm sm:text-base">
                            {t('shared.shareBar.benefits.earnings.title')}
                          </span>
                        </div>
                        <p className="text-xs sm:text-sm text-default-500 ml-6">
                          {t('shared.shareBar.benefits.earnings.description')}
                        </p>
                      </div>
                      <div className="space-y-1 sm:space-y-2">
                        <div className="flex items-center gap-2">
                          <IoTrendingUpOutline className="text-primary flex-shrink-0" />
                          <span className="font-medium text-sm sm:text-base">
                            {t('shared.shareBar.benefits.incomeSources.title')}
                          </span>
                        </div>
                        <p className="text-xs sm:text-sm text-default-500 ml-6">
                          {t('shared.shareBar.benefits.incomeSources.description')}
                        </p>
                      </div>
                      <div className="text-xs sm:text-sm text-default-500 space-y-1 bg-gray-50 p-3 rounded-lg mt-3 border border-gray-100">
                        <p className="flex items-start"><span className="mr-2 flex-shrink-0">•</span> {t('shared.shareBar.benefits.bulletPoints.0')}</p>
                        <p className="flex items-start"><span className="mr-2 flex-shrink-0">•</span> {t('shared.shareBar.benefits.bulletPoints.1')}</p>
                        <p className="flex items-start"><span className="mr-2 flex-shrink-0">•</span> {t('shared.shareBar.benefits.bulletPoints.2')}</p>
                        <p className="flex items-start"><span className="mr-2 flex-shrink-0">•</span> {t('shared.shareBar.benefits.bulletPoints.3')}</p>
                      </div>
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
};
