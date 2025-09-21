interface DecodedToken {
  [key: string]: any;
  exp?: number;
  address?: string;
  chainId?: number;
}

export function decodeJwt(token: string): DecodedToken {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map(c => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    );
    return JSON.parse(jsonPayload);
  } catch (error) {
    console.error('Error decoding JWT:', error);
    return {};
  }
}

export function isTokenValid(token: string): boolean {
  try {
    const decoded = decodeJwt(token);
    const now = Math.floor(Date.now() / 1000);
    return decoded.exp ? decoded.exp > now : false;
  } catch (error) {
    return false;
  }
}
