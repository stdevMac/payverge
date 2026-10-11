import { Metadata } from "next";
import ResetPasswordClient from "./ResetPasswordClient";

export const metadata: Metadata = {
  title: "Reset password — Payverge",
  robots: { index: false, follow: true },
};

export default function ResetPasswordPage() {
  return <ResetPasswordClient />;
}
