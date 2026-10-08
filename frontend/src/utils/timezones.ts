// Common timezones grouped by region for business settings
export interface TimezoneOption {
  value: string; // IANA timezone identifier
  label: string; // Display name
  offset: string; // UTC offset for reference
}

export const TIMEZONE_OPTIONS: TimezoneOption[] = [
  // Middle East & Gulf
  { value: "Asia/Dubai", label: "Dubai / Abu Dhabi (GST)", offset: "UTC+4" },
  { value: "Asia/Riyadh", label: "Riyadh / Saudi Arabia (AST)", offset: "UTC+3" },
  { value: "Asia/Kuwait", label: "Kuwait", offset: "UTC+3" },
  { value: "Asia/Bahrain", label: "Bahrain", offset: "UTC+3" },
  { value: "Asia/Qatar", label: "Qatar", offset: "UTC+3" },
  { value: "Asia/Muscat", label: "Muscat / Oman", offset: "UTC+4" },
  
  // Europe
  { value: "Europe/London", label: "London (GMT/BST)", offset: "UTC+0/+1" },
  { value: "Europe/Paris", label: "Paris / Berlin / Rome (CET)", offset: "UTC+1/+2" },
  { value: "Europe/Madrid", label: "Madrid", offset: "UTC+1/+2" },
  { value: "Europe/Amsterdam", label: "Amsterdam", offset: "UTC+1/+2" },
  { value: "Europe/Istanbul", label: "Istanbul", offset: "UTC+3" },
  { value: "Europe/Moscow", label: "Moscow", offset: "UTC+3" },
  
  // Americas
  { value: "America/New_York", label: "New York (EST/EDT)", offset: "UTC-5/-4" },
  { value: "America/Chicago", label: "Chicago (CST/CDT)", offset: "UTC-6/-5" },
  { value: "America/Denver", label: "Denver (MST/MDT)", offset: "UTC-7/-6" },
  { value: "America/Los_Angeles", label: "Los Angeles (PST/PDT)", offset: "UTC-8/-7" },
  { value: "America/Toronto", label: "Toronto", offset: "UTC-5/-4" },
  { value: "America/Mexico_City", label: "Mexico City", offset: "UTC-6/-5" },
  { value: "America/Sao_Paulo", label: "São Paulo", offset: "UTC-3" },
  { value: "America/Buenos_Aires", label: "Buenos Aires", offset: "UTC-3" },
  
  // Asia Pacific
  { value: "Asia/Singapore", label: "Singapore", offset: "UTC+8" },
  { value: "Asia/Hong_Kong", label: "Hong Kong", offset: "UTC+8" },
  { value: "Asia/Tokyo", label: "Tokyo", offset: "UTC+9" },
  { value: "Asia/Seoul", label: "Seoul", offset: "UTC+9" },
  { value: "Asia/Shanghai", label: "Shanghai / Beijing", offset: "UTC+8" },
  { value: "Asia/Bangkok", label: "Bangkok", offset: "UTC+7" },
  { value: "Asia/Kolkata", label: "Mumbai / Delhi (IST)", offset: "UTC+5:30" },
  { value: "Australia/Sydney", label: "Sydney", offset: "UTC+10/+11" },
  { value: "Australia/Melbourne", label: "Melbourne", offset: "UTC+10/+11" },
  { value: "Pacific/Auckland", label: "Auckland", offset: "UTC+12/+13" },
  
  // Africa
  { value: "Africa/Cairo", label: "Cairo", offset: "UTC+2" },
  { value: "Africa/Johannesburg", label: "Johannesburg", offset: "UTC+2" },
  { value: "Africa/Lagos", label: "Lagos", offset: "UTC+1" },
  { value: "Africa/Nairobi", label: "Nairobi", offset: "UTC+3" },
  
  // Other
  { value: "UTC", label: "UTC (Coordinated Universal Time)", offset: "UTC+0" },
];

// Helper function to get timezone display name
export function getTimezoneLabel(timezone: string): string {
  const option = TIMEZONE_OPTIONS.find(tz => tz.value === timezone);
  return option ? option.label : timezone;
}
