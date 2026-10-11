import { translatePositionName } from "./positionLabel";

const t = (key: string) =>
  ({
    "positions.manager": "Gerente",
    "positions.server": "Mozo",
    "positions.host": "Anfitrión",
    "positions.kitchen": "Cocina",
  })[key] ?? key;

it("translates seeded default position names", () => {
  expect(translatePositionName("Manager", t)).toBe("Gerente");
  expect(translatePositionName("Server", t)).toBe("Mozo");
});

it("passes custom names through untouched", () => {
  expect(translatePositionName("Sommelier Jefe", t)).toBe("Sommelier Jefe");
});

it("matches seeded names case- and whitespace-insensitively", () => {
  expect(translatePositionName("  server ", t)).toBe("Mozo");
  expect(translatePositionName("KITCHEN", t)).toBe("Cocina");
});
