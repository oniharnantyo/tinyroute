import {
  OverviewData,
  ProviderDetail,
  ProviderItem,
  PresetItem,
  ComboItem,
  HistoryRecord,
  KeyItem,
  ClientItem,
  ServerSettings,
  TimeWindow,
} from '@/types/api';

class ApiClient {
  private async request<T>(endpoint: string, options?: RequestInit): Promise<T> {
    const res = await fetch(endpoint, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
      credentials: 'same-origin',
    });

    if (res.status === 401 && !endpoint.includes('/auth/')) {
      window.dispatchEvent(new CustomEvent('auth-unauthorized'));
    }

    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error(data.error || `HTTP error ${res.status}`);
    }
    return data as T;
  }

  // Auth
  async getAuthStatus(): Promise<{ authenticated: boolean; is_default_password: boolean }> {
    return this.request('/dashboard/api/auth/status');
  }

  async login(password: string): Promise<{ success: boolean; error?: string }> {
    return this.request('/dashboard/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    });
  }

  async logout(): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/auth/logout', { method: 'POST' });
  }

  // Overview
  async getOverview(window: TimeWindow = '24h'): Promise<OverviewData> {
    return this.request(`/dashboard/api/overview?window=${window}`);
  }

  // Providers
  async getProviders(): Promise<{ providers: ProviderItem[]; presets: PresetItem[] }> {
    return this.request('/dashboard/api/providers');
  }

  async getProviderDetail(name: string): Promise<ProviderDetail> {
    return this.request(`/dashboard/api/providers/${encodeURIComponent(name)}`);
  }

  async addProvider(name: string, presetName: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/add', {
      method: 'POST',
      body: JSON.stringify({ name, preset: presetName }),
    });
  }

  async addCustomProvider(data: {
    name: string;
    dialect: string;
    base_url: string;
    api_key?: string;
    models?: string[];
  }): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/add-custom', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteProvider(name: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/delete', {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  }

  async saveCredential(
    provider: string,
    account: string,
    apiKey: string
  ): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/credential', {
      method: 'POST',
      body: JSON.stringify({ provider, account, api_key: apiKey }),
    });
  }

  async deleteCredential(provider: string, account: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/credential/delete', {
      method: 'POST',
      body: JSON.stringify({ provider, account }),
    });
  }

  async renameAccount(provider: string, oldName: string, newName: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/providers/account/rename', {
      method: 'POST',
      body: JSON.stringify({ provider, old_name: oldName, new_name: newName }),
    });
  }

  async addModel(provider: string, model: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/models/add', {
      method: 'POST',
      body: JSON.stringify({ provider, model }),
    });
  }

  async removeModel(provider: string, model: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/models/remove', {
      method: 'POST',
      body: JSON.stringify({ provider, model }),
    });
  }

  async testModelProbe(
    provider: string,
    model: string,
    dialect: string
  ): Promise<{ status_code: number; duration_ms: number; error?: string; success: boolean }> {
    return this.request('/dashboard/api/models/test', {
      method: 'POST',
      body: JSON.stringify({ provider, model, dialect }),
    });
  }

  // Combos
  async getCombos(): Promise<{
    combos: ComboItem[];
    available_models: { provider: string; model: string }[];
    provider_accounts?: Record<string, string[]>;
  }> {
    return this.request('/dashboard/api/combos');
  }

  async saveCombo(data: {
    name: string;
    mode?: string;
    members: string[];
    capabilities?: string[];
  }): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/combos/wizard', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async toggleCombo(name: string, enabled: boolean): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/combos/toggle', {
      method: 'POST',
      body: JSON.stringify({ name, enabled }),
    });
  }

  async deleteCombo(name: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/combos/delete', {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  }

  // History
  async getHistory(params?: {
    limit?: number;
    offset?: number;
    provider?: string;
    key?: string;
    model?: string;
    search?: string;
    from?: string;
    to?: string;
  }): Promise<{ records: HistoryRecord[]; total: number }> {
    const q = new URLSearchParams();
    if (params?.limit) q.set('limit', params.limit.toString());
    if (params?.offset) q.set('offset', params.offset.toString());
    if (params?.provider) q.set('provider', params.provider);
    if (params?.key) q.set('key', params.key);
    if (params?.model) q.set('model', params.model);
    if (params?.search) q.set('search', params.search);
    if (params?.from) q.set('from', params.from);
    if (params?.to) q.set('to', params.to);
    return this.request(`/dashboard/api/history?${q.toString()}`);
  }

  async getHistoryDetail(id: string): Promise<HistoryRecord> {
    return this.request(`/dashboard/api/history/${id}`);
  }

  // Keys
  async getKeys(): Promise<{ keys: KeyItem[] }> {
    return this.request('/dashboard/api/keys');
  }

  async createKey(
    name: string,
    rpm?: number,
    rpd?: number,
    expiresAt?: string
  ): Promise<{ key: string; item: KeyItem }> {
    return this.request('/dashboard/api/keys/create', {
      method: 'POST',
      body: JSON.stringify({ name, rate_limit_rpm: rpm, rate_limit_rpd: rpd, expires_at: expiresAt }),
    });
  }

  async revokeKey(id: string): Promise<{ success: boolean }> {
    return this.request(`/dashboard/api/keys/${id}/revoke`, { method: 'POST' });
  }

  async updateKey(
    id: string,
    data: { name: string; expires_at?: string; rate_limit_rpm?: number; rate_limit_rpd?: number }
  ): Promise<{ success: boolean }> {
    return this.request(`/dashboard/api/keys/${id}/update`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // Clients
  async getClients(): Promise<{ clients: ClientItem[] }> {
    return this.request('/dashboard/api/clients');
  }

  async getClientDetail(id: string): Promise<ClientItem> {
    return this.request(`/dashboard/api/clients/${id}`);
  }

  async planClient(
    id: string,
    payload: Record<string, any>
  ): Promise<{ plan: any; preview: string }> {
    return this.request(`/dashboard/api/clients/${id}/plan`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async applyClient(
    id: string,
    payload: Record<string, any>
  ): Promise<{ success: boolean; message: string; minted_key?: string; files?: string[] }> {
    return this.request(`/dashboard/api/clients/${id}/apply`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async resetClient(id: string): Promise<{ success: boolean; message: string }> {
    return this.request(`/dashboard/api/clients/${id}/reset`, { method: 'POST' });
  }

  // Settings
  async getSettings(): Promise<ServerSettings> {
    return this.request('/dashboard/api/settings');
  }

  async changePassword(oldPassword: string, newPassword: string): Promise<{ success: boolean }> {
    return this.request('/dashboard/api/settings/password', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
  }
}

export const api = new ApiClient();
