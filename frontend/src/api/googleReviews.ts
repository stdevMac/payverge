import { axiosInstance } from '@/api/tools/instance';
import { apiErrorDetail } from '@/utils/apiError';

export interface GoogleReview {
  author_name: string;
  author_url?: string;
  profile_photo_url?: string;
  rating: number;
  relative_time_description: string;
  text: string;
  time: number;
}

interface GooglePlaceDetails {
  place_id: string;
  name: string;
  rating: number;
  user_ratings_total: number;
  reviews: GoogleReview[];
}

export interface GoogleReviewsResponse {
  reviews: GoogleReview[];
  count: number;
  google_place_id: string;
  business_name: string;
  google_business_url: string;
  google_review_link: string;
  message?: string;
}

export interface GoogleDetailsResponse {
  place_details: GooglePlaceDetails | null;
  google_place_id: string;
  business_name: string;
  google_business_url: string;
  google_review_link: string;
  message?: string;
}

/**
 * Fetch Google reviews for a business by custom URL (public endpoint)
 */
export const getBusinessGoogleReviews = async (customUrl: string, language?: string): Promise<GoogleReviewsResponse> => {
  try {
    const params = language ? { language } : {};
    const response = await axiosInstance.get<GoogleReviewsResponse>(
      `/business/${customUrl}/google/reviews`,
      { params }
    );
    return response.data;
  } catch (error) {
    throw new Error(apiErrorDetail(error) || 'Failed to fetch Google reviews');
  }
};

/**
 * Fetch detailed Google business information including reviews by custom URL (public endpoint)
 */
export const getBusinessGoogleDetails = async (customUrl: string): Promise<GoogleDetailsResponse> => {
  try {
    const response = await axiosInstance.get<GoogleDetailsResponse>(
      `/business/${customUrl}/google/details`
    );
    return response.data;
  } catch (error) {
    throw new Error(apiErrorDetail(error) || 'Failed to fetch Google business details');
  }
};
