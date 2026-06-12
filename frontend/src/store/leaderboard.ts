import { create } from 'zustand';
import type { ConnectionStatus, LeaderboardEntry } from '@/types/leaderboard';
import { getLeaderboard } from '@/lib/api';
import { wsManager } from '@/lib/websocket';

export interface TickerEvent {
  message: string;
  priority: number;
  created_at: number;
}

interface LeaderboardState {
  entries: LeaderboardEntry[];
  lastUpdated: number;
  connectionStatus: ConnectionStatus;
  ticker: TickerEvent[];
  setEntries: (entries: LeaderboardEntry[]) => void;
  pushTicker: (e: TickerEvent) => void;
  setConnectionStatus: (s: ConnectionStatus) => void;
  fetchLeaderboard: () => Promise<void>;
  connectWebSocket: () => void;
}

export const useLeaderboardStore = create<LeaderboardState>((set, get) => ({
  entries: [],
  lastUpdated: 0,
  connectionStatus: 'disconnected',
  ticker: [],

  setEntries: (incoming) => {
    const prev = get().entries;
    const prevRank = new Map(prev.map((e) => [e.contestant_id, e.rank]));
    const entries = incoming.map((e) => {
      const before = prevRank.get(e.contestant_id);
      return {
        ...e,
        previousRank: before,
        rankChange: before !== undefined ? before - e.rank : 0,
      };
    });
    set({ entries, lastUpdated: Date.now() });
  },

  pushTicker: (e) => set({ ticker: [e, ...get().ticker].slice(0, 30) }),

  setConnectionStatus: (connectionStatus) => set({ connectionStatus }),

  fetchLeaderboard: async () => {
    try {
      const res = await getLeaderboard();
      get().setEntries(res.entries || []);
    } catch {
      /* keep stale data */
    }
  },

  connectWebSocket: () => {
    wsManager.onStatus((s) => get().setConnectionStatus(s));
    wsManager.onMessage((data) => {
      const msg = data as { type?: string; entries?: LeaderboardEntry[]; message?: string; priority?: number; created_at?: number };
      if (msg?.type === 'ticker_event' && msg.message) {
        get().pushTicker({ message: msg.message, priority: msg.priority ?? 1, created_at: msg.created_at ?? Date.now() });
      } else if (msg?.entries) {
        get().setEntries(msg.entries);
      }
    });
    wsManager.connect();
  },
}));
