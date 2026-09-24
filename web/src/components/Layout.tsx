import React, { useState, useEffect } from 'react';
import {
  LayoutDashboard,
  Cpu,
  Layers,
  History,
  Key,
  Terminal,
  Settings,
  LogOut,
  Route,
  TriangleAlert,
} from 'lucide-react';
import { api } from '@/lib/api';

interface Props {
  activeTab: string;
  onSelectTab: (tab: string) => void;
  onLogout: () => void;
  children: React.ReactNode;
}

export const Layout: React.FC<Props> = ({ activeTab, onSelectTab, onLogout, children }) => {
  const [isDefaultPassword, setIsDefaultPassword] = useState(false);

  useEffect(() => {
    api.getAuthStatus().then((res) => {
      setIsDefaultPassword(res.is_default_password);
    }).catch(() => {});
  }, []);

  const navGroups = [
    {
      group: 'Routing',
      items: [
        { id: 'overview', label: 'Overview', icon: LayoutDashboard, badge: null },
        { id: 'providers', label: 'Providers', icon: Cpu, badge: null },
        { id: 'combos', label: 'Combos', icon: Layers, badge: null },
      ],
    },
    {
      group: 'Observability',
      items: [
        { id: 'history', label: 'History', icon: History, badge: null },
        { id: 'keys', label: 'API Keys', icon: Key, badge: null },
      ],
    },
    {
      group: 'Integration',
      items: [
        { id: 'clients', label: 'Coding Clients', icon: Terminal, badge: null },
        { id: 'settings', label: 'Settings', icon: Settings, badge: isDefaultPassword ? 'Alert' : null },
      ],
    },
  ];

  return (
    <div className="min-h-screen text-foreground bg-background font-sans flex flex-col md:flex-row antialiased">
      {/* Mobile top header */}
      <div className="md:hidden flex items-center justify-between bg-background border-b border-border px-4 py-3 sticky top-0 z-40">
        <div className="flex items-center gap-2.5">
          <div className="p-1.5 bg-accent text-accent-foreground rounded-md border border-border">
            <Route className="size-4" />
          </div>
          <span className="font-semibold tracking-tight text-foreground">tinyroute</span>
        </div>
        <button
          onClick={onLogout}
          className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1.5 font-medium cursor-pointer"
        >
          <LogOut className="size-4" />
          Logout
        </button>
      </div>

      {/* Sidebar */}
      <aside className="w-full md:w-60 bg-background md:border-r border-border flex flex-col justify-between shrink-0 z-30">
        <div>
          {/* Brand */}
          <div className="hidden md:flex items-center gap-3 p-5 border-b border-border">
            <div className="p-2.5 bg-accent text-accent-foreground rounded-md border border-border">
              <Route className="size-5" />
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <h1 className="font-semibold text-base tracking-tight text-foreground">tinyroute</h1>
                <span className="text-[10px] font-mono px-1.5 rounded bg-muted border border-border text-muted-foreground">
                  v0.1
                </span>
              </div>
              <p className="text-[11px] text-muted-foreground mt-0.5">LLM proxy gateway</p>
            </div>
          </div>

          {/* Navigation */}
          <nav className="p-3.5 space-y-5">
            {navGroups.map((group) => (
              <div key={group.group} className="space-y-1">
                <h2 className="px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                  {group.group}
                </h2>
                <div className="space-y-0.5 pt-1">
                  {group.items.map((item) => {
                    const Icon = item.icon;
                    const isActive = activeTab === item.id;
                    return (
                      <button
                        key={item.id}
                        onClick={() => onSelectTab(item.id)}
                        className={`w-full flex items-center justify-between px-3 py-2 rounded-md text-xs font-medium transition-colors text-left group cursor-pointer ${
                          isActive
                            ? 'bg-secondary text-foreground'
                            : 'text-muted-foreground hover:text-foreground hover:bg-secondary/60'
                        }`}
                      >
                        <div className="flex items-center gap-2.5">
                          <Icon
                            className={`size-4 transition-colors ${
                              isActive ? 'text-accent-foreground' : 'text-muted-foreground group-hover:text-foreground'
                            }`}
                          />
                          <span>{item.label}</span>
                        </div>
                        {item.badge && (
                          <span
                            className={`text-[10px] font-mono px-1.5 rounded uppercase ${
                              item.badge === 'Alert'
                                ? 'bg-warning/10 text-warning border border-warning/30'
                                : 'bg-muted text-muted-foreground border border-border'
                            }`}
                          >
                            {item.badge}
                          </span>
                        )}
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
        </div>

        {/* Sidebar footer */}
        <div className="p-3.5 border-t border-border space-y-3">
          {isDefaultPassword && (
            <div className="p-3 bg-warning/10 border border-warning/30 rounded-md text-xs text-warning flex items-start gap-2.5">
              <TriangleAlert className="size-4 shrink-0 mt-0.5" />
              <div>
                <p className="font-semibold">Default password active</p>
                <p className="text-warning/90 mt-0.5">
                  Change it in{' '}
                  <button
                    onClick={() => onSelectTab('settings')}
                    className="underline hover:text-foreground font-medium cursor-pointer"
                  >
                    Settings
                  </button>
                  .
                </p>
              </div>
            </div>
          )}

          <div className="flex items-center justify-end px-1.5">
            <button
              onClick={onLogout}
              className="text-[11px] font-medium text-muted-foreground hover:text-destructive-text flex items-center gap-1 transition-colors cursor-pointer"
              title="End session"
            >
              <LogOut className="size-3.5" />
              <span>Logout</span>
            </button>
          </div>
        </div>
      </aside>

      {/* Main workspace */}
      <div className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Topbar */}
        <header className="bg-background border-b border-border px-6 py-3 flex items-center justify-between gap-4 sticky top-0 z-20">
          <div className="text-xs font-mono text-muted-foreground flex items-center gap-2">
            <span className="text-muted-foreground">gateway</span>
            <span className="text-muted-foreground/60">/</span>
            <span className="text-foreground capitalize font-medium">{activeTab}</span>
          </div>

          {/* Daemon status — the app's single live indicator */}
          <div className="flex items-center gap-2 text-[11px] font-mono text-muted-foreground">
            <span className="size-1.5 rounded-full bg-success animate-pulse" />
            <span>127.0.0.1:8787</span>
          </div>
        </header>

        {/* Content */}
        <main className="p-6 lg:p-10 max-w-7xl w-full mx-auto space-y-6">
          {children}
        </main>
      </div>
    </div>
  );
};
