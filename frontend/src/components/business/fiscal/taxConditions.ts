export type SupportedFiscalCountry = "AR" | "AE";

/** AR tax condition wire values must match backend fiscal policy (monotributo). */
export const TAX_CONDITIONS_BY_COUNTRY: Record<
  SupportedFiscalCountry,
  string[]
> = {
  AR: [
    "responsable_inscripto",
    "monotributo",
    "exento",
    "consumidor_final",
    "no_responsable",
  ],
  AE: ["vat_registered", "vat_exempt", "non_resident"],
};
