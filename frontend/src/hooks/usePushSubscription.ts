import { getPublicConfig } from "@/config/publicConfig";
import { useCallback, useEffect, useState } from "react";
import { axiosInstance } from "@/api/index";

// Runtime public config (window.__PAYVERGE_ENV__), read lazily.
const vapidPublicKey = () => getPublicConfig().vapidPublicKey;

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const rawData = window.atob(base64);
  const buffer = new ArrayBuffer(rawData.length);
  const view = new Uint8Array(buffer);
  for (let i = 0; i < rawData.length; i++) {
    view[i] = rawData.charCodeAt(i);
  }
  return view;
}

export function usePushSubscription(businessId: number) {
  const [isSubscribed, setIsSubscribed] = useState(false);
  const [isSupported, setIsSupported] = useState(false);

  useEffect(() => {
    setIsSupported("serviceWorker" in navigator && "PushManager" in window);
  }, []);

  useEffect(() => {
    if (!isSupported) return;
    navigator.serviceWorker.ready.then((reg) => {
      reg.pushManager.getSubscription().then((sub) => {
        setIsSubscribed(!!sub);
      });
    });
  }, [isSupported]);

  const subscribe = useCallback(async () => {
    if (!isSupported || !vapidPublicKey()) return;
    const reg = await navigator.serviceWorker.register("/sw.js");
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKey()),
    });
    const key = sub.toJSON();
    await axiosInstance.post("/inside/push-subscriptions", {
      business_id: businessId,
      endpoint: key.endpoint,
      p256dh_key: key.keys?.p256dh,
      auth_key: key.keys?.auth,
      user_agent: navigator.userAgent,
    });
    setIsSubscribed(true);
  }, [isSupported, businessId]);

  const unsubscribe = useCallback(async () => {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (sub) {
      await sub.unsubscribe();
      setIsSubscribed(false);
    }
  }, []);

  return { isSupported, isSubscribed, subscribe, unsubscribe };
}
