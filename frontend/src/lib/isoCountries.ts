// Full ISO-3166-1 alpha-2 registry for the registration country picker.
// Curated countries (frontend/src/lib/geoDefaults.ts COUNTRY_OPTIONS) get a
// real currency/timezone seed; every other code submits as-is and the backend
// geodefaults fallback seeds {USD, UTC} (backend/internal/geodefaults).
// Display names come from Intl.DisplayNames so they localize for free.

export const ISO_3166_ALPHA2_CODES: readonly string[] = [
  "AD","AE","AF","AG","AI","AL","AM","AO","AQ","AR","AS","AT","AU","AW","AX","AZ",
  "BA","BB","BD","BE","BF","BG","BH","BI","BJ","BL","BM","BN","BO","BQ","BR","BS","BT","BV","BW","BY","BZ",
  "CA","CC","CD","CF","CG","CH","CI","CK","CL","CM","CN","CO","CR","CU","CV","CW","CX","CY","CZ",
  "DE","DJ","DK","DM","DO","DZ",
  "EC","EE","EG","EH","ER","ES","ET",
  "FI","FJ","FK","FM","FO","FR",
  "GA","GB","GD","GE","GF","GG","GH","GI","GL","GM","GN","GP","GQ","GR","GS","GT","GU","GW","GY",
  "HK","HM","HN","HR","HT","HU",
  "ID","IE","IL","IM","IN","IO","IQ","IR","IS","IT",
  "JE","JM","JO","JP",
  "KE","KG","KH","KI","KM","KN","KP","KR","KW","KY","KZ",
  "LA","LB","LC","LI","LK","LR","LS","LT","LU","LV","LY",
  "MA","MC","MD","ME","MF","MG","MH","MK","ML","MM","MN","MO","MP","MQ","MR","MS","MT","MU","MV","MW","MX","MY","MZ",
  "NA","NC","NE","NF","NG","NI","NL","NO","NP","NR","NU","NZ",
  "OM",
  "PA","PE","PF","PG","PH","PK","PL","PM","PN","PR","PS","PT","PW","PY",
  "QA",
  "RE","RO","RS","RU","RW",
  "SA","SB","SC","SD","SE","SG","SH","SI","SJ","SK","SL","SM","SN","SO","SR","SS","ST","SV","SX","SY","SZ",
  "TC","TD","TF","TG","TH","TJ","TK","TL","TM","TN","TO","TR","TT","TV","TW","TZ",
  "UA","UG","UM","US","UY","UZ",
  "VA","VC","VE","VG","VI","VN","VU",
  "WF","WS",
  "YE","YT",
  "ZA","ZM","ZW",
];

export interface IsoCountryOption {
  code: string;
  name: string;
}

/**
 * Returns every ISO country as {code, name}, localized to `locale` when the
 * runtime's ICU data supports it (falls back to English, then the raw code),
 * sorted by localized display name.
 */
export function getAllCountryOptions(locale: string = "en"): IsoCountryOption[] {
  let display: Intl.DisplayNames | null = null;
  try {
    display = new Intl.DisplayNames([locale, "en"], { type: "region" });
  } catch {
    display = null;
  }
  return ISO_3166_ALPHA2_CODES.map((code) => ({
    code,
    name: display?.of(code) ?? code,
  })).sort((a, b) => a.name.localeCompare(b.name, locale));
}
