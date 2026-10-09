"use client";

import React, { useState, useEffect, useMemo, useCallback } from "react";
import Image from "next/image";
import { Button } from "@nextui-org/react";
import { ChevronLeft, ChevronRight, Utensils } from "lucide-react";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import ImageLoadingSkeleton, {
  isImageAlreadyLoaded,
} from "@/components/shared/ImageLoadingSkeleton";

interface ImageCarouselProps {
  images: string[];
  itemName: string;
  className?: string;
  size?: "sm" | "md" | "lg";
}

export const ImageCarousel: React.FC<ImageCarouselProps> = ({
  images,
  itemName,
  className = "",
  size = "md",
}) => {
  const { t } = useGuestTranslation();
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const [touchStart, setTouchStart] = useState<number | null>(null);
  const [touchEnd, setTouchEnd] = useState<number | null>(null);
  const [failedImages, setFailedImages] = useState<Set<number>>(new Set());
  // Indices whose bitmap has painted; until then the active frame shows a
  // pulse skeleton instead of a blank bg-warm-100 box.
  const [loadedImages, setLoadedImages] = useState<Set<number>>(new Set());

  // Filter out empty strings - memoized to prevent re-computation
  const validImages = useMemo(() => {
    if (!images || images.length === 0) return [];
    return images.filter((img) => img && img.trim() !== "");
  }, [images]);

  // Reset current image index when images content changes (different item)
  const imagesKey = useMemo(() => {
    return validImages.join("|");
  }, [validImages]);

  useEffect(() => {
    setCurrentImageIndex(0);
    setFailedImages(new Set());
    setLoadedImages(new Set());
  }, [imagesKey]);

  const handleImageError = useCallback((index: number) => {
    setFailedImages((prev) => {
      const next = new Set(prev);
      next.add(index);
      return next;
    });
  }, []);

  const handleImageLoad = useCallback((index: number) => {
    setLoadedImages((prev) => {
      if (prev.has(index)) return prev;
      const next = new Set(prev);
      next.add(index);
      return next;
    });
  }, []);

  // Images that loaded successfully
  const loadableImages = useMemo(
    () => validImages.filter((_, i) => !failedImages.has(i)),
    [validImages, failedImages],
  );

  // The subset of validImages indices that haven't errored. Navigation, the
  // counter, and dots all operate over THIS list so a guest can't swipe or tap
  // to a failed (blank) frame.
  const loadableIndices = useMemo(
    () => validImages.map((_, i) => i).filter((i) => !failedImages.has(i)),
    [validImages, failedImages],
  );

  // If the currently-shown image errors out, snap to the first still-loadable
  // one instead of stranding the guest on a blank frame.
  useEffect(() => {
    if (loadableIndices.length === 0) return;
    if (!loadableIndices.includes(currentImageIndex)) {
      setCurrentImageIndex(loadableIndices[0]);
    }
  }, [loadableIndices, currentImageIndex]);

  // Size configurations
  const sizeConfig = {
    sm: {
      container: "w-20 h-20",
      button: "w-5 h-5",
      icon: "w-3 h-3",
      dot: "w-1.5 h-1.5",
    },
    md: {
      container: "w-28 h-28",
      button: "w-6 h-6",
      icon: "w-4 h-4",
      dot: "w-2 h-2",
    },
    lg: {
      container: "w-full aspect-square max-w-lg",
      button: "w-8 h-8",
      icon: "w-5 h-5",
      dot: "w-2.5 h-2.5",
    },
  };

  const config = sizeConfig[size];

  // Only mount the active image and its immediate loadable neighbors (prev/next)
  // rather than every image. Cuts DOM nodes and network fetches to at most 3
  // <Image> elements while preserving swipe/transition behavior (the outgoing
  // neighbor stays mounted long enough to fade out). Keyed by validImages index
  // so opacity toggling and the transition still animate the same nodes.
  const mountedIndices = useMemo(() => {
    if (loadableIndices.length === 0) return new Set<number>();
    const pos = Math.max(0, loadableIndices.indexOf(currentImageIndex));
    const len = loadableIndices.length;
    const window = new Set<number>();
    window.add(loadableIndices[pos]);
    window.add(loadableIndices[(pos - 1 + len) % len]);
    window.add(loadableIndices[(pos + 1) % len]);
    return window;
  }, [loadableIndices, currentImageIndex]);

  // If no images, show a styled placeholder
  if (validImages.length === 0) {
    return (
      <div className={`flex-shrink-0 ${className}`}>
        <div
          className={`${config.container} rounded-xl overflow-hidden border border-gray-100 shadow-sm bg-warm-100 flex items-center justify-center`}
        >
          <Utensils className="w-1/4 h-1/4 text-gray-400" />
        </div>
      </div>
    );
  }

  // All images failed — show a placeholder
  if (loadableImages.length === 0) {
    return (
      <div className={`flex-shrink-0 ${className}`}>
        <div
          className={`${config.container} rounded-xl overflow-hidden border border-gray-100 shadow-sm bg-warm-100 flex items-center justify-center`}
        >
          <Utensils className="w-1/4 h-1/4 text-gray-400" />
        </div>
      </div>
    );
  }

  const nextImage = () => {
    if (loadableIndices.length <= 1) return;
    const pos = Math.max(0, loadableIndices.indexOf(currentImageIndex));
    setCurrentImageIndex(loadableIndices[(pos + 1) % loadableIndices.length]);
  };

  const prevImage = () => {
    if (loadableIndices.length <= 1) return;
    const pos = Math.max(0, loadableIndices.indexOf(currentImageIndex));
    setCurrentImageIndex(
      loadableIndices[
        (pos - 1 + loadableIndices.length) % loadableIndices.length
      ],
    );
  };

  const goToImage = (index: number) => {
    if (index === currentImageIndex || failedImages.has(index)) return;
    setCurrentImageIndex(index);
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

    if (isLeftSwipe && loadableIndices.length > 1) {
      nextImage();
    }
    if (isRightSwipe && loadableIndices.length > 1) {
      prevImage();
    }
  };

  return (
    <div className={`flex-shrink-0 ${className}`}>
      <div
        className={`${config.container} rounded-xl overflow-hidden border border-gray-100 shadow-sm relative group`}
        onTouchStart={onTouchStart}
        onTouchMove={onTouchMove}
        onTouchEnd={onTouchEnd}
      >
        <div className="relative w-full h-full overflow-hidden">
          {validImages.map((imgSrc, index) =>
            failedImages.has(index) || !mountedIndices.has(index) ? null : (
              <Image
                key={index}
                src={imgSrc}
                alt={t("menu.itemImageAlt", {
                  name: itemName,
                  index: index + 1,
                })}
                fill
                sizes={
                  size === "lg" ? "(max-width: 1024px) 100vw, 32rem" : "7rem"
                }
                unoptimized={!canOptimizeImageSrc(imgSrc)}
                className={`absolute inset-0 w-full h-full object-cover transition-opacity duration-300 ease-out ${
                  index === currentImageIndex
                    ? "opacity-100 z-[1]"
                    : "opacity-0 z-0"
                }`}
                aria-hidden={index === currentImageIndex ? undefined : true}
                draggable={false}
                ref={(node) => {
                  // Cached images can be complete before onLoad is wired —
                  // mark them immediately so there is no skeleton flash.
                  if (isImageAlreadyLoaded(node)) handleImageLoad(index);
                }}
                onLoad={() => handleImageLoad(index)}
                onError={() => handleImageError(index)}
              />
            ),
          )}
          {/* Pulse skeleton while the active frame's image is in flight.
              Failed frames fall through to the utensils placeholder above,
              never a stuck skeleton. */}
          {!failedImages.has(currentImageIndex) &&
            !loadedImages.has(currentImageIndex) && (
              <ImageLoadingSkeleton className="z-[2] bg-warm-100" />
            )}
        </div>

        {/* Navigation arrows - only show if multiple loadable images */}
        {loadableIndices.length > 1 && (
          <>
            <Button
              isIconOnly
              size="sm"
              variant="flat"
              aria-label={t("accessibility.previousImage")}
              className={`absolute start-2 top-1/2 -translate-y-1/2 ${config.button} bg-black/60 backdrop-blur-sm ${size === "lg" ? "opacity-90 hover:opacity-100" : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100"} transition-all duration-200 min-w-0 z-10 hover:scale-110 active:scale-95`}
              onClick={(e) => {
                e.stopPropagation();
                prevImage();
              }}
            >
              <ChevronLeft
                className={`${config.icon} text-white rtl:rotate-180`}
              />
            </Button>

            <Button
              isIconOnly
              size="sm"
              variant="flat"
              aria-label={t("accessibility.nextImage")}
              className={`absolute end-2 top-1/2 -translate-y-1/2 ${config.button} bg-black/60 backdrop-blur-sm ${size === "lg" ? "opacity-90 hover:opacity-100" : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100"} transition-all duration-200 min-w-0 z-10 hover:scale-110 active:scale-95`}
              onClick={(e) => {
                e.stopPropagation();
                nextImage();
              }}
            >
              <ChevronRight
                className={`${config.icon} text-white rtl:rotate-180`}
              />
            </Button>
          </>
        )}

        {/* Image counter */}
        {loadableIndices.length > 1 && (
          <div className="absolute end-2 top-2 bg-black/70 backdrop-blur-sm text-white text-xs px-2 py-1 rounded-full z-10 font-medium">
            {Math.max(0, loadableIndices.indexOf(currentImageIndex)) + 1}/
            {loadableIndices.length}
          </div>
        )}

        {/* Swipe hint for mobile (only show for lg size) */}
        {loadableIndices.length > 1 && size === "lg" && (
          <div className="absolute bottom-2 start-2 bg-black/60 backdrop-blur-sm text-white text-xs px-2 py-1 rounded-full opacity-80 z-10">
            {t("menu.swipeToBrowse")}
          </div>
        )}

        {/* Dot indicators - only show if multiple loadable images */}
        {loadableIndices.length > 1 && loadableIndices.length <= 5 && (
          <div className="absolute bottom-2 left-1/2 -translate-x-1/2 flex gap-1">
            {loadableIndices.map((imgIndex, dotPos) => (
              <button
                key={imgIndex}
                type="button"
                aria-label={t("accessibility.goToImage", { index: dotPos + 1 })}
                aria-current={imgIndex === currentImageIndex}
                className={`${config.dot} rounded-full transition-all duration-300 ${
                  imgIndex === currentImageIndex
                    ? "bg-white scale-125"
                    : "bg-white/50 hover:bg-white/75 hover:scale-110"
                }`}
                onClick={(e) => {
                  e.stopPropagation();
                  goToImage(imgIndex);
                }}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
