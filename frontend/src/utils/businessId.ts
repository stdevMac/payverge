/**
 * Utility functions for handling business ID conversion between string and number formats
 * This is a temporary solution during the transition from numeric to string business IDs
 */

/**
 * Extracts numeric ID from business object or converts string businessId to number
 * This is a temporary helper during the transition period
 */
export function getNumericBusinessId(business: any, businessId?: string): number {
  // If we have a business object with numeric ID, use that
  if (business && typeof business.id === 'number') {
    return business.id;
  }
  
  // If businessId is a string that looks like a number, convert it
  if (businessId && /^\d+$/.test(businessId)) {
    return parseInt(businessId, 10);
  }
  
  // For string businessIds (like "my-restaurant-a1b2c3d4"), we need to look up the numeric ID
  // For now, return 0 as a fallback - this will need proper implementation
  console.warn('Cannot convert string businessId to number:', businessId);
  return 0;
}

/**
 * Checks if a businessId is in the new string format
 */
export function isStringBusinessId(businessId: string | number): boolean {
  if (typeof businessId === 'number') return false;
  return !/^\d+$/.test(businessId);
}
