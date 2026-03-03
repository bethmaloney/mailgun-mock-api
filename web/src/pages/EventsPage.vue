<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import type { Column } from "@/components/DataTable.vue";
import { useWebSocket } from "@/composables/useWebSocket";

interface Domain {
  name: string;
  state: string;
}

interface DomainsResponse {
  items: Domain[];
  total_count: number;
}

interface EventItem {
  id: string;
  event: string;
  timestamp: number;
  recipient: string;
  severity?: string;
  reason?: string;
  method?: string;
  message?: {
    headers?: {
      from?: string;
      to?: string;
      subject?: string;
      "message-id"?: string;
    };
  };
  storage?: {
    key?: string;
    url?: string;
  };
  [key: string]: unknown;
}

interface EventsPaging {
  next?: string;
  previous?: string;
}

interface EventsResponse {
  items: EventItem[];
  paging: EventsPaging;
}

const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const events = ref<EventItem[]>([]);
const paging = ref<EventsPaging>({});
const loading = ref(false);
const domainsLoading = ref(true);
const error = ref<string | null>(null);
const expandedEventId = ref<string | null>(null);

// Filters
const filterEventType = ref("");
const filterRecipient = ref("");

const columns: Column[] = [
  { key: "event", label: "Event Type" },
  { key: "recipient", label: "Recipient" },
  { key: "subject", label: "Subject" },
  { key: "domain", label: "Domain" },
  { key: "timestamp", label: "Timestamp" },
  { key: "severity", label: "Severity" },
];

const eventTypes = [
  "",
  "accepted",
  "delivered",
  "failed",
  "rejected",
  "opened",
  "clicked",
  "unsubscribed",
  "complained",
];

function formatTimestamp(ts: number): string {
  if (!ts) return "-";
  const date = new Date(ts * 1000);
  return date.toLocaleString();
}

const tableRows = computed(() =>
  events.value.map((ev) => ({
    id: ev.id,
    event: ev.event || "",
    recipient: ev.recipient || "",
    subject: ev.message?.headers?.subject || "-",
    domain: selectedDomain.value,
    timestamp: formatTimestamp(ev.timestamp),
    severity: ev.severity || "-",
    _raw: ev,
  }))
);

const hasNext = computed(() => !!paging.value.next);
const hasPrevious = computed(() => !!paging.value.previous);

async function fetchDomains() {
  domainsLoading.value = true;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
      await fetchEvents();
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

function buildEventsUrl(pagingUrl?: string): string {
  if (pagingUrl) {
    // The paging URL from the API is a full URL; extract the path + query
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  const params = new URLSearchParams();
  if (filterEventType.value) params.set("event", filterEventType.value);
  if (filterRecipient.value) params.set("recipient", filterRecipient.value);
  const qs = params.toString();
  return `/v3/${encodeURIComponent(selectedDomain.value)}/events${qs ? "?" + qs : ""}`;
}

async function fetchEvents(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  expandedEventId.value = null;
  try {
    const url = buildEventsUrl(pagingUrl);
    const resp = await api.get<EventsResponse>(url);
    events.value = resp.items || [];
    paging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load events";
  } finally {
    loading.value = false;
  }
}

function onDomainChange() {
  fetchEvents();
}

function applyFilters() {
  fetchEvents();
}

function loadNext() {
  if (paging.value.next) fetchEvents(paging.value.next);
}

function loadPrev() {
  if (paging.value.previous) fetchEvents(paging.value.previous);
}

function toggleExpand(row: Record<string, unknown>) {
  const ev = row._raw as EventItem;
  if (expandedEventId.value === ev.id) {
    expandedEventId.value = null;
  } else {
    expandedEventId.value = ev.id;
  }
}

function getExpandedEvent(): EventItem | null {
  if (!expandedEventId.value) return null;
  return events.value.find((ev) => ev.id === expandedEventId.value) || null;
}

const { onMessage } = useWebSocket();

onMessage((msg) => {
  if (msg.type === "event.new") {
    fetchEvents();
  }
});

onMounted(() => fetchDomains());
</script>

<template>
  <div class="page">
    <h1>Events</h1>

    <!-- Domain Selector -->
    <div class="controls">
      <div class="domain-selector">
        <label for="domain-select">Domain:</label>
        <select
          id="domain-select"
          v-model="selectedDomain"
          class="select-input"
          @change="onDomainChange"
        >
          <option
            v-if="domainsLoading"
            value=""
          >
            Loading domains...
          </option>
          <option
            v-else-if="domains.length === 0"
            value=""
          >
            No domains available
          </option>
          <option
            v-for="d in domains"
            :key="d.name"
            :value="d.name"
          >
            {{ d.name }}
          </option>
        </select>
      </div>

      <!-- Filters -->
      <div class="filters">
        <select
          v-model="filterEventType"
          class="select-input"
        >
          <option value="">
            All Event Types
          </option>
          <option
            v-for="et in eventTypes.filter((e) => e)"
            :key="et"
            :value="et"
          >
            {{ et }}
          </option>
        </select>
        <input
          v-model="filterRecipient"
          type="text"
          placeholder="Recipient"
          class="filter-input"
          @keyup.enter="applyFilters"
        >
        <button
          class="btn btn-primary"
          :disabled="!selectedDomain"
          @click="applyFilters"
        >
          Filter
        </button>
      </div>
    </div>

    <div
      v-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <div
      v-if="!selectedDomain && !domainsLoading"
      class="info"
    >
      No domains configured. Create a domain first to view events.
    </div>

    <template v-if="selectedDomain">
      <DataTable
        :columns="columns"
        :rows="tableRows"
        :loading="loading"
      >
        <template #cell-event="{ value, row }">
          <a
            href="#"
            class="event-link"
            @click.prevent="toggleExpand(row)"
          >
            <StatusBadge :status="String(value)" />
          </a>
        </template>
        <template #cell-severity="{ value }">
          <span
            v-if="value && value !== '-'"
            class="severity"
            :class="value === 'permanent' ? 'severity-permanent' : 'severity-temporary'"
          >
            {{ value }}
          </span>
          <span v-else>-</span>
        </template>
      </DataTable>

      <!-- Expanded Event Detail -->
      <div
        v-if="getExpandedEvent()"
        class="event-detail"
      >
        <div class="detail-header">
          <h3>Event Detail</h3>
          <button
            class="btn btn-sm"
            @click="expandedEventId = null"
          >
            Close
          </button>
        </div>
        <pre class="event-json">{{ JSON.stringify(getExpandedEvent(), null, 2) }}</pre>
      </div>

      <Pagination
        :has-next="hasNext"
        :has-previous="hasPrevious"
        @next="loadNext"
        @previous="loadPrev"
      />
    </template>
  </div>
</template>

<style scoped>
.controls {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: flex-end;
  margin-bottom: 1rem;
}

.domain-selector {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.domain-selector label {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
}

.select-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  min-width: 10rem;
}

.select-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.filter-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  min-width: 8rem;
}

.filter-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.btn {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  cursor: pointer;
  transition: background-color 0.15s;
}

.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #ffffff;
  border-color: var(--color-primary, #3b82f6);
}

.btn-primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-sm {
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
  background: var(--color-bg-primary, #ffffff);
}

.btn-sm:hover {
  background: var(--color-bg-hover, #f1f5f9);
}

.error {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-radius: 0.375rem;
}

.info {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border-radius: 0.375rem;
}

.event-link {
  text-decoration: none;
}

.severity {
  font-size: 0.75rem;
  font-weight: 600;
}

.severity-permanent {
  color: var(--color-badge-danger-text, #991b1b);
}

.severity-temporary {
  color: var(--color-badge-warning-text, #92400e);
}

.event-detail {
  margin-top: 1rem;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1rem;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.detail-header h3 {
  margin: 0;
  font-size: 1rem;
}

.event-json {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 0.75rem;
  font-size: 0.8125rem;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  max-height: 30rem;
  overflow-y: auto;
}
</style>
