// Utility functions for handling cookies
export const setCookie = (name: string, value: string, days: number): void => {
    if (typeof document === "undefined") return;
    const expires = new Date(Date.now() + days * 864e5).toUTCString();
    document.cookie = `${name}=${encodeURIComponent(
        value,
    )}; expires=${expires}; path=/; Secure; SameSite=Strict`;
};

export const getCookie = (name: string): string | null => {
    if (typeof document === "undefined") return null;
    const cookies = document.cookie.split("; ");
    for (const cookie of cookies) {
        const [key, value] = cookie.split("=");
        if (key === name) {
            return decodeURIComponent(value);
        }
    }
    return null;
};

// Parsing and stringifying cookies data
export const setCookieJSON = (
    name: string,
    value: object,
    days: number,
): void => {
    setCookie(name, JSON.stringify(value), days);
};

export const getCookieJSON = function <T>(name: string): T | null {
    const cookieValue = getCookie(name);
    if (!cookieValue) return null;
    try {
        return JSON.parse(cookieValue) as T;
    } catch {
        return null;
    }
};

export const clearCookies = () => {
    if (typeof document === "undefined") return;
    const cookies = document.cookie.split("; ");
    for (const cookie of cookies) {
        const [name] = cookie.split("=");
        document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; Secure; SameSite=Strict`;
    }
};
