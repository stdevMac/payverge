import { axiosInstance } from './index';

// Google Places API interfaces
export interface GooglePlaceResult {
  place_id: string;
  name: string;
  formatted_address: string;
  rating?: number;
  user_ratings_total?: number;
  types?: string[];
  geometry?: {
    location: {
      lat: number;
      lng: number;
    };
  };
}

export interface GoogleBusinessSearchResponse {
  results: GooglePlaceResult[];
  count: number;
}

export interface GoogleBusinessUpdateRequest {
  place_id: string;
  business_name: string;
}

export interface GoogleBusinessUpdateResponse {
  message: string;
  google_place_id: string;
  google_business_name: string;
  google_review_link: string;
  google_business_url: string;
}

// Search for Google businesses
export const searchGoogleBusinesses = async (query: string): Promise<GoogleBusinessSearchResponse> => {
  const response = await axiosInstance.post('/inside/google/businesses/search', { query });
  return response.data;
};

// Update business Google information
export const updateBusinessGoogleInfo = async (
  businessId: string,
  data: GoogleBusinessUpdateRequest
): Promise<GoogleBusinessUpdateResponse> => {
  const response = await axiosInstance.put(`/inside/businesses/${businessId}/google`, data);
  return response.data;
};

// Remove business Google integration
export const removeBusinessGoogleInfo = async (businessId: string): Promise<{ message: string }> => {
  const response = await axiosInstance.delete(`/inside/businesses/${businessId}/google`);
  return response.data;
};

// Helper function to format business query for better search results
export const formatBusinessQuery = (businessName: string, address?: string): string => {
  let query = businessName.trim();
  
  if (address) {
    query = `${query}, ${address.trim()}`;
  }
  
  return query;
};

// Helper function to generate review link from Place ID
export const generateReviewLink = (placeId: string): string => {
  return `https://search.google.com/local/writereview?placeid=${placeId}`;
};

// Helper function to generate Google Maps business URL from Place ID
export const generateBusinessURL = (placeId: string): string => {
  return `https://maps.google.com/maps/place/?q=place_id:${placeId}`;
};
