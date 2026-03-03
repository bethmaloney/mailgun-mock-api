<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import type { Column } from "@/components/DataTable.vue";
import { useWebSocket } from "@/composables/useWebSocket";

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

interface AttachmentDetail {
  filename: string;
  size: number;
  content_type: string;
}

interface EventDetail {
  id: string;
  event_type: string;
  timestamp: number;
}

interface MessageDetail {
  id: string;
  message_id: string;
  storage_key: string;
  domain: string;
  from: string;
  to: string[];
  subject: string;
  text_body: string;
  html_body: string;
  tags: string[];
  custom_headers: Record<string, unknown>;
  custom_variables: Record<string, unknown>;
  options: Record<string, unknown>;
  timestamp: number;
  attachments: AttachmentDetail[];
  events: EventDetail[];
}

const messages = ref<MessageListItem[]>([]);
const paging = ref<Paging>({ next: "", previous: "" });
const totalCount = ref(0);
const loading = ref(true);
const error = ref<string | null>(null);

// Filters
const filterDomain = ref("");
const filterFrom = ref("");
const filterTo = ref("");
const filterSubject = ref("");

// Detail view
const selectedMessage = ref<MessageDetail | null>(null);
const detailLoading = ref(false);

const columns: Column[] = [
  { key: "from", label: "From" },
  { key: "to", label: "To" },
  { key: "subject", label: "Subject" },
  { key: "domain", label: "Domain" },
  { key: "tags", label: "Tags" },
  { key: "status", label: "Status" },
  { key: "date", label: "Date" },
];

// Pagination state
const currentPage = ref("");

function formatTimestamp(ts: number): string {
  if (!ts) return "-";
  const date = new Date(ts * 1000);
  return date.toLocaleString();
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

const tableRows = computed(() =>
  messages.value.map((msg) => ({
    ...msg,
    to: msg.to.join(", "),
    tags: msg.tags.length > 0 ? msg.tags.join(", ") : "-",
    date: formatTimestamp(msg.timestamp),
    _storage_key: msg.storage_key,
    _raw: msg,
  }))
);

const hasNext = computed(() => !!paging.value.next);
const hasPrevious = computed(() => !!paging.value.previous);

function buildQueryString(page?: string): string {
  const params = new URLSearchParams();
  if (filterDomain.value) params.set("domain", filterDomain.value);
  if (filterFrom.value) params.set("from", filterFrom.value);
  if (filterTo.value) params.set("to", filterTo.value);
  if (filterSubject.value) params.set("subject", filterSubject.value);
  if (page) params.set("page", page);
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

async function fetchMessages(page?: string) {
  loading.value = true;
  error.value = null;
  selectedMessage.value = null;
  try {
    const qs = buildQueryString(page);
    const resp = await api.get<MessagesListResponse>(`/mock/messages${qs}`);
    messages.value = resp.items || [];
    paging.value = resp.paging || { next: "", previous: "" };
    totalCount.value = resp.total_count;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load messages";
  } finally {
    loading.value = false;
  }
}

function extractPageToken(url: string): string {
  try {
    const u = new URL(url, window.location.origin);
    return u.searchParams.get("page") || "";
  } catch {
    return "";
  }
}

function loadNext() {
  const token = extractPageToken(paging.value.next);
  currentPage.value = token;
  fetchMessages(token);
}

function loadPrev() {
  const token = extractPageToken(paging.value.previous);
  currentPage.value = token;
  fetchMessages(token);
}

function applyFilters() {
  currentPage.value = "";
  fetchMessages();
}

async function viewDetail(row: Record<string, unknown>) {
  const storageKey = row._storage_key as string;
  if (
    selectedMessage.value &&
    selectedMessage.value.storage_key === storageKey
  ) {
    selectedMessage.value = null;
    return;
  }
  detailLoading.value = true;
  try {
    selectedMessage.value = await api.get<MessageDetail>(
      `/mock/messages/${encodeURIComponent(storageKey)}`
    );
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load message detail";
  } finally {
    detailLoading.value = false;
  }
}

async function deleteMessage(storageKey: string) {
  if (!window.confirm("Delete this message?")) return;
  try {
    await api.del(`/mock/messages/${encodeURIComponent(storageKey)}`);
    if (
      selectedMessage.value &&
      selectedMessage.value.storage_key === storageKey
    ) {
      selectedMessage.value = null;
    }
    await fetchMessages(currentPage.value || undefined);
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete message";
  }
}

async function clearAll() {
  if (!window.confirm("Delete ALL messages, events, and attachments?")) return;
  try {
    await api.post("/mock/messages/clear");
    selectedMessage.value = null;
    currentPage.value = "";
    await fetchMessages();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to clear messages";
  }
}

const { onMessage } = useWebSocket();

onMessage((msg) => {
  if (msg.type === "message.new") {
    fetchMessages();
  }
});

onMounted(() => fetchMessages());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Messages</h1>
      <div class="header-actions">
        <span class="total-count">{{ totalCount }} total</span>
        <button
          class="btn btn-danger"
          @click="clearAll"
        >
          Clear All
        </button>
      </div>
    </div>

    <!-- Filters -->
    <div class="filters">
      <input
        v-model="filterDomain"
        type="text"
        placeholder="Domain"
        class="filter-input"
        @keyup.enter="applyFilters"
      >
      <input
        v-model="filterFrom"
        type="text"
        placeholder="From"
        class="filter-input"
        @keyup.enter="applyFilters"
      >
      <input
        v-model="filterTo"
        type="text"
        placeholder="To"
        class="filter-input"
        @keyup.enter="applyFilters"
      >
      <input
        v-model="filterSubject"
        type="text"
        placeholder="Subject"
        class="filter-input"
        @keyup.enter="applyFilters"
      >
      <button
        class="btn btn-primary"
        @click="applyFilters"
      >
        Filter
      </button>
    </div>

    <div
      v-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <DataTable
      :columns="columns"
      :rows="tableRows"
      :loading="loading"
    >
      <template #cell-status="{ value }">
        <StatusBadge :status="String(value)" />
      </template>
      <template #cell-subject="{ row }">
        <a
          href="#"
          class="msg-link"
          @click.prevent="viewDetail(row)"
        >
          {{ row.subject }}
        </a>
      </template>
      <template #cell-date="{ row }">
        <span class="date-cell">
          {{ row.date }}
          <button
            class="btn-icon btn-delete"
            title="Delete message"
            @click.stop="deleteMessage(String(row._storage_key))"
          >
            x
          </button>
        </span>
      </template>
    </DataTable>

    <Pagination
      :has-next="hasNext"
      :has-previous="hasPrevious"
      @next="loadNext"
      @previous="loadPrev"
    />

    <!-- Detail Panel -->
    <div
      v-if="detailLoading"
      class="detail-panel loading"
    >
      Loading message detail...
    </div>
    <div
      v-else-if="selectedMessage"
      class="detail-panel"
    >
      <div class="detail-header">
        <h2>Message Detail</h2>
        <button
          class="btn btn-sm"
          @click="selectedMessage = null"
        >
          Close
        </button>
      </div>

      <div class="detail-grid">
        <div class="detail-field">
          <label>From</label>
          <span>{{ selectedMessage.from }}</span>
        </div>
        <div class="detail-field">
          <label>To</label>
          <span>{{ selectedMessage.to.join(", ") }}</span>
        </div>
        <div class="detail-field">
          <label>Subject</label>
          <span>{{ selectedMessage.subject }}</span>
        </div>
        <div class="detail-field">
          <label>Domain</label>
          <span>{{ selectedMessage.domain }}</span>
        </div>
        <div class="detail-field">
          <label>Message ID</label>
          <span class="mono">{{ selectedMessage.message_id }}</span>
        </div>
        <div class="detail-field">
          <label>Storage Key</label>
          <span class="mono">{{ selectedMessage.storage_key }}</span>
        </div>
      </div>

      <!-- Tags -->
      <div
        v-if="selectedMessage.tags.length > 0"
        class="detail-section"
      >
        <h3>Tags</h3>
        <div class="tag-list">
          <span
            v-for="tag in selectedMessage.tags"
            :key="tag"
            class="tag"
          >
            {{ tag }}
          </span>
        </div>
      </div>

      <!-- Text Body -->
      <div
        v-if="selectedMessage.text_body"
        class="detail-section"
      >
        <h3>Text Body</h3>
        <pre class="body-content">{{ selectedMessage.text_body }}</pre>
      </div>

      <!-- HTML Body -->
      <div
        v-if="selectedMessage.html_body"
        class="detail-section"
      >
        <h3>HTML Body</h3>
        <iframe
          class="html-preview"
          sandbox=""
          :srcdoc="selectedMessage.html_body"
        />
      </div>

      <!-- Custom Headers -->
      <div
        v-if="Object.keys(selectedMessage.custom_headers).length > 0"
        class="detail-section"
      >
        <h3>Custom Headers</h3>
        <pre class="body-content">{{ JSON.stringify(selectedMessage.custom_headers, null, 2) }}</pre>
      </div>

      <!-- Custom Variables -->
      <div
        v-if="Object.keys(selectedMessage.custom_variables).length > 0"
        class="detail-section"
      >
        <h3>Custom Variables</h3>
        <pre class="body-content">{{ JSON.stringify(selectedMessage.custom_variables, null, 2) }}</pre>
      </div>

      <!-- Options -->
      <div
        v-if="Object.keys(selectedMessage.options).length > 0"
        class="detail-section"
      >
        <h3>Options</h3>
        <pre class="body-content">{{ JSON.stringify(selectedMessage.options, null, 2) }}</pre>
      </div>

      <!-- Attachments -->
      <div
        v-if="selectedMessage.attachments.length > 0"
        class="detail-section"
      >
        <h3>Attachments</h3>
        <ul class="attachment-list">
          <li
            v-for="att in selectedMessage.attachments"
            :key="att.filename"
          >
            <span class="att-name">{{ att.filename }}</span>
            <span class="att-meta">{{ att.content_type }} - {{ formatSize(att.size) }}</span>
          </li>
        </ul>
      </div>

      <!-- Events Timeline -->
      <div
        v-if="selectedMessage.events.length > 0"
        class="detail-section"
      >
        <h3>Events Timeline</h3>
        <div class="timeline">
          <div
            v-for="ev in selectedMessage.events"
            :key="ev.id"
            class="timeline-item"
          >
            <StatusBadge :status="ev.event_type" />
            <span class="timeline-time">{{ formatTimestamp(ev.timestamp) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.page-header h1 {
  margin: 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.total-count {
  color: var(--color-text-secondary, #64748b);
  font-size: 0.875rem;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1rem;
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

.btn-primary:hover {
  opacity: 0.9;
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

.btn-icon {
  border: none;
  background: none;
  cursor: pointer;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.75rem;
  line-height: 1;
}

.btn-delete {
  color: var(--color-badge-danger-text, #991b1b);
}

.btn-delete:hover {
  background: var(--color-badge-danger-bg, #fee2e2);
}

.error {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-radius: 0.375rem;
}

.msg-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.msg-link:hover {
  text-decoration: underline;
}

.date-cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.detail-panel {
  margin-top: 1rem;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
}

.detail-panel.loading {
  text-align: center;
  color: var(--color-text-secondary, #64748b);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.detail-header h2 {
  margin: 0;
  font-size: 1.125rem;
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

.mono {
  font-family: monospace;
  font-size: 0.8125rem !important;
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

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.tag {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
}

.body-content {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 0.75rem;
  font-size: 0.8125rem;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}

.html-preview {
  width: 100%;
  min-height: 12rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: #ffffff;
}

.attachment-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.attachment-list li {
  display: flex;
  justify-content: space-between;
  padding: 0.375rem 0;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  font-size: 0.875rem;
}

.att-name {
  font-weight: 500;
}

.att-meta {
  color: var(--color-text-secondary, #64748b);
  font-size: 0.8125rem;
}

.timeline {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.timeline-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.timeline-time {
  font-size: 0.8125rem;
  color: var(--color-text-secondary, #64748b);
}
</style>
