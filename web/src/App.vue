<script setup lang="ts">
import ToastNotification from "@/components/ToastNotification.vue";
import { useAuth } from "@/composables/useAuth";
import { useWebSocket } from "@/composables/useWebSocket";

const { connected } = useWebSocket();
const { user, isAuthenticated, signOut } = useAuth();
</script>

<template>
  <div class="app-layout">
    <aside class="sidebar">
      <div class="sidebar-header">
        <h1 class="sidebar-brand">
          Mailgun Mock
        </h1>
        <div
          class="connection-status"
          :class="{ 'is-connected': connected }"
        >
          <span class="status-dot" />
          <span class="status-text">{{ connected ? 'Connected' : 'Disconnected' }}</span>
        </div>
        <div
          v-if="isAuthenticated"
          class="sidebar-user"
        >
          <span class="sidebar-user-name">{{ user?.name || user?.email || 'User' }}</span>
          <button
            class="sidebar-sign-out"
            @click="signOut()"
          >
            Sign out
          </button>
        </div>
      </div>
      <nav class="sidebar-nav">
        <router-link
          to="/"
          class="nav-item"
        >
          Dashboard
        </router-link>
        <router-link
          to="/messages"
          class="nav-item"
        >
          Messages
        </router-link>
        <router-link
          to="/events"
          class="nav-item"
        >
          Events
        </router-link>
        <router-link
          to="/webhooks"
          class="nav-item"
        >
          Webhooks
        </router-link>
        <router-link
          to="/domains"
          class="nav-item"
        >
          Domains
        </router-link>
        <router-link
          to="/templates"
          class="nav-item"
        >
          Templates
        </router-link>
        <router-link
          to="/mailing-lists"
          class="nav-item"
        >
          Mailing Lists
        </router-link>
        <router-link
          to="/routes"
          class="nav-item"
        >
          Routes
        </router-link>
        <router-link
          to="/suppressions"
          class="nav-item"
        >
          Suppressions
        </router-link>

        <div class="nav-section-header">
          Testing
        </div>
        <router-link
          to="/trigger-events"
          class="nav-item"
        >
          Trigger Events
        </router-link>
        <router-link
          to="/simulate-inbound"
          class="nav-item"
        >
          Simulate Inbound
        </router-link>

        <div class="nav-section-header">
          Config
        </div>
        <router-link
          to="/settings"
          class="nav-item"
        >
          Settings
        </router-link>
      </nav>
    </aside>

    <main class="main-content">
      <router-view />
    </main>

    <ToastNotification />
  </div>
</template>

<style>
/* -------------------------------------------------------
   CSS Custom Properties (theming)
   ------------------------------------------------------- */
:root {
  /* Sidebar */
  --sidebar-bg: #1e293b;
  --sidebar-text: #cbd5e1;
  --sidebar-text-active: #f8fafc;
  --sidebar-hover-bg: #334155;
  --sidebar-width: 15rem;
  --sidebar-brand-text: #f8fafc;
  --sidebar-section-text: #64748b;

  /* Main */
  --color-bg-page: #f1f5f9;
  --color-bg-primary: #ffffff;
  --color-bg-subtle: #f8fafc;
  --color-bg-hover: #f1f5f9;

  /* Text */
  --color-text-primary: #1e293b;
  --color-text-secondary: #64748b;

  /* Borders */
  --color-border: #e2e8f0;
  --color-border-hover: #cbd5e1;

  /* Badges */
  --color-badge-success-bg: #dcfce7;
  --color-badge-success-text: #166534;
  --color-badge-info-bg: #dbeafe;
  --color-badge-info-text: #1e40af;
  --color-badge-danger-bg: #fee2e2;
  --color-badge-danger-text: #991b1b;
  --color-badge-warning-bg: #fef3c7;
  --color-badge-warning-text: #92400e;
  --color-badge-default-bg: #f1f5f9;
  --color-badge-default-text: #475569;
}

/* -------------------------------------------------------
   Global reset
   ------------------------------------------------------- */
*,
*::before,
*::after {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    "Helvetica Neue", Arial, "Noto Sans", sans-serif;
  color: var(--color-text-primary);
  background: var(--color-bg-page);
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}

/* -------------------------------------------------------
   Layout
   ------------------------------------------------------- */
.app-layout {
  display: flex;
  min-height: 100vh;
}

/* -------------------------------------------------------
   Sidebar
   ------------------------------------------------------- */
.sidebar {
  width: var(--sidebar-width);
  background: var(--sidebar-bg);
  color: var(--sidebar-text);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  overflow-y: auto;
}

.sidebar-header {
  padding: 1.25rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.sidebar-brand {
  font-size: 1.125rem;
  font-weight: 700;
  color: var(--sidebar-brand-text);
  letter-spacing: -0.01em;
}

.sidebar-nav {
  padding: 0.5rem 0;
  display: flex;
  flex-direction: column;
}

.nav-item {
  display: block;
  padding: 0.5rem 1rem;
  color: var(--sidebar-text);
  text-decoration: none;
  font-size: 0.875rem;
  transition: background-color 0.15s, color 0.15s;
  border-left: 3px solid transparent;
}

.nav-item:hover {
  background: var(--sidebar-hover-bg);
  color: var(--sidebar-text-active);
}

.nav-item.router-link-exact-active {
  color: var(--sidebar-text-active);
  background: var(--sidebar-hover-bg);
  border-left-color: #3b82f6;
  font-weight: 600;
}

.nav-section-header {
  padding: 1rem 1rem 0.375rem;
  font-size: 0.6875rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--sidebar-section-text);
}

/* -------------------------------------------------------
   Main content
   ------------------------------------------------------- */
.main-content {
  flex: 1;
  margin-left: var(--sidebar-width);
  padding: 1.5rem 2rem;
  min-height: 100vh;
}

/* -------------------------------------------------------
   Page-level typography shared by all pages
   ------------------------------------------------------- */
.page h1 {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 1rem;
  color: var(--color-text-primary);
}

.page p {
  color: var(--color-text-secondary);
}

/* -------------------------------------------------------
   WebSocket connection status indicator
   ------------------------------------------------------- */
.connection-status {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding-top: 0.5rem;
}

.status-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  background: #ef4444;
  flex-shrink: 0;
}

.connection-status.is-connected .status-dot {
  background: #22c55e;
}

.status-text {
  font-size: 0.6875rem;
  color: var(--sidebar-section-text);
}

/* -------------------------------------------------------
   Sidebar user block
   ------------------------------------------------------- */
.sidebar-user {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 0.5rem;
  gap: 0.5rem;
}

.sidebar-user-name {
  font-size: 0.75rem;
  color: var(--sidebar-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-sign-out {
  background: none;
  border: none;
  color: var(--sidebar-section-text);
  font-size: 0.6875rem;
  cursor: pointer;
  padding: 0;
  flex-shrink: 0;
  transition: color 0.15s;
}

.sidebar-sign-out:hover {
  color: var(--sidebar-text-active);
}
</style>
