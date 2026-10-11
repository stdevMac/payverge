import { redirect } from "next/navigation";

export default async function PrintersPage({
  params,
}: {
  params: Promise<{ businessId: string }>;
}) {
  const { businessId } = await params;
  redirect(`/business/${businessId}/dashboard?tab=printers`);
}
