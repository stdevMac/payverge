import {
  KITCHEN_TICKET_IDENTITY_SEP,
  kitchenTicketIdentityText,
} from "./kitchenTicketIdentity";

describe("kitchenTicketIdentityText", () => {
  it("keeps order number and Spanish bill label as separate tokens (#775)", () => {
    expect(
      kitchenTicketIdentityText("G86-36604192", "Cuenta n.º 1132"),
    ).toBe("#G86-36604192 · Cuenta n.º 1132");
    expect(KITCHEN_TICKET_IDENTITY_SEP).toBe(" · ");
    expect(
      kitchenTicketIdentityText("G86-36604192", "Cuenta n.º 1132"),
    ).not.toContain("36604192Cuenta");
  });
});
