import { Metadata } from "next";
import VerifyEmailClient from "./VerifyEmailClient";

export const metadata: Metadata = {
  title: "Verify email — Payverge",
  robots: { index: false, follow: true },
};

export default function VerifyEmailPage() {
  return <VerifyEmailClient />;
}
