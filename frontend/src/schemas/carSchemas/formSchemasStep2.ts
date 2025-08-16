import * as Yup from "yup";

const frequencies = ["Hourly", "Daily", "Weekly", "Monthly"];

export const formShemasStep2 = Yup.object().shape({
    expected_rental_income: Yup.number()
        .transform((value, originalValue) =>
            String(originalValue).trim() === "" ? undefined : value,
        )
        .typeError("Expected Rental Income must be a number")
        .required("Expected Rental Income is a required field"),

    rental_return_frequency: Yup.string()
        .oneOf(frequencies, "Invalid Rental Return Frequency")
        .required("Rental Return Frequency is required"),

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
