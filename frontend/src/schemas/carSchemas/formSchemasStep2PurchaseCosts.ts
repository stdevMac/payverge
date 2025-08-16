import * as Yup from "yup";

export const formSchemasStep2PurchaseCosts = Yup.object().shape({
    purchase_price: Yup.number()
        .typeError("Purchase price must be a number")
        .positive("Purchase price must be a positive number")
        .required("Purchase price is a required field"),
    insurance_yearly: Yup.number()
        .typeError("Insurance yearly must be a number")
        .positive("Insurance yearly must be a positive number")
        .required("Insurance yearly is a required field"),
    initial_mods: Yup.number()
        .typeError("Initial mods must be a number")
        .integer("Initial mods must be a positive number")
        .required("Initial mods is a required field"),
});
