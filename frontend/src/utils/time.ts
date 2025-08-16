export const getDubaiTime = () => {
  // Get current time in UTC
  const now = new Date();
  
  // Convert UTC to Dubai time (UTC+4)
  const utcTime = now.getTime();
  const dubaiOffset = 4 * 60 * 60 * 1000; // 4 hours in milliseconds
  
  // Create a new date object with Dubai time
  return new Date(utcTime + dubaiOffset);
};

export const parseGoTime = (goTime: string | null | undefined): Date | null => {
  if (!goTime) return null;
  
  try {
    // Parse the Go time string (RFC3339 format)
    const date = new Date(goTime);
    
    // Check if the date is valid
    if (isNaN(date.getTime())) {
      console.error('Invalid date from Go backend:', goTime);
      return null;
    }
    
    return date;
  } catch (error) {
    console.error('Error parsing Go time:', error);
    return null;
  }
};
