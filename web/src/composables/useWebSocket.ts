import { ref, onUnmounted } from "vue";
import { getAccessToken } from "@/auth/msalInstance";

export interface WSMessage {
  type: string;
  data: Record<string, unknown>;
}

type MessageHandler = (msg: WSMessage) => void;

// Singleton state (shared across all component instances)
const connected = ref(false);
const handlers: MessageHandler[] = [];
let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
const maxReconnectDelay = 30000;

async function getWSUrl(): Promise<string> {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const base = `${protocol}//${window.location.host}/mock/ws`;
  const token = await getAccessToken();
  if (token) {
    return `${base}?access_token=${encodeURIComponent(token)}`;
  }
  return base;
}

async function connect() {
  if (socket && (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN)) {
    return;
  }

  const url = await getWSUrl();
  socket = new WebSocket(url);

  socket.onopen = () => {
    connected.value = true;
    reconnectAttempts = 0;
  };

  socket.onclose = () => {
    connected.value = false;
    scheduleReconnect();
  };

  socket.onerror = () => {
    // onclose will fire after onerror
  };

  socket.onmessage = (event) => {
    try {
      const msg: WSMessage = JSON.parse(event.data);
      for (const handler of handlers) {
        handler(msg);
      }
    } catch {
      // ignore malformed messages
    }
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), maxReconnectDelay);
  reconnectAttempts++;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect().catch(() => {
      // Will retry on next scheduled reconnect
    });
  }, delay);
}

export function startWebSocket() {
  connect();
}

export function useWebSocket() {
  function onMessage(handler: MessageHandler) {
    handlers.push(handler);
    // Clean up on component unmount
    onUnmounted(() => {
      const idx = handlers.indexOf(handler);
      if (idx !== -1) handlers.splice(idx, 1);
    });
  }

  return {
    connected,
    onMessage,
  };
}
