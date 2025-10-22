'use client';

import React, { useState } from 'react';
import { Image, Button } from '@nextui-org/react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useGuestTranslation } from '../../i18n/GuestTranslationProvider';

interface ImageCarouselProps {
  images: string[];
  itemName: string;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

export const ImageCarousel: React.FC<ImageCarouselProps> = ({
  images,
  itemName,
  className = '',
  size = 'md'
}) => {
  const { t } = useGuestTranslation();
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const [touchStart, setTouchStart] = useState<number | null>(null);
  const [touchEnd, setTouchEnd] = useState<number | null>(null);
  const [isTransitioning, setIsTransitioning] = useState(false);
  const [slideDirection, setSlideDirection] = useState<'left' | 'right' | null>(null);

  // If no images, return null
  if (!images || images.length === 0) {
    return null;
  }

  // Filter out empty strings
  const validImages = images.filter(img => img && img.trim() !== '');
  if (validImages.length === 0) {
    return null;
  }

  const nextImage = () => {
    if (isTransitioning) return;
    setIsTransitioning(true);
    setSlideDirection('left');
    
    setTimeout(() => {
      setCurrentImageIndex((prev) => (prev + 1) % validImages.length);
      setTimeout(() => {
        setIsTransitioning(false);
        setSlideDirection(null);
      }, 50);
    }, 150);
  };

  const prevImage = () => {
    if (isTransitioning) return;
    setIsTransitioning(true);
    setSlideDirection('right');
    
    setTimeout(() => {
      setCurrentImageIndex((prev) => (prev - 1 + validImages.length) % validImages.length);
      setTimeout(() => {
        setIsTransitioning(false);
        setSlideDirection(null);
      }, 50);
    }, 150);
  };

  const goToImage = (index: number) => {
    if (isTransitioning || index === currentImageIndex) return;
    
    setIsTransitioning(true);
    setSlideDirection(index > currentImageIndex ? 'left' : 'right');
    
    setTimeout(() => {
      setCurrentImageIndex(index);
      setTimeout(() => {
        setIsTransitioning(false);
        setSlideDirection(null);
      }, 50);
    }, 150);
  };

  // Touch handlers for swipe navigation
  const minSwipeDistance = 50;

  const onTouchStart = (e: React.TouchEvent) => {
    setTouchEnd(null);
    setTouchStart(e.targetTouches[0].clientX);
  };

  const onTouchMove = (e: React.TouchEvent) => {
    setTouchEnd(e.targetTouches[0].clientX);
  };

  const onTouchEnd = () => {
    if (!touchStart || !touchEnd) return;
    
    const distance = touchStart - touchEnd;
    const isLeftSwipe = distance > minSwipeDistance;
    const isRightSwipe = distance < -minSwipeDistance;

    if (isLeftSwipe && validImages.length > 1) {
      nextImage();
    }
    if (isRightSwipe && validImages.length > 1) {
      prevImage();
    }
  };

  // Size configurations
  const sizeConfig = {
    sm: {
      container: 'w-20 h-20',
      button: 'w-5 h-5',
      icon: 'w-3 h-3',
      dot: 'w-1.5 h-1.5'
    },
    md: {
      container: 'w-28 h-28',
      button: 'w-6 h-6',
      icon: 'w-4 h-4',
      dot: 'w-2 h-2'
    },
    lg: {
      container: 'w-full aspect-square max-w-lg',
      button: 'w-8 h-8',
      icon: 'w-5 h-5',
      dot: 'w-2.5 h-2.5'
    }
  };

  const config = sizeConfig[size];

  return (
    <div className={`flex-shrink-0 ${className}`}>
      <div 
        className={`${config.container} rounded-xl overflow-hidden border border-gray-100 shadow-sm relative group`}
        onTouchStart={onTouchStart}
        onTouchMove={onTouchMove}
        onTouchEnd={onTouchEnd}
      >
        <div className="relative w-full h-full overflow-hidden">
          <Image
            src={validImages[currentImageIndex]}
            alt={`${itemName} - Image ${currentImageIndex + 1}`}
            className={`w-full h-full object-cover transition-all duration-300 ease-out ${
              isTransitioning
                ? slideDirection === 'left'
                  ? 'transform translate-x-full opacity-0'
                  : 'transform -translate-x-full opacity-0'
                : 'transform translate-x-0 opacity-100'
            }`}
          />
          
          {/* Previous image for smooth transition */}
          {isTransitioning && slideDirection && (
            <Image
              src={validImages[slideDirection === 'left' 
                ? (currentImageIndex - 1 + validImages.length) % validImages.length
                : (currentImageIndex + 1) % validImages.length
              ]}
              alt="Previous image"
              className={`absolute inset-0 w-full h-full object-cover transition-all duration-300 ease-out ${
                slideDirection === 'left'
                  ? 'transform -translate-x-full opacity-100'
                  : 'transform translate-x-full opacity-100'
              }`}
            />
          )}
        </div>
        
        {/* Navigation arrows - only show if multiple images */}
        {validImages.length > 1 && (
          <>
            <Button
              isIconOnly
              size="sm"
              variant="flat"
              className={`absolute left-2 top-1/2 -translate-y-1/2 ${config.button} bg-black/60 backdrop-blur-sm ${size === 'lg' ? 'opacity-90 hover:opacity-100' : 'opacity-0 group-hover:opacity-100'} transition-all duration-200 min-w-0 z-10 hover:scale-110 active:scale-95`}
              onClick={(e) => {
                e.stopPropagation();
                prevImage();
              }}
            >
              <ChevronLeft className={`${config.icon} text-white`} />
            </Button>
            
            <Button
              isIconOnly
              size="sm"
              variant="flat"
              className={`absolute right-2 top-1/2 -translate-y-1/2 ${config.button} bg-black/60 backdrop-blur-sm ${size === 'lg' ? 'opacity-90 hover:opacity-100' : 'opacity-0 group-hover:opacity-100'} transition-all duration-200 min-w-0 z-10 hover:scale-110 active:scale-95`}
              onClick={(e) => {
                e.stopPropagation();
                nextImage();
              }}
            >
              <ChevronRight className={`${config.icon} text-white`} />
            </Button>
          </>
        )}

        {/* Image counter */}
        {validImages.length > 1 && (
          <div className="absolute top-2 right-2 bg-black/70 backdrop-blur-sm text-white text-xs px-2 py-1 rounded-full z-10 font-medium">
            {currentImageIndex + 1}/{validImages.length}
          </div>
        )}

        {/* Swipe hint for mobile (only show for lg size) */}
        {validImages.length > 1 && size === 'lg' && (
          <div className="absolute bottom-2 left-2 bg-black/60 backdrop-blur-sm text-white text-xs px-2 py-1 rounded-full opacity-80 z-10">
{t('menu.swipeToBrowse')}
          </div>
        )}

        {/* Dot indicators - only show if multiple images and size is md or lg */}
        {validImages.length > 1 && validImages.length <= 5 && (size === 'md' || size === 'lg') && (
          <div className="absolute bottom-2 left-1/2 -translate-x-1/2 flex gap-1">
            {validImages.map((_, index) => (
              <button
                key={index}
                className={`${config.dot} rounded-full transition-all duration-300 ${
                  index === currentImageIndex 
                    ? 'bg-white scale-125' 
                    : 'bg-white/50 hover:bg-white/75 hover:scale-110'
                }`}
                onClick={(e) => {
                  e.stopPropagation();
                  goToImage(index);
                }}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
