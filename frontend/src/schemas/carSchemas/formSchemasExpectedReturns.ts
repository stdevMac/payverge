import * as Yup from "yup";

const frequencies = ["Hourly", "Daily", "Weekly", "Monthly"];

export const formSchemasExpectedReturns = Yup.object().shape({
    expected_rental_income: Yup.number()
        .transform((value, originalValue) =>
            String(originalValue).trim() === "" ? undefined : value,
        )
        .typeError("Expected Rental Income must be a number")
        .required("Expected Rental Income is a required field"),

    rental_return_frequency: Yup.string()
        .oneOf(frequencies, "Invalid Rental Return Frequency")
        .required("Rental Return Frequency is required"),
});
