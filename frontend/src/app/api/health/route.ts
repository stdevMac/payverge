import { NextResponse } from 'next/server';

export async function GET() {
  return NextResponse.json({
    status: 'ok',
    timestamp: new Date().toISOString(),
  });
}

// Explicit HEAD so the same-origin connectivity probe (ConnectivityContext does
// a HEAD to confirm reachability) gets a 200 without depending on framework
// auto-HEAD behavior.
export async function HEAD() {
  return new NextResponse(null, { status: 200 });
}
