import { axiosInstance } from './tools/instance';
import { defaultLocale, type Locale } from '@/i18n/localeRegistry';
import type { Dollars } from '@/types/money';

// Plugin interfaces
export interface Plugin {
  id: number;
  name: string;
  display_name: string;
  description: string;
  message: string; // Enhanced user message
  image: string;
  is_active: boolean;
  coming_soon: boolean;
  category: string;
  version: string;
  features: string; // JSON string
  config_schema: string; // JSON string
  created_at: string;
  updated_at: string;
  translations?: PluginTranslation[];
}

interface PluginTranslation {
  id: number;
  plugin_id: number;
  language_code: string;
  field_name: string;
  content: string;
  created_at: string;
  updated_at: string;
}

export interface BusinessPlugin {
  id: number;
  business_id: number;
  plugin_id: number;
  is_enabled: boolean;
  config?: string; // JSON string; omitted from list responses that hide secrets
  created_at: string;
  updated_at: string;
  plugin?: Plugin; // Populated plugin data
  // Per-business webhook health, written best-effort by the backend webhook
  // handler. last_status is "", "ok", or "error".
  last_status?: string;
  last_error?: string;
  last_error_at?: string | null;
  last_success_at?: string | null;
}

export interface CreatePluginData {
  name: string;
  display_name: string;
  description: string;
  message: string;
  image?: string;
  coming_soon?: boolean;
  category: string;
  version?: string;
  features: string; // JSON string
  config_schema: string; // JSON string
}

export interface UpdatePluginData extends Partial<CreatePluginData> {
  is_active?: boolean;
  coming_soon?: boolean;
}

export interface EnablePluginData {
  config: Record<string, any>;
}

export interface UpdatePluginConfigData {
  config: Record<string, any>;
}

// Payment Plugin specific interfaces
export interface PaymentPluginResponse {
  payment_id: string;
  status: string;
  payment_url?: string;
  redirect_url?: string;
  qr_code?: string;
  expires_at?: number;
  metadata?: Record<string, any>;
}

export interface CreatePluginPaymentData {
  plugin_id: string;
  /** Total charge in dollars (see wire contract in @/types/money). */
  amount: Dollars;
  currency: string;
  /** Optional tip in dollars (see wire contract in @/types/money). */
  tip_amount?: Dollars;
  metadata?: Record<string, any>;
  return_url?: string;
  cancel_url?: string;
}

// Admin Plugin Management API
export const adminPluginAPI = {
  // Get all plugins
  getAllPlugins: async (): Promise<{ plugins: Plugin[] }> => {
    const response = await axiosInstance.get('/admin/plugins', {
      _useCache: false,
    });
    return response.data;
  },

  // Create a new plugin
  createPlugin: async (data: CreatePluginData): Promise<{ plugin: Plugin }> => {
    const response = await axiosInstance.post('/admin/plugins', data);
    return response.data;
  },

  // Update an existing plugin
  updatePlugin: async (id: number, data: UpdatePluginData): Promise<{ plugin: Plugin }> => {
    const response = await axiosInstance.put(`/admin/plugins/${id}`, data);
    return response.data;
  },

  // Toggle plugin active status
  togglePluginActive: async (id: number): Promise<{ plugin: Plugin }> => {
    const response = await axiosInstance.post(`/admin/plugins/${id}/toggle-active`);
    return response.data;
  },

  // Delete a plugin
  deletePlugin: async (id: number): Promise<void> => {
    await axiosInstance.post(`/admin/plugins/${id}/deactivate`);
  },
};

// Business Plugin Management API
export const businessPluginAPI = {
  // Get business plugins. enabled_payment_count / payments_ready are the
  // backend's truthful enabled-rail summary (#795) — derive payment-readiness
  // UI from these instead of hardcoding it.
  getBusinessPlugins: async (
    businessId: string,
  ): Promise<{
    plugins: BusinessPlugin[];
    enabled_payment_count?: number;
    payments_ready?: boolean;
  }> => {
    const response = await axiosInstance.get(`/inside/businesses/${businessId}/plugins`);
    return response.data;
  },

  // Enable a plugin for a business
  enablePlugin: async (businessId: string, pluginId: number, data: EnablePluginData): Promise<{ business_plugin: BusinessPlugin }> => {
    const response = await axiosInstance.post(`/inside/businesses/${businessId}/plugins/${pluginId}/enable`, data);
    return response.data;
  },

  // Disable a plugin for a business
  disablePlugin: async (businessId: string, pluginId: number): Promise<void> => {
    await axiosInstance.post(`/inside/businesses/${businessId}/plugins/${pluginId}/disable`);
  },

  // Update plugin configuration
  updatePluginConfig: async (businessId: string, pluginId: number, data: UpdatePluginConfigData): Promise<{ business_plugin: BusinessPlugin }> => {
    const response = await axiosInstance.put(`/inside/businesses/${businessId}/plugins/${pluginId}/config`, data);
    return response.data;
  },

  // Get plugin configuration
  getPluginConfig: async (businessId: string, pluginId: number): Promise<{ config: Record<string, any> }> => {
    const response = await axiosInstance.get(`/inside/businesses/${businessId}/plugins/${pluginId}/config`);
    return response.data;
  },

  // Test the stored credentials for a payment plugin with a cheap read-only
  // provider call. The route keys on the plugin *name* (e.g. "stripe"), matching
  // the backend TestPluginConnection dispatch. Never returns credentials.
  testPluginConnection: async (
    businessId: string,
    pluginName: string,
  ): Promise<{ ok: boolean; provider?: string; message?: string }> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/plugins/${pluginName}/test`,
    );
    return response.data;
  },

  // Start Mercado Pago OAuth for a business. Returns the authorization URL the
  // operator should be redirected to (same-window navigation).
  startMercadoPagoOAuth: async (
    businessId: string,
  ): Promise<{ authorization_url: string }> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/plugins/mercadopago/oauth/start`,
    );
    return response.data;
  },

  // Start Stripe Connect OAuth (Standard accounts) for a business.
  startStripeOAuth: async (
    businessId: string,
  ): Promise<{ authorization_url: string }> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/plugins/stripe/oauth/start`,
    );
    return response.data;
  },
};

// Protected Plugin API (for businesses to view marketplace)
const protectedPluginAPI = {
  // Get all active plugins for marketplace (requires authentication)
  getAllPlugins: async (language?: Locale): Promise<{ plugins: Plugin[] }> => {
    const params = language ? { lang: language } : {};
    const response = await axiosInstance.get('/inside/plugins', { params });
    return response.data;
  },
};

// Mercado Pago Point terminal APIs (staff / inside)
export interface MercadoPagoTerminal {
  id: string;
  pos_id?: string;
  store_id?: string;
  external_pos_id?: string;
  operating_mode?: string;
}

export interface MercadoPagoPointChargeBody {
  terminal_id: string;
  /** Optional partial charge in cents; omit / 0 → bill outstanding (server-authoritative). */
  amount_cents?: number;
}

export interface MercadoPagoPointChargeResponse {
  order_id: string;
  status: string;
}

export interface MercadoPagoOrderStatusResponse {
  order_id: string;
  status: string;
  /** Present for QR orders when Mercado Pago still returns the scannable payload. */
  qr_data?: string;
  /** Server-encoded PNG when qr_data is available (409 recovery / poll). */
  qr_png_base64?: string;
}

export const mercadoPagoPointAPI = {
  listTerminals: async (
    businessId: string,
  ): Promise<{ terminals: MercadoPagoTerminal[] }> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/mercadopago/terminals`,
      { _useCache: false },
    );
    return response.data;
  },

  setTerminalMode: async (
    businessId: string,
    terminalId: string,
    mode: "PDV" | "STANDALONE",
  ): Promise<{ terminal_id: string; operating_mode: string }> => {
    const response = await axiosInstance.patch(
      `/inside/businesses/${businessId}/mercadopago/terminals/${encodeURIComponent(terminalId)}/mode`,
      { mode },
    );
    return response.data;
  },

  charge: async (
    businessId: string,
    billId: number,
    body: MercadoPagoPointChargeBody,
  ): Promise<MercadoPagoPointChargeResponse> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/bills/${billId}/mercadopago/point/charge`,
      body,
    );
    return response.data;
  },

  orderStatus: async (
    businessId: string,
    orderId: string,
  ): Promise<MercadoPagoOrderStatusResponse> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/mercadopago/orders/${encodeURIComponent(orderId)}`,
      { _useCache: false },
    );
    return response.data;
  },

  cancelOrder: async (
    businessId: string,
    orderId: string,
  ): Promise<{ status: string }> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/mercadopago/orders/${encodeURIComponent(orderId)}/cancel`,
    );
    return response.data;
  },
};

// Mercado Pago dynamic QR charge (staff / inside). Status + cancel reuse the
// same Orders endpoints as Point (Task 10).
export interface MercadoPagoQRChargeBody {
  /** Optional partial charge in cents; omit / 0 → bill outstanding (server-authoritative). */
  amount_cents?: number;
}

export interface MercadoPagoQRChargeResponse {
  order_id: string;
  qr_data: string;
  qr_png_base64: string;
  expires_at: string;
  status?: string;
}

export const mercadoPagoQRAPI = {
  charge: async (
    businessId: string,
    billId: number,
    body: MercadoPagoQRChargeBody = {},
  ): Promise<MercadoPagoQRChargeResponse> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/bills/${billId}/mercadopago/qr/charge`,
      body,
    );
    return response.data;
  },

  orderStatus: mercadoPagoPointAPI.orderStatus,
  cancelOrder: mercadoPagoPointAPI.cancelOrder,
};

// Payment Plugin API (for guest bill payments)
export const paymentPluginAPI = {
  // Get available payment plugins for a business (public).
  // Optional language is forwarded as ?lang= so the backend can return
  // translated display_name / description for the guest storefront.
  getBusinessPaymentPlugins: async (
    businessId: number,
    language?: string,
  ): Promise<{ plugins: Plugin[]; counter_settlement_ready?: boolean }> => {
    const params = language ? { lang: language } : {};
    const response = await axiosInstance.get(
      `/businesses/${businessId}/payment-plugins`,
      { params },
    );
    return response.data;
  },

  // Create a payment using a plugin (public). billToken is the guest capability.
  createPluginPayment: async (billToken: string, data: CreatePluginPaymentData): Promise<PaymentPluginResponse> => {
    const response = await axiosInstance.post(`/guest/bill/${billToken}/plugin-payment`, data);
    return response.data;
  },

  // Get payment status (public). billToken is the guest capability.
  getPluginPaymentStatus: async (
    billToken: string,
    paymentId: string,
    pluginName: string
  ): Promise<{ status: string; metadata?: Record<string, any> }> => {
    // Never serve this from the GET cache: settlement happens server-side via
    // webhook with no client mutation to invalidate the entry, so a cached first
    // "pending" would be replayed for the full 5-min TTL and the poller could
    // never observe the completed status — guaranteeing a false "Verification
    // Timed Out" on a payment that actually succeeded.
    const response = await axiosInstance.get(`/guest/bill/${billToken}/plugin-payment/${paymentId}/status`, {
      params: { plugin: pluginName },
      _useCache: false,
    });
    return response.data;
  },
};

// Plugin utility functions
export const pluginUtils = {
  // Parse plugin features from JSON string
  parseFeatures: (features: string): string[] => {
    try {
      return JSON.parse(features);
    } catch (e) {
      return [];
    }
  },

  // Parse plugin config schema from JSON string
  parseConfigSchema: (configSchema: string): Record<string, any> => {
    try {
      return JSON.parse(configSchema);
    } catch (e) {
      return {};
    }
  },

  // Parse plugin config from JSON string
  parseConfig: (config: string): Record<string, any> => {
    try {
      return JSON.parse(config);
    } catch (e) {
      return {};
    }
  },

  // Get plugin category display info
  getCategoryInfo: (category: string) => {
    const categories: Record<string, { label: string; color: string }> = {
      payment: { label: "Payment Processing", color: "primary" },
      analytics: { label: "Analytics & Reporting", color: "secondary" },
      integration: { label: "Third-party Integration", color: "success" },
      reporting: { label: "Tax & Compliance", color: "warning" },
      marketing: { label: "Marketing & CRM", color: "danger" },
    };
    return categories[category] || { label: category, color: "default" };
  },

  // Get translated content for a plugin field
  getTranslatedContent: (plugin: Plugin, fieldName: string, languageCode: Locale = defaultLocale): string => {
    // If no translations or requesting English, return original content
    if (!plugin.translations || languageCode === 'en') {
      switch (fieldName) {
        case 'display_name': return plugin.display_name;
        case 'description': return plugin.description;
        case 'message': return plugin.message || plugin.description;
        case 'features': return plugin.features || '[]';
        default: return plugin.description;
      }
    }

    // Look for translation in the requested language, then base Spanish for
    // es-AR when only "es" rows exist, then original English catalog fields.
    const findContent = (code: string) =>
      plugin.translations?.find(
        (t) => t.language_code === code && t.field_name === fieldName,
      )?.content;

    const translation =
      findContent(languageCode) ??
      (languageCode === 'es-AR' ? findContent('es') : undefined);

    if (translation) {
      return translation;
    }

    // Fallback to original content if translation not found
    switch (fieldName) {
      case 'display_name': return plugin.display_name;
      case 'description': return plugin.description;
      case 'message': return plugin.message || plugin.description;
      case 'features': return plugin.features || '[]';
      default: return plugin.description;
    }
  },

  // Get translated features as an array
  getTranslatedFeatures: (plugin: Plugin, languageCode: Locale = defaultLocale): string[] => {
    const featuresJson = pluginUtils.getTranslatedContent(plugin, 'features', languageCode);
    try {
      return JSON.parse(featuresJson);
    } catch (e) {
      return [];
    }
  },
};

// Export all APIs
export const pluginAPI = {
  admin: adminPluginAPI,
  business: businessPluginAPI,
  protected: protectedPluginAPI,
  payment: paymentPluginAPI,
  mercadoPagoPoint: mercadoPagoPointAPI,
  mercadoPagoQR: mercadoPagoQRAPI,
  utils: pluginUtils,
};
