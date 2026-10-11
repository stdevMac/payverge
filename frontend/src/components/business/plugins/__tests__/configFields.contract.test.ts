import stripeSchema from "../__fixtures__/plugin-config-schemas/stripe.json";
import mercadopagoSchema from "../__fixtures__/plugin-config-schemas/mercadopago.json";
import paypalSchema from "../__fixtures__/plugin-config-schemas/paypal.json";
import {
  buildStripeInitialConfig,
  buildMercadoPagoInitialConfig,
  buildPayPalInitialConfig,
  STRIPE_UI_ONLY_FIELDS,
  MERCADOPAGO_UI_ONLY_FIELDS,
  PAYPAL_UI_ONLY_FIELDS,
} from "../configFields";

type Schema = {
  required?: string[];
  properties?: Record<string, unknown>;
};

const cases: Array<{
  name: string;
  schema: Schema;
  submittedFields: string[];
  uiOnlyFields: string[];
}> = [
  {
    name: "stripe",
    schema: stripeSchema as Schema,
    submittedFields: Object.keys(buildStripeInitialConfig({})),
    uiOnlyFields: STRIPE_UI_ONLY_FIELDS,
  },
  {
    name: "mercadopago",
    schema: mercadopagoSchema as Schema,
    submittedFields: Object.keys(buildMercadoPagoInitialConfig({})),
    uiOnlyFields: MERCADOPAGO_UI_ONLY_FIELDS,
  },
  {
    name: "paypal",
    schema: paypalSchema as Schema,
    submittedFields: Object.keys(buildPayPalInitialConfig({})),
    uiOnlyFields: PAYPAL_UI_ONLY_FIELDS,
  },
];

describe("payment plugin config form ↔ backend schema contract", () => {
  describe.each(cases)("$name", ({ schema, submittedFields, uiOnlyFields }) => {
    const properties = Object.keys(schema.properties ?? {});
    const required = schema.required ?? [];

    it("collects every field the backend marks required", () => {
      const missing = required.filter(
        (field) => !submittedFields.includes(field),
      );
      // A required field with no form input means the plugin 400s on every save
      // and can never be enabled (the MercadoPago/PayPal base_url class of bug).
      expect(missing).toEqual([]);
    });

    it("only submits fields the backend actually reads", () => {
      const dead = submittedFields.filter(
        (field) =>
          !properties.includes(field) && !uiOnlyFields.includes(field),
      );
      // A submitted field that is neither a schema property nor a declared
      // UI-only field is dead weight the backend ignores (the old Stripe
      // webhook_endpoint / MercadoPago webhook_url class of bug).
      expect(dead).toEqual([]);
    });

    it("does not declare a schema property as UI-only", () => {
      const bogus = uiOnlyFields.filter((field) => properties.includes(field));
      expect(bogus).toEqual([]);
    });
  });
});
