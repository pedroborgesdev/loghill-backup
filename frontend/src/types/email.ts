import type { EmailProviderType } from "./alert";

export interface OutlookSettings {
  tenant_id: string;
  client_id: string;
  client_secret_configured: boolean;
  sender_email: string;
  sender_name: string;
  managed_by_environment: boolean;
}

export interface EmailSettings {
  provider: EmailProviderType;
  enabled: boolean;
  configured: boolean;
  outlook: OutlookSettings;
  providers: Array<{ id: EmailProviderType; enabled: boolean; available: boolean }>;
  updated_at: string;
  last_test_at: string | null;
  last_test_status?: "success" | "failed";
  last_test_error?: string;
}

export interface EmailSettingsInput {
  provider: EmailProviderType;
  enabled: boolean;
  outlook: {
    tenant_id: string;
    client_id: string;
    client_secret?: string;
    sender_email: string;
    sender_name: string;
  };
}

