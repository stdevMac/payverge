import { capitalizeFirstLetter } from "./capitalizeFirstLetter";

describe("capitalizeFirstLetter", () => {
  it("capitalizes a lowercase Spanish highlight title", () => {
    expect(capitalizeFirstLetter("servicio de IA")).toBe("Servicio de IA");
  });

  it("leaves an already-capped title unchanged", () => {
    expect(capitalizeFirstLetter("Servicio de IA")).toBe("Servicio de IA");
  });

  it("does not mutate empty or whitespace-only input", () => {
    expect(capitalizeFirstLetter("")).toBe("");
    expect(capitalizeFirstLetter("   ")).toBe("   ");
  });
});
