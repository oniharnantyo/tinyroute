import { useEffect, useState } from 'react';
import { Layout } from '@/components/Layout';
import { OverviewPage } from '@/pages/OverviewPage';
import { ProvidersPage } from '@/pages/ProvidersPage';
import { CombosPage } from '@/pages/CombosPage';
import { HistoryPage } from '@/pages/HistoryPage';
import { KeysPage } from '@/pages/KeysPage';
import { ClientsPage } from '@/pages/ClientsPage';
import { SettingsPage } from '@/pages/SettingsPage';
import { LoginPage } from '@/pages/LoginPage';
import { api } from '@/lib/api';

export function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [activeTab, setActiveTab] = useState<string>('overview');
  const [providerParam, setProviderParam] = useState<string | null>(null);

  // Sync tab with URL pathname on load / navigation
  useEffect(() => {
    const path = window.location.pathname;
    if (path.includes('/providers')) {
      setActiveTab('providers');
      const parts = path.split('/providers/');
      if (parts.length > 1 && parts[1]) {
        setProviderParam(parts[1]);
      }
    } else if (path.includes('/combos')) {
      setActiveTab('combos');
    } else if (path.includes('/history')) {
      setActiveTab('history');
    } else if (path.includes('/keys')) {
      setActiveTab('keys');
    } else if (path.includes('/clients')) {
      setActiveTab('clients');
    } else if (path.includes('/settings')) {
      setActiveTab('settings');
    } else {
      setActiveTab('overview');
    }
  }, []);

  const handleTabChange = (tab: string, itemParam?: string) => {
    setActiveTab(tab);
    if (tab === 'providers' && itemParam) {
      setProviderParam(itemParam);
      window.history.pushState({}, '', `/dashboard/providers/${itemParam}`);
    } else {
      setProviderParam(null);
      window.history.pushState({}, '', `/dashboard/${tab}`);
    }
  };

  const checkAuth = async () => {
    setCheckingAuth(true);
    try {
      const status = await api.getAuthStatus();
      setAuthenticated(status.authenticated);
    } catch {
      setAuthenticated(false);
    } finally {
      setCheckingAuth(false);
    }
  };

  useEffect(() => {
    checkAuth();
    const handleUnauthorized = () => {
      setAuthenticated(false);
    };
    window.addEventListener('auth-unauthorized', handleUnauthorized);
    return () => window.removeEventListener('auth-unauthorized', handleUnauthorized);
  }, []);

  const handleLogin = async (password: string) => {
    const res = await api.login(password);
    if (res.success) {
      setAuthenticated(true);
    }
    return res;
  };

  const handleLogout = async () => {
    await api.logout();
    setAuthenticated(false);
  };

  if (checkingAuth) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center text-xs text-muted-foreground">
        <div className="flex items-center gap-2">
          <div className="size-2 rounded-full bg-primary animate-pulse" />
          <span>Connecting to the tinyroute gateway...</span>
        </div>
      </div>
    );
  }

  if (!authenticated) {
    return <LoginPage onLogin={handleLogin} />;
  }

  return (
    <Layout activeTab={activeTab} onSelectTab={handleTabChange} onLogout={handleLogout}>
      <div>
        {activeTab === 'overview' && <OverviewPage onNavigateTab={handleTabChange} />}
        {activeTab === 'providers' && (
          <ProvidersPage
            initialProvider={providerParam}
            onClearInitialProvider={() => setProviderParam(null)}
          />
        )}
        {activeTab === 'combos' && <CombosPage />}
        {activeTab === 'history' && <HistoryPage />}
        {activeTab === 'keys' && <KeysPage />}
        {activeTab === 'clients' && <ClientsPage />}
        {activeTab === 'settings' && <SettingsPage />}
      </div>
    </Layout>
  );
}

export default App;
