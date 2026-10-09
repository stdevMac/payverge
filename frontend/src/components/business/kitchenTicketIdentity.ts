/**
 * Visible text separator between kitchen / KDS order number and bill label.
 * CSS gap, wrap, or stacked buttons are not text nodes — live QA reads
 * innerText and treats `#G86-36604192Cuenta n.º 1132` as smashed (#775).
 */
export const KITCHEN_TICKET_IDENTITY_SEP = " · ";

export function kitchenTicketIdentityText(
  orderNumber: string,
  billLabel: string,
): string {
  return `#${orderNumber}${KITCHEN_TICKET_IDENTITY_SEP}${billLabel}`;
}
