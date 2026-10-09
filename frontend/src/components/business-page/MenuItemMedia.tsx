import React, { useCallback, useState } from "react";
import NextImage from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import MenuItemNoMediaHeader from "./MenuItemNoMediaHeader";
import ImageLoadingSkeleton, {
  isImageAlreadyLoaded,
} from "@/components/shared/ImageLoadingSkeleton";

interface MenuItemMediaProps {
  images: string[];
  alt: string;
  noMediaLabel?: string;
}

export default function MenuItemMedia({ images, alt, noMediaLabel = "No image" }: MenuItemMediaProps) {
  const [errored, setErrored] = useState<Record<number, boolean>>({});
  const [index, setIndex] = useState(0);
  // Srcs whose bitmap has painted. Keyed by src (not index) so switching
  // carousel dots back to an already-loaded image never re-shows a skeleton.
  const [loadedSrcs, setLoadedSrcs] = useState<Record<string, boolean>>({});

  const markLoaded = useCallback((src: string) => {
    setLoadedSrcs((prev) => (prev[src] ? prev : { ...prev, [src]: true }));
  }, []);

  const visible = images.filter((_, i) => !errored[i]);

  if (visible.length === 0) {
    return <MenuItemNoMediaHeader label={noMediaLabel} />;
  }

  const safeIndex = Math.min(index, visible.length - 1);
  const src = visible[safeIndex];

  return (
    <div className="relative w-full aspect-[4/3] overflow-hidden bg-gray-50">
      <NextImage
        src={src}
        unoptimized={!canOptimizeImageSrc(src)}
        alt={alt}
        fill
        sizes="(max-width: 768px) 100vw, (max-width: 1024px) 50vw, 33vw"
        className="object-cover transition-transform duration-500 group-hover:scale-[1.04] motion-reduce:transform-none"
        ref={(node) => {
          // Cached images can be complete before onLoad is wired — mark them
          // immediately so there is no skeleton flash.
          if (isImageAlreadyLoaded(node)) markLoaded(src);
        }}
        onLoad={() => markLoaded(src)}
        onError={() => {
          setErrored((prev) => ({ ...prev, [safeIndex]: true }));
        }}
      />
      {!loadedSrcs[src] && <ImageLoadingSkeleton className="bg-warm-100" />}
      {visible.length > 1 && (
        <div className="absolute bottom-2 left-1/2 -translate-x-1/2 flex gap-1">
          {visible.map((_, i) => (
            <button
              key={i}
              type="button"
              aria-label={`Image ${i + 1} of ${visible.length}`}
              onClick={(e) => {
                e.stopPropagation();
                setIndex(i);
              }}
              className={`w-1.5 h-1.5 rounded-full transition-colors ${
                i === safeIndex ? "bg-white" : "bg-white/60 hover:bg-white"
              }`}
            />
          ))}
        </div>
      )}
    </div>
  );
}
