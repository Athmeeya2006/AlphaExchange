import type { ConnectionStatus } from '@/types/leaderboard';

type MessageHandler = (data: unknown) => void;
type StatusHandler = (status: ConnectionStatus) => void;

const WS_URL = import.meta.env.VITE_WS_URL || `ws://${location.host}/ws`;

// Singleton WebSocket manager with exponential-backoff reconnection.
class WebSocketManager {
  private ws: WebSocket | null = null;
  private messageHandlers = new Set<MessageHandler>();
  private statusHandlers = new Set<StatusHandler>();
  private backoff = 1000;
  private readonly maxBackoff = 30000;
  private shouldRun = false;

  connect(): void {
    this.shouldRun = true;
    this.open();
  }

  disconnect(): void {
    this.shouldRun = false;
    this.ws?.close();
    this.ws = null;
  }

  onMessage(h: MessageHandler): () => void {
    this.messageHandlers.add(h);
    return () => this.messageHandlers.delete(h);
  }

  onStatus(h: StatusHandler): () => void {
    this.statusHandlers.add(h);
    return () => this.statusHandlers.delete(h);
  }

  private emitStatus(s: ConnectionStatus) {
    this.statusHandlers.forEach((h) => h(s));
  }

  private open() {
    this.emitStatus('connecting');
    try {
      this.ws = new WebSocket(WS_URL);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws.onopen = () => {
      this.backoff = 1000;
      this.emitStatus('connected');
    };
    this.ws.onmessage = (ev) => {
      try {
        this.messageHandlers.forEach((h) => h(JSON.parse(ev.data)));
      } catch {
        /* ignore malformed */
      }
    };
    this.ws.onclose = () => {
      this.emitStatus('disconnected');
      this.scheduleReconnect();
    };
    this.ws.onerror = () => this.ws?.close();
  }

  private scheduleReconnect() {
    if (!this.shouldRun) return;
    setTimeout(() => this.open(), this.backoff);
    this.backoff = Math.min(this.backoff * 2, this.maxBackoff);
  }
}

export const wsManager = new WebSocketManager();
