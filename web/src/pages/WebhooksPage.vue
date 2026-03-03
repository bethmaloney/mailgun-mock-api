<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import type { Column } from "@/components/DataTable.vue";

// --- Types ---

interface Domain {
  name: string;
  state: string;
}

interface DomainsResponse {
  items: Domain[];
  total_count: number;
}

interface WebhookEntry {
  urls: string[];
}

interface WebhooksResponse {
  webhooks: Record<string, WebhookEntry>;
}

interface WebhookMutationResponse {
  message: string;
  webhook: { urls: string[] };
}

interface DeliveryRequest {
  headers: Record<string, unknown>;
  body: Record<string, unknown>;
}

interface DeliveryResponse {
  status_code: number;
  headers: Record<string, unknown>;
  body: string;
}

interface DeliveryItem {
  id: string;
  timestamp: number;
  webhook_id: string;
  url: string;
  event_type: string;
  domain: string;
  message_id: string;
  request: DeliveryRequest;
  response: DeliveryResponse;
  response_time_ms: number;
  attempt: number;
  success: boolean;
}

interface DeliveryPaging {
  next?: string;
  previous?: string;
}

interface DeliveriesResponse {
  items: DeliveryItem[];
  paging: DeliveryPaging;
  total_count: number;
}

// --- Domain selector state ---

const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const domainsLoading = ref(true);
const error = ref<string | null>(null);

// --- Webhook config state ---

const webhooks = ref<Record<string, WebhookEntry>>({});
const webhooksLoading = ref(false);

const newWebhookEventType = ref("delivered");
const newWebhookUrl = ref("");
const creating = ref(false);

const webhookEventTypes = [
  "accepted",
  "delivered",
  "failed",
  "opened",
  "clicked",
  "unsubscribed",
  "complained",
  "stored",
];

const webhookColumns: Column[] = [
  { key: "event_type", label: "Event Type" },
  { key: "urls", label: "URLs" },
  { key: "actions", label: "" },
];

const webhookRows = computed(() =>
  Object.entries(webhooks.value).map(([eventType, entry]) => ({
    event_type: eventType,
    urls: (entry.urls || []).join(", "),
    _event_type: eventType,
  }))
);

// --- Delivery log state ---

const deliveries = ref<DeliveryItem[]>([]);
const deliveryPaging = ref<DeliveryPaging>({});
const deliveryTotalCount = ref(0);
const deliveriesLoading = ref(false);
const deliveryError = ref<string | null>(null);
const expandedDeliveryId = ref<string | null>(null);

const deliveryColumns: Column[] = [
  { key: "timestamp", label: "Timestamp" },
  { key: "url", label: "URL" },
  { key: "event_type", label: "Event Type" },
  { key: "domain", label: "Domain" },
  { key: "status", label: "Status" },
  { key: "response_time", label: "Response Time" },
  { key: "attempt", label: "Attempt" },
];

function formatTimestamp(ts: number): string {
  if (!ts) return "-";
  const date = new Date(ts * 1000);
  return date.toLocaleString();
}

const deliveryRows = computed(() =>
  deliveries.value.map((d) => ({
    id: d.id,
    timestamp: formatTimestamp(d.timestamp),
    url: d.url,
    event_type: d.event_type,
    domain: d.domain,
    status: d.success ? "delivered" : "failed",
    response_time: d.response_time_ms + "ms",
    attempt: String(d.attempt),
    _raw: d,
  }))
);

const hasDeliveryNext = computed(() => !!deliveryPaging.value.next);
const hasDeliveryPrevious = computed(() => !!deliveryPaging.value.previous);

// --- API functions ---

async function fetchDomains() {
  domainsLoading.value = true;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
      await fetchWebhooks();
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

async function fetchWebhooks() {
  if (!selectedDomain.value) return;
  webhooksLoading.value = true;
  error.value = null;
  try {
    const resp = await api.get<WebhooksResponse>(
      `/v3/domains/${encodeURIComponent(selectedDomain.value)}/webhooks`
    );
    webhooks.value = resp.webhooks || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load webhooks";
  } finally {
    webhooksLoading.value = false;
  }
}

async function createWebhook() {
  if (!selectedDomain.value || !newWebhookUrl.value.trim()) return;
  creating.value = true;
  error.value = null;
  try {
    await api.post<WebhookMutationResponse>(
      `/v3/domains/${encodeURIComponent(selectedDomain.value)}/webhooks`,
      {
        id: newWebhookEventType.value,
        url: [newWebhookUrl.value.trim()],
      }
    );
    newWebhookUrl.value = "";
    await fetchWebhooks();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to create webhook";
  } finally {
    creating.value = false;
  }
}

async function deleteWebhook(eventType: string) {
  if (!selectedDomain.value) return;
  error.value = null;
  try {
    await api.del<WebhookMutationResponse>(
      `/v3/domains/${encodeURIComponent(selectedDomain.value)}/webhooks/${encodeURIComponent(eventType)}`
    );
    await fetchWebhooks();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete webhook";
  }
}

function buildDeliveriesUrl(pagingUrl?: string): string {
  if (pagingUrl) {
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  return "/mock/webhooks/deliveries";
}

async function fetchDeliveries(pagingUrl?: string) {
  deliveriesLoading.value = true;
  deliveryError.value = null;
  expandedDeliveryId.value = null;
  try {
    const url = buildDeliveriesUrl(pagingUrl);
    const resp = await api.get<DeliveriesResponse>(url);
    deliveries.value = resp.items || [];
    deliveryPaging.value = resp.paging || {};
    deliveryTotalCount.value = resp.total_count || 0;
  } catch (e: unknown) {
    const err = e as { message?: string };
    deliveryError.value = err.message || "Failed to load delivery log";
  } finally {
    deliveriesLoading.value = false;
  }
}

function onDomainChange() {
  fetchWebhooks();
}

function loadDeliveryNext() {
  if (deliveryPaging.value.next) fetchDeliveries(deliveryPaging.value.next);
}

function loadDeliveryPrev() {
  if (deliveryPaging.value.previous) fetchDeliveries(deliveryPaging.value.previous);
}

function toggleDeliveryExpand(row: Record<string, unknown>) {
  const d = row._raw as DeliveryItem;
  if (expandedDeliveryId.value === d.id) {
    expandedDeliveryId.value = null;
  } else {
    expandedDeliveryId.value = d.id;
  }
}

function getExpandedDelivery(): DeliveryItem | null {
  if (!expandedDeliveryId.value) return null;
  return deliveries.value.find((d) => d.id === expandedDeliveryId.value) || null;
}

onMounted(() => {
  fetchDomains();
  fetchDeliveries();
});
</script>

<template>
  <div class="page">
    <h1>Webhooks</h1>

    <!-- Webhook Configuration Section -->
    <div class="card-section">
      <div class="section-header">
        <h2>Webhook Configuration</h2>
      </div>

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
        No domains configured. Create a domain first to manage webhooks.
      </div>

      <template v-if="selectedDomain">
        <!-- Add Webhook Form -->
        <div class="add-webhook-form">
          <select
            v-model="newWebhookEventType"
            class="select-input"
          >
            <option
              v-for="et in webhookEventTypes"
              :key="et"
              :value="et"
            >
              {{ et }}
            </option>
          </select>
          <input
            v-model="newWebhookUrl"
            type="text"
            placeholder="https://example.com/webhook"
            class="filter-input url-input"
            @keyup.enter="createWebhook"
          >
          <button
            class="btn btn-primary"
            :disabled="!newWebhookUrl.trim() || creating"
            @click="createWebhook"
          >
            {{ creating ? "Creating..." : "Create" }}
          </button>
        </div>

        <!-- Webhook Table -->
        <DataTable
          :columns="webhookColumns"
          :rows="webhookRows"
          :loading="webhooksLoading"
        >
          <template #cell-event_type="{ value }">
            <StatusBadge :status="String(value)" />
          </template>
          <template #cell-urls="{ value }">
            <span class="mono">{{ value }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button
              class="btn btn-danger btn-sm"
              @click="deleteWebhook(String(row._event_type))"
            >
              Delete
            </button>
          </template>
        </DataTable>
      </template>
    </div>

    <!-- Delivery Log Section -->
    <div class="card-section">
      <div class="section-header">
        <h2>Delivery Log</h2>
        <span class="total-count">{{ deliveryTotalCount }} total</span>
      </div>

      <div
        v-if="deliveryError"
        class="error"
      >
        {{ deliveryError }}
      </div>

      <DataTable
        :columns="deliveryColumns"
        :rows="deliveryRows"
        :loading="deliveriesLoading"
      >
        <template #cell-timestamp="{ value, row }">
          <a
            href="#"
            class="delivery-link"
            @click.prevent="toggleDeliveryExpand(row)"
          >
            {{ value }}
          </a>
        </template>
        <template #cell-event_type="{ value }">
          <StatusBadge :status="String(value)" />
        </template>
        <template #cell-status="{ value }">
          <StatusBadge :status="String(value)" />
        </template>
      </DataTable>

      <!-- Expanded Delivery Detail -->
      <div
        v-if="getExpandedDelivery()"
        class="delivery-detail"
      >
        <div class="detail-header">
          <h3>Delivery Detail</h3>
          <button
            class="btn btn-sm"
            @click="expandedDeliveryId = null"
          >
            Close
          </button>
        </div>

        <div class="detail-grid">
          <div class="detail-field">
            <label>ID</label>
            <span class="mono">{{ getExpandedDelivery()!.id }}</span>
          </div>
          <div class="detail-field">
            <label>Webhook ID</label>
            <span class="mono">{{ getExpandedDelivery()!.webhook_id }}</span>
          </div>
          <div class="detail-field">
            <label>URL</label>
            <span class="mono">{{ getExpandedDelivery()!.url }}</span>
          </div>
          <div class="detail-field">
            <label>Event Type</label>
            <span>{{ getExpandedDelivery()!.event_type }}</span>
          </div>
          <div class="detail-field">
            <label>Domain</label>
            <span>{{ getExpandedDelivery()!.domain }}</span>
          </div>
          <div class="detail-field">
            <label>Message ID</label>
            <span class="mono">{{ getExpandedDelivery()!.message_id }}</span>
          </div>
          <div class="detail-field">
            <label>Timestamp</label>
            <span>{{ formatTimestamp(getExpandedDelivery()!.timestamp) }}</span>
          </div>
          <div class="detail-field">
            <label>Response Time</label>
            <span>{{ getExpandedDelivery()!.response_time_ms }}ms</span>
          </div>
          <div class="detail-field">
            <label>Attempt</label>
            <span>{{ getExpandedDelivery()!.attempt }}</span>
          </div>
          <div class="detail-field">
            <label>Success</label>
            <StatusBadge :status="getExpandedDelivery()!.success ? 'delivered' : 'failed'" />
          </div>
        </div>

        <!-- Request -->
        <div class="detail-section">
          <h3>Request</h3>
          <pre class="detail-json">{{ JSON.stringify(getExpandedDelivery()!.request, null, 2) }}</pre>
        </div>

        <!-- Response -->
        <div class="detail-section">
          <h3>Response</h3>
          <pre class="detail-json">{{ JSON.stringify(getExpandedDelivery()!.response, null, 2) }}</pre>
        </div>
      </div>

      <Pagination
        :has-next="hasDeliveryNext"
        :has-previous="hasDeliveryPrevious"
        @next="loadDeliveryNext"
        @previous="loadDeliveryPrev"
      />
    </div>
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

.url-input {
  flex: 1;
  min-width: 16rem;
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

.btn-danger {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-color: var(--color-badge-danger-text, #991b1b);
}

.btn-danger:hover {
  opacity: 0.9;
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

.card-section {
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.section-header h2 {
  margin: 0;
  font-size: 1.125rem;
}

.total-count {
  color: var(--color-text-secondary, #64748b);
  font-size: 0.875rem;
}

.add-webhook-form {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
  margin-bottom: 1rem;
}

.mono {
  font-family: monospace;
  font-size: 0.8125rem;
  word-break: break-all;
}

.delivery-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.delivery-link:hover {
  text-decoration: underline;
}

.delivery-detail {
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

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.detail-field {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.detail-field label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.detail-field span {
  font-size: 0.875rem;
  color: var(--color-text-primary, #1e293b);
  word-break: break-all;
}

.detail-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--color-border, #e2e8f0);
}

.detail-section h3 {
  font-size: 0.875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  margin: 0 0 0.5rem;
}

.detail-json {
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
