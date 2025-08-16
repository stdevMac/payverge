import * as yup from "yup";
export const formShemasStep3 = yup.object().shape({
    /////Step 4
    images: yup.array().required("Images1 is a required field"),
});
