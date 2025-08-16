import * as yup from "yup";

export const formShemasStep1 = yup.object().shape({
    /////Step 1
    brand: yup
        .string()
        .required("Brand is a required field")
        .min(2, "Brand must be at least 2 characters long")
        .max(50, "Brand cannot be more than 50 characters long"),

    model: yup
        .string()
        .required("Model is a required field")
        .min(2, "Model must be at least 2 characters long")
        .max(50, "Model cannot be more than 50 characters long"),

    year: yup
        .string()
        .required("Year is a required field")
        .matches(/^\d{4}$/, "Year must be a 4-digit number"),

    about: yup
        .string()
        .required("About is a required field")
        .min(10, "About must be at least 10 characters long")
        .max(5000, "About cannot be more than 5000 characters long"),

    about_es: yup
        .string()
        .min(10, "Spanish description must be at least 10 characters long")
        .max(5000, "Spanish description cannot be more than 5000 characters long"),
});
