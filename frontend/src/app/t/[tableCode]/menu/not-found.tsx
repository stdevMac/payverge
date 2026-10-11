import type { Metadata } from "next";
import NotFoundClient from "@/app/NotFoundClient";

export const metadata: Metadata = {
  title: "Menu | Payverge",
  robots: { index: false, follow: false },
};

export default function MenuNotFound() {
  return <NotFoundClient />;
}
