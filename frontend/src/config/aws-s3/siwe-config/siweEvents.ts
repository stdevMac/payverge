// Custom event for SIWE verification
export const SIWE_VERIFIED_EVENT = "siwe_verified";

export const emitSiweVerified = () => {
    const event = new CustomEvent(SIWE_VERIFIED_EVENT, {
        detail: { timestamp: Date.now() },
    });
    window.dispatchEvent(event);
};
