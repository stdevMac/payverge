import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { isAdmin } from "@/utils/auth";
import { decodeJwt } from "@/utils/jwt";

interface SessionToken {
  address: string;
  exp: number;
  role: string;
}

export function middleware(request: NextRequest) {
  // Check if the path starts with /admin
  const pathname = request.nextUrl.pathname;
  
  if (pathname.startsWith("/admin")) {
    // Get the session token from cookies
    const sessionToken = request.cookies.get("session_token")?.value;

    if (!sessionToken) {
      // Redirect to home if no token
      return NextResponse.redirect(new URL('/', request.url));
    }

    try {
      // Parse the JWT token
      const tokenData = decodeJwt(sessionToken) as SessionToken;

      // Check if user has admin role
      if (!isAdmin(tokenData)) {
        // Redirect non-admin users to home
        return NextResponse.redirect(new URL('/', request.url));
      }
    } catch (error) {
      // If there's any error parsing the token, redirect to home
      return NextResponse.redirect(new URL('/', request.url));
    }
  }

  return NextResponse.next();
}

export const config = {
  // Match admin routes
  matcher: ["/admin/:path*"],
};
