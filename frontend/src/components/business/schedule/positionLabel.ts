// Position names are USER DATA: the demo/seed defaults ship in English
// (backend/internal/demo/generator.go seeds Manager/Server/Host/Kitchen), while
// operator-created positions carry whatever the operator typed. The es/es-AR
// values of positions.* are kept byte-identical to Equipo's roles.*.label
// (businessDashboard.json) so both surfaces name the same role the same way —
// but only seeded defaults translate. Anything else (e.g. "Sommelier Jefe")
// renders raw.
const DEFAULT_POSITION_KEYS: Record<string, string> = {
  manager: "positions.manager",
  server: "positions.server",
  host: "positions.host",
  kitchen: "positions.kitchen",
};

export function translatePositionName(
  name: string,
  t: (key: string) => string,
): string {
  const key = DEFAULT_POSITION_KEYS[name.trim().toLowerCase()];
  if (!key) return name;
  return t(key);
}
