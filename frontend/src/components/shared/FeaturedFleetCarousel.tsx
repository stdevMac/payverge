"use client";
import { useEffect } from "react";
import { Swiper, SwiperSlide } from "swiper/react";
import { Autoplay, Navigation, Pagination } from "swiper/modules";
import Image from "next/image";
import { useRouter } from "next/navigation";
import { useTranslation } from "@/i18n/useTranslation";

// Import Swiper styles
import "swiper/css";
import "swiper/css/navigation";
import "swiper/css/pagination";

// Banner image sources - these should be placed in public/images/banners/
const bannerImageSources = [
  "/images/banners/banner1.webp",
  "/images/banners/banner2.webp",
  "/images/banners/banner3.webp",
];

interface Props {
  initialFeaturedFleetId?: string;
}

export const FeaturedFleetCarousel = ({ initialFeaturedFleetId }: Props) => {
  const { t } = useTranslation();
  const router = useRouter();
  
  // Get banner images from translations
  const bannerImages = bannerImageSources.map((src, index) => ({
    src,
    alt: t(`featuredFleet.carousel.banners.${index}.alt`),
    title: t(`featuredFleet.carousel.banners.${index}.title`),
    subtitle: t(`featuredFleet.carousel.banners.${index}.subtitle`),
  }));

  const handleClick = () => {
    // if (featuredFleetId) {
    //   router.push(`/fleet/${featuredFleetId}`);
    // }
  };

  return (
    <div className="relative w-full bg-gradient-to-r from-blue-900 to-blue-800 overflow-hidden rounded-b-3xl">
      <Swiper
        spaceBetween={0}
        centeredSlides={true}
        loop={true}
        slidesPerView={1}
        slidesPerGroup={1}
        effect="slide"
        rewind={true}
        speed={800}
        autoplay={{
          delay: 5000,
          disableOnInteraction: false,
          pauseOnMouseEnter: true,
        }}
        pagination={{
          clickable: true,
          dynamicBullets: true,
        }}
        navigation={{
          prevEl: ".swiper-button-prev",
          nextEl: ".swiper-button-next",
        }}
        modules={[Autoplay, Pagination, Navigation]}
        className="h-[400px] w-full group"
        onClick={handleClick}
      >
        {bannerImages.map((image, index) => (
          <SwiperSlide
            key={index}
            className="relative cursor-pointer transition-transform duration-300 hover:scale-[1.02]"
            style={{ cursor: "pointer" }}
          >
            <Image
              src={image.src}
              alt={image.alt}
              fill
              className="object-cover transition-transform duration-500"
              priority
            />
            <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/20 to-transparent">
              <div className="absolute bottom-0 left-0 right-0 p-8 text-white transform transition-transform duration-300">
                <h2 className="text-3xl font-bold mb-2 drop-shadow-lg">
                  {image.title}
                </h2>
                <p className="text-xl drop-shadow-md">{image.subtitle}</p>
                {/* {featuredFleetId && (
                  <p className="text-sm mt-4 text-blue-200 opacity-0 group-hover:opacity-100 transition-opacity duration-300">
                    {t('featuredFleet.carousel.viewFeaturedFleet')}
                  </p>
                )} */}
              </div>
            </div>
          </SwiperSlide>
        ))}
        <div className="swiper-button-prev !text-white/80 !opacity-0 group-hover:!opacity-100 transition-opacity duration-300"></div>
        <div className="swiper-button-next !text-white/80 !opacity-0 group-hover:!opacity-100 transition-opacity duration-300"></div>
      </Swiper>
    </div>
  );
};
