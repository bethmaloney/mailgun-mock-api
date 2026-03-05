<script setup lang="ts">
import { ref, onMounted, computed, watch } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
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

interface MessageListItem {
  id: string;
  storage_key: string;
  domain: string;
  from: string;
  to: string[];
  subject: string;
  tags: string[];
  timestamp: number;
  status: string;
  has_attachments: boolean;
}

interface Paging {
  next: string;
  previous: string;
}

interface MessagesListResponse {
  items: MessageListItem[];
  paging: Paging;
  total_count: number;
}

interface TriggerRequestBody {
  severity?: string;
  reason?: string;
  url?: string;
}

interface TriggerResponse {
  message: string;
  [key: string]: unknown;
}

// --- Event type definitions ---

interface EventTypeConfig {
  key: string;
  label: string;
  btnClass: string;
}

const eventTypes: EventTypeConfig[] = [
  { key: "deliver", label: "Deliver", btnClass: "btn-event-deliver" },
  { key: "fail", label: "Fail", btnClass: "btn-event-fail" },
  { key: "open", label: "Open", btnClass: "btn-event-open" },
  { key: "click", label: "Click", btnClass: "btn-event-click" },
  { key: "unsubscribe", label: "Unsubscribe", btnClass: "btn-event-unsubscribe" },
  { key: "complain", label: "Complain", btnClass: "btn-event-complain" },
];

// --- Domain selector state ---

const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const domainsLoading = ref(true);
const error = ref<string | null>(null);

// --- Messages state ---

const messages = ref<MessageListItem[]>([]);
const messagesLoading = ref(false);
const messagesTotalCount = ref(0);
const messageSearch = ref("");

// --- Selected message & trigger state ---

const selectedMessageId = ref<string | null>(null);
const selectedEventType = ref<string | null>(null);
const triggering = ref(false);
const triggerResult = ref<{ success: boolean; message: string } | null>(null);

// --- Conditional fields ---

const failSeverity = ref("permanent");
const failErrorMessage = ref("");
const clickUrl = ref("");

// --- Computed ---

const selectedMessage = computed(() => {
  if (!selectedMessageId.value) return null;
  return messages.value.find((m) => m.id === selectedMessageId.value) || null;
});

const filteredMessages = computed(() => {
  if (!messageSearch.value.trim()) return messages.value;
  const q = messageSearch.value.toLowerCase();
  return messages.value.filter(
    (m) =>
      m.from.toLowerCase().includes(q) ||
      m.to.some((t) => t.toLowerCase().includes(q)) ||
      m.subject.toLowerCase().includes(q) ||
      m.id.toLowerCase().includes(q)
  );
});

const messageColumns: Column[] = [
  { key: "from", label: "From" },
  { key: "to", label: "To" },
  { key: "subject", label: "Subject" },
  { key: "date", label: "Date" },
  { key: "select", label: "" },
];

function formatTimestamp(ts: number): string {
  if (!ts) return "-";
  const date = new Date(ts * 1000);
  return date.toLocaleString();
}

const messageRows = computed(() =>
  filteredMessages.value.map((msg) => ({
    id: msg.id,
    from: msg.from,
    to: msg.to.join(", "),
    subject: msg.subject || "(no subject)",
    date: formatTimestamp(msg.timestamp),
    _raw: msg,
  }))
);

// --- API functions ---

async function fetchDomains() {
  domainsLoading.value = true;
  error.value = null;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
      await fetchMessages();
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

async function fetchMessages() {
  if (!selectedDomain.value) return;
  messagesLoading.value = true;
  error.value = null;
  selectedMessageId.value = null;
  selectedEventType.value = null;
  triggerResult.value = null;
  try {
    const resp = await api.get<MessagesListResponse>(
      `/mock/messages?domain=${encodeURIComponent(selectedDomain.value)}`
    );
    messages.value = resp.items || [];
    messagesTotalCount.value = resp.total_count || 0;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load messages";
  } finally {
    messagesLoading.value = false;
  }
}

function onDomainChange() {
  fetchMessages();
}

function selectMessage(row: Record<string, unknown>) {
  const msg = row._raw as MessageListItem;
  if (selectedMessageId.value === msg.id) {
    selectedMessageId.value = null;
    selectedEventType.value = null;
    triggerResult.value = null;
  } else {
    selectedMessageId.value = msg.id;
    selectedEventType.value = null;
    triggerResult.value = null;
    resetFields();
  }
}

function selectEventType(eventKey: string) {
  selectedEventType.value = eventKey;
  triggerResult.value = null;
  resetFields();
}

function resetFields() {
  failSeverity.value = "permanent";
  failErrorMessage.value = "";
  clickUrl.value = "";
}

function buildRequestBody(): TriggerRequestBody {
  const body: TriggerRequestBody = {};

  if (selectedEventType.value === "fail") {
    body.severity = failSeverity.value;
    if (failErrorMessage.value.trim()) {
      body.reason = failErrorMessage.value.trim();
    }
  }

  if (selectedEventType.value === "click" && clickUrl.value.trim()) {
    body.url = clickUrl.value.trim();
  }

  return body;
}

async function triggerEvent() {
  if (!selectedDomain.value || !selectedMessageId.value || !selectedEventType.value) return;
  const msg = selectedMessage.value;
  if (!msg) return;
  triggering.value = true;
  triggerResult.value = null;
  try {
    const body = buildRequestBody();
    const resp = await api.post<TriggerResponse>(
      `/mock/events/${selectedDomain.value}/${selectedEventType.value}/${msg.storage_key}`,
      body
    );
    triggerResult.value = {
      success: true,
      message: resp.message || "Event triggered successfully",
    };
  } catch (e: unknown) {
    const err = e as { message?: string };
    triggerResult.value = {
      success: false,
      message: err.message || "Failed to trigger event",
    };
  } finally {
    triggering.value = false;
  }
}

// --- Watchers ---

watch(selectedDomain, () => {
  messageSearch.value = "";
});

// --- Lifecycle ---

onMounted(() => {
  fetchDomains();
});
</script>

<template>
  <div class="page">
    <h1>Trigger Events</h1>

    <!-- Step 1: Domain Selector -->
    <div class="card-section">
      <div class="section-header">
        <h2>1. Select Domain</h2>
      </div>

      <div class="controls">
        <div class="domain-selector">
          <label for="trigger-domain-select">Domain:</label>
          <select
            id="trigger-domain-select"
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
        No domains configured. Create a domain and send messages first.
      </div>
    </div>

    <!-- Step 2: Select Message -->
    <div
      v-if="selectedDomain"
      class="card-section"
    >
      <div class="section-header">
        <h2>2. Select Message</h2>
        <span class="total-count">{{ messagesTotalCount }} messages</span>
      </div>

      <!-- Search filter -->
      <div class="controls">
        <input
          v-model="messageSearch"
          type="text"
          placeholder="Search by from, to, subject, or ID..."
          class="filter-input search-input"
        >
      </div>

      <DataTable
        :columns="messageColumns"
        :rows="messageRows"
        :loading="messagesLoading"
      >
        <template #cell-from="{ value }">
          <span class="mono">{{ value }}</span>
        </template>
        <template #cell-to="{ value }">
          <span class="mono">{{ value }}</span>
        </template>
        <template #cell-subject="{ row }">
          <a
            href="#"
            class="msg-link"
            @click.prevent="selectMessage(row)"
          >
            {{ row.subject }}
          </a>
        </template>
        <template #cell-select="{ row }">
          <button
            class="btn btn-sm"
            :class="{ 'btn-primary': selectedMessageId === (row._raw as MessageListItem).id }"
            @click="selectMessage(row)"
          >
            {{ selectedMessageId === (row._raw as MessageListItem).id ? "Selected" : "Select" }}
          </button>
        </template>
      </DataTable>

      <div
        v-if="!messagesLoading && messages.length === 0"
        class="info"
      >
        No messages found for this domain. Send some messages first.
      </div>
    </div>

    <!-- Step 3: Trigger Event -->
    <div
      v-if="selectedMessage"
      class="card-section"
    >
      <div class="section-header">
        <h2>3. Trigger Event</h2>
      </div>

      <!-- Selected message summary -->
      <div class="selected-message-summary">
        <div class="detail-grid">
          <div class="detail-field">
            <label>From</label>
            <span class="mono">{{ selectedMessage.from }}</span>
          </div>
          <div class="detail-field">
            <label>To</label>
            <span class="mono">{{ selectedMessage.to.join(", ") }}</span>
          </div>
          <div class="detail-field">
            <label>Subject</label>
            <span>{{ selectedMessage.subject || "(no subject)" }}</span>
          </div>
          <div class="detail-field">
            <label>Storage Key</label>
            <span class="mono">{{ selectedMessage.storage_key }}</span>
          </div>
        </div>
      </div>

      <!-- Event type buttons -->
      <div class="event-type-buttons">
        <button
          v-for="et in eventTypes"
          :key="et.key"
          class="btn"
          :class="[et.btnClass, { 'btn-event-active': selectedEventType === et.key }]"
          @click="selectEventType(et.key)"
        >
          {{ et.label }}
        </button>
      </div>

      <!-- Conditional fields for Fail -->
      <div
        v-if="selectedEventType === 'fail'"
        class="event-fields"
      >
        <h3>Failure Options</h3>
        <div class="field-row">
          <div class="field-group">
            <label for="fail-severity">Severity</label>
            <select
              id="fail-severity"
              v-model="failSeverity"
              class="select-input"
            >
              <option value="permanent">
                Permanent
              </option>
              <option value="temporary">
                Temporary
              </option>
            </select>
          </div>
          <div class="field-group field-grow">
            <label for="fail-error">Error Message</label>
            <input
              id="fail-error"
              v-model="failErrorMessage"
              type="text"
              placeholder="e.g. 550 User not found"
              class="filter-input"
            >
          </div>
        </div>
      </div>

      <!-- Conditional fields for Click -->
      <div
        v-if="selectedEventType === 'click'"
        class="event-fields"
      >
        <h3>Click Options</h3>
        <div class="field-row">
          <div class="field-group field-grow">
            <label for="click-url">Clicked URL</label>
            <input
              id="click-url"
              v-model="clickUrl"
              type="text"
              placeholder="https://example.com/link"
              class="filter-input"
            >
          </div>
        </div>
      </div>

      <!-- Trigger button -->
      <div
        v-if="selectedEventType"
        class="trigger-action"
      >
        <button
          class="btn btn-primary btn-trigger"
          :disabled="triggering"
          @click="triggerEvent"
        >
          {{ triggering ? "Triggering..." : `Trigger ${eventTypes.find(e => e.key === selectedEventType)?.label} Event` }}
        </button>
      </div>

      <!-- Result panel -->
      <div
        v-if="triggerResult"
        class="trigger-result"
        :class="triggerResult.success ? 'result-success' : 'result-error'"
      >
        <div class="result-header">
          <StatusBadge :status="triggerResult.success ? 'delivered' : 'failed'" />
          <span class="result-label">{{ triggerResult.success ? "Success" : "Error" }}</span>
        </div>
        <p class="result-message">
          {{ triggerResult.message }}
        </p>
      </div>
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

.search-input {
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

.mono {
  font-family: monospace;
  font-size: 0.8125rem;
  word-break: break-all;
}

.msg-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.msg-link:hover {
  text-decoration: underline;
}

/* Selected message summary */
.selected-message-summary {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 1rem;
  margin-bottom: 1rem;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: 0.75rem;
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

/* Event type buttons */
.event-type-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.btn-event-deliver {
  background: var(--color-badge-success-bg, #dcfce7);
  color: var(--color-badge-success-text, #166534);
  border-color: var(--color-badge-success-text, #166534);
}

.btn-event-fail {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-color: var(--color-badge-danger-text, #991b1b);
}

.btn-event-open {
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border-color: var(--color-badge-info-text, #1e40af);
}

.btn-event-click {
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border-color: var(--color-badge-info-text, #1e40af);
}

.btn-event-unsubscribe {
  background: var(--color-badge-warning-bg, #fef3c7);
  color: var(--color-badge-warning-text, #92400e);
  border-color: var(--color-badge-warning-text, #92400e);
}

.btn-event-complain {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-color: var(--color-badge-danger-text, #991b1b);
}

.btn-event-active {
  outline: 3px solid var(--color-primary, #3b82f6);
  outline-offset: 1px;
  font-weight: 700;
}

/* Event fields */
.event-fields {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 1rem;
  margin-bottom: 1rem;
}

.event-fields h3 {
  font-size: 0.875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  margin: 0 0 0.75rem;
}

.field-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: flex-end;
}

.field-group {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.field-group label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
}

.field-grow {
  flex: 1;
  min-width: 12rem;
}

/* Trigger action */
.trigger-action {
  margin-bottom: 1rem;
}

.btn-trigger {
  padding: 0.5rem 1.5rem;
  font-size: 1rem;
  font-weight: 600;
}

/* Result panel */
.trigger-result {
  border-radius: 0.375rem;
  padding: 1rem;
}

.result-success {
  background: var(--color-badge-success-bg, #dcfce7);
  border: 1px solid var(--color-badge-success-text, #166534);
}

.result-error {
  background: var(--color-badge-danger-bg, #fee2e2);
  border: 1px solid var(--color-badge-danger-text, #991b1b);
}

.result-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.result-label {
  font-weight: 600;
  font-size: 0.875rem;
}

.result-message {
  margin: 0;
  font-size: 0.875rem;
  word-break: break-word;
}
</style>
