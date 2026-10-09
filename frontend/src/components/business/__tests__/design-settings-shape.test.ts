import type { PublicBusiness } from "@/api/publicBusiness";

describe("PublicBusiness.design_settings", () => {
  it("includes hero_layout and section_density", () => {
    const sample: NonNullable<PublicBusiness["design_settings"]> = {
      primary_color: "black",
      secondary_color: "black",
      font_family: "Inter",
      theme: "light",
      menu_layout: "grid",
      show_images: true,
      show_descriptions: true,
      header_style: "banner",
      corner_radius: "medium",
      shadow_intensity: "subtle",
      background_pattern: "none",
      pattern_opacity: 0.1,
      hero_layout: "centered",
      section_density: "comfortable",
    };
    expect(sample.hero_layout).toBe("centered");
    expect(sample.section_density).toBe("comfortable");
  });
});
