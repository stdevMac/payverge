import {
  SAMPLE_CHECK_SUBTOTAL,
  formatSampleCheckMoney,
  previewSampleCheck,
} from "./sampleCheckPreview";

describe("previewSampleCheck (#266)", () => {
  it("defaults to a $100 sample subtotal", () => {
    expect(SAMPLE_CHECK_SUBTOTAL).toBe(100);
  });

  it("matches BillTotalsCents half-up math on $100", () => {
    const preview = previewSampleCheck({
      tax_rate: 10,
      service_fee_rate: 5,
      tax_inclusive: false,
      service_inclusive: false,
    });
    expect(preview.subtotal).toBe(100);
    expect(preview.tax).toBe(10);
    expect(preview.serviceFee).toBe(5);
    expect(preview.total).toBe(115);
  });

  it("rounds fractional rates the same way as backend cents math", () => {
    // 8.875% of $100 = 8.875 → half-up to $8.88
    const preview = previewSampleCheck({
      tax_rate: 8.875,
      service_fee_rate: 7.25,
      tax_inclusive: false,
      service_inclusive: false,
    });
    expect(preview.tax).toBe(8.88);
    expect(preview.serviceFee).toBe(7.25);
    expect(preview.total).toBe(116.13);
  });

  it("surfaces inclusive flags without changing additive bill math", () => {
    const preview = previewSampleCheck({
      tax_rate: 10,
      service_fee_rate: 0,
      tax_inclusive: true,
      service_inclusive: true,
    });
    expect(preview.total).toBe(110);
    expect(preview.taxInclusive).toBe(true);
    expect(preview.serviceInclusive).toBe(true);
  });

  it("formats sample money with two decimals", () => {
    expect(formatSampleCheckMoney(115)).toBe("$115.00");
    expect(formatSampleCheckMoney(8.8)).toBe("$8.80");
  });
});
