// Guest AI Waiter visibility is gated on the backend-computed `ai_available`
// flag (instance AI configured + business operational + enabled toggle).
//
// This lives in its own module rather than in page.tsx because Next.js App
// Router page files may only export a fixed set of reserved fields (default,
// metadata, generateMetadata, etc.); a stray named export fails the build with
// "<name> is not a valid Page export field".
export function shouldShowAiWaiter(
  business: { id?: number; ai_available?: boolean } | null | undefined,
): boolean {
  return !!business?.id && business?.ai_available === true;
}
