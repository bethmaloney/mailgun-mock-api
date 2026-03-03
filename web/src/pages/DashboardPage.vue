<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
import type { Column } from "@/components/DataTable.vue";

interface WebhookDelivery {
  url: string;
  event: string;
  status_code: number;
  timestamp: number;
}

interface DashboardData {
  messages: { total: number; last_hour: number };
  events: {
    accepted: number;
    delivered: number;
    failed: number;
    opened: number;
    clicked: number;
    complained: number;
    unsubscribed: number;
  };
  domains: { total: number; active: number; unverified: number };
  webhooks: {
    configured: number;
    recent_deliveries: WebhookDelivery[];
  };
}

const data = ref<DashboardData | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);

const deliveryColumns: Column[] = [
  { key: "url", label: "URL" },
  { key: "event", label: "Event" },
  { key: "status_code", label: "Status" },
  { key: "time", label: "Time" },
];

function formatTimestamp(ts: number): string {
  if (!ts) return "-";
  const date = new Date(ts * 1000);
  return date.toLocaleString();
}

function deliveryRows(deliveries: WebhookDelivery[]): Record<string, unknown>[] {
  return deliveries.map((d) => ({
    url: d.url,
    event: d.event,
    status_code: d.status_code,
    time: formatTimestamp(d.timestamp),
  }));
}

onMounted(async () => {
  try {
    data.value = await api.get<DashboardData>("/mock/dashboard");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load dashboard data";
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="page">
    <h1>Dashboard</h1>

    <div
      v-if="loading"
      class="loading"
    >
      Loading dashboard data...
    </div>

    <div
      v-else-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <template v-else-if="data">
      <!-- Summary Cards -->
      <div class="card-grid">
        <!-- Messages Card -->
        <div class="card">
          <h2 class="card-title">
            Messages
          </h2>
          <div class="card-body">
            <div class="stat-row">
              <span class="stat-label">Total</span>
              <span class="stat-value">{{ data.messages.total }}</span>
            </div>
            <div class="stat-row">
              <span class="stat-label">Last Hour</span>
              <span class="stat-value">{{ data.messages.last_hour }}</span>
            </div>
          </div>
        </div>

        <!-- Domains Card -->
        <div class="card">
          <h2 class="card-title">
            Domains
          </h2>
          <div class="card-body">
            <div class="stat-row">
              <span class="stat-label">Total</span>
              <span class="stat-value">{{ data.domains.total }}</span>
            </div>
            <div class="stat-row">
              <span class="stat-label">Active</span>
              <span class="stat-value">{{ data.domains.active }}</span>
            </div>
            <div class="stat-row">
              <span class="stat-label">Unverified</span>
              <span class="stat-value">{{ data.domains.unverified }}</span>
            </div>
          </div>
        </div>

        <!-- Webhooks Card -->
        <div class="card">
          <h2 class="card-title">
            Webhooks
          </h2>
          <div class="card-body">
            <div class="stat-row">
              <span class="stat-label">Configured</span>
              <span class="stat-value">{{ data.webhooks.configured }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Events Card (full width) -->
      <div class="card card-full">
        <h2 class="card-title">
          Events
        </h2>
        <div class="card-body">
          <div class="event-grid">
            <div class="event-stat">
              <StatusBadge
                status="accepted"
                type="event"
              />
              <span class="event-count">{{ data.events.accepted }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="delivered"
                type="event"
              />
              <span class="event-count">{{ data.events.delivered }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="failed"
                type="event"
              />
              <span class="event-count">{{ data.events.failed }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="opened"
                type="event"
              />
              <span class="event-count">{{ data.events.opened }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="clicked"
                type="event"
              />
              <span class="event-count">{{ data.events.clicked }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="complained"
                type="event"
              />
              <span class="event-count">{{ data.events.complained }}</span>
            </div>
            <div class="event-stat">
              <StatusBadge
                status="unsubscribed"
                type="event"
              />
              <span class="event-count">{{ data.events.unsubscribed }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Webhook Deliveries -->
      <div class="card card-full">
        <h2 class="card-title">
          Recent Webhook Deliveries
        </h2>
        <div class="card-body">
          <DataTable
            :columns="deliveryColumns"
            :rows="deliveryRows(data.webhooks.recent_deliveries)"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.loading,
.error {
  padding: 2rem;
  text-align: center;
  color: var(--color-text-secondary, #64748b);
}

.error {
  color: var(--color-badge-danger-text, #991b1b);
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: 1rem;
  margin-bottom: 1rem;
}

.card {
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  overflow: hidden;
}

.card-full {
  margin-bottom: 1rem;
}

.card-title {
  font-size: 0.875rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-secondary, #64748b);
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  margin: 0;
}

.card-body {
  padding: 1rem;
}

.stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.375rem 0;
}

.stat-row + .stat-row {
  border-top: 1px solid var(--color-border, #e2e8f0);
}

.stat-label {
  color: var(--color-text-secondary, #64748b);
  font-size: 0.875rem;
}

.stat-value {
  font-size: 1.125rem;
  font-weight: 700;
  color: var(--color-text-primary, #1e293b);
}

.event-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
  gap: 0.75rem;
}

.event-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.event-count {
  font-size: 1.125rem;
  font-weight: 700;
  color: var(--color-text-primary, #1e293b);
}
</style>
