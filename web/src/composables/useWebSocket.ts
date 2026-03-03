import { ref, onUnmounted } from "vue";

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

function getWSUrl(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/mock/ws`;
}

function connect() {
  if (socket && (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN)) {
    return;
  }

  socket = new WebSocket(getWSUrl());

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
    connect();
  }, delay);
}

// Initialize connection on first import
connect();

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
