import { formatStaffActor } from "./formatStaffActor";

const team = [
  { id: 1, name: "Ana Server", email: "ana@x.com" },
  { id: 2, name: "Bob Host", email: "bob@x.com" },
  { id: 3, name: "Cara Mgr", email: "cara@x.com" },
];

describe("formatStaffActor L5-25", () => {
  it("resolves another team member by email (not only the modal subject)", () => {
    const subject = team[0];
    // Cara changed Ana's role — must show Cara's name, not raw email.
    expect(
      formatStaffActor("cara@x.com", {
        subject,
        teamMembers: team,
      }),
    ).toBe("Cara Mgr");
  });

  it("resolves by staff id string", () => {
    expect(
      formatStaffActor("2", {
        subject: team[0],
        teamMembers: team,
      }),
    ).toBe("Bob Host");
  });

  it("falls back to truncated wallet for unknown 0x, owner label for known", () => {
    expect(
      formatStaffActor("0x1234567890abcdef", {
        teamMembers: team,
        ownerAddresses: ["0xABCDEF0000000001"],
      }),
    ).toBe("0x1234…cdef");

    expect(
      formatStaffActor("0xABCDEF0000000001", {
        teamMembers: team,
        ownerAddresses: ["0xabcdef0000000001"],
        ownerLabel: "Mara Owner",
      }),
    ).toBe("Mara Owner");
  });

  it("cannot resolve other actors when teamMembers is empty (the regression)", () => {
    // Pre-L5-25: only subject matched → other emails stayed raw.
    expect(
      formatStaffActor("cara@x.com", {
        subject: team[0],
        teamMembers: [],
      }),
    ).toBe("cara@x.com");
  });
});
