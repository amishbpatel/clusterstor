import { browser } from '$app/environment';

const API_BASE = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '');
const TOKEN_KEY = 'clusterstor.session';

export type User = {
  id: string;
  email: string;
  display_name?: string | null;
  created_at: string;
};

export type Session = {
  token: string;
  expires_at: string;
  user: User;
};

export type ProviderAccount = {
  id: string;
  provider: string;
  external_account_id?: string | null;
  display_name?: string | null;
  status: string;
  quota_total_bytes?: number | null;
  quota_used_bytes?: number | null;
  quota_free_bytes?: number | null;
  last_synced_at?: string | null;
  managed_root_ready?: boolean;
};

export type FileItem = {
  node_id: string;
  provider: string;
  provider_item_id: string;
  parent_item_id?: string | null;
  name: string;
  node_type: 'file' | 'folder';
  mime_type?: string;
  size_bytes?: number | null;
  modified_at?: string | null;
};

export type DriveItem = FileItem;

export type UploadCapacity = {
  provider: string;
  requested_bytes: number;
  total_bytes?: number | null;
  used_bytes?: number | null;
  free_bytes?: number | null;
  allowed: boolean;
};

export function getToken(): string | null {
  return browser ? localStorage.getItem(TOKEN_KEY) : null;
}

export function setToken(token: string) {
  if (browser) localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  if (browser) localStorage.removeItem(TOKEN_KEY);
}

export async function api<T>(path: string, init: RequestInit = {}, auth = true): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body) headers.set('Content-Type', 'application/json');
  if (auth) {
    const token = getToken();
    if (!token) throw new Error('You are not signed in.');
    headers.set('Authorization', `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  if (response.status === 401 && auth) clearToken();

  if (!response.ok) {
    let message = `Request failed (${response.status})`;
    try {
      const body = await response.json();
      message = body.message || message;
    } catch {}
    throw new Error(message);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export async function signup(email: string, password: string, displayName?: string) {
  const session = await api<Session>('/api/v1/auth/signup', {
    method: 'POST',
    body: JSON.stringify({ email, password, display_name: displayName || null })
  }, false);
  setToken(session.token);
  return session;
}

export async function login(email: string, password: string) {
  const session = await api<Session>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password })
  }, false);
  setToken(session.token);
  return session;
}

export async function logout() {
  try {
    await api<void>('/api/v1/auth/logout', { method: 'POST' });
  } finally {
    clearToken();
  }
}

export function formatBytes(value?: number | null) {
  if (value == null) return '—';
  if (value === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024, index)).toFixed(index >= 3 ? 1 : 0)} ${units[index]}`;
}

export { API_BASE };

export async function openEventSocket(onEventsAvailable: (sequence: number) => void) {
  const ticket = await api<{ ticket: string }>('/api/v1/events/socket-ticket', { method: 'POST' });
  const base = new URL(API_BASE || window.location.origin, window.location.origin);
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
  base.pathname = '/api/v1/events/socket';
  base.search = new URLSearchParams({ ticket: ticket.ticket }).toString();

  const socket = new WebSocket(base.toString());
  socket.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      if (message.type === 'events_available' && typeof message.sequence === 'number') {
        onEventsAvailable(message.sequence);
      }
    } catch {}
  };
  return socket;
}
