<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";
import DataTable from "@/components/DataTable.vue";
import type { Column } from "@/components/DataTable.vue";

interface DomainListItem {
  name: string;
  state: string;
  type: string;
  spam_action: string;
  wildcard: boolean;
  web_scheme: string;
  web_prefix: string;
  created_at: string;
  smtp_login: string;
}

interface DomainsListResponse {
  items: DomainListItem[];
  total_count: number;
}

interface DnsRecord {
  record_type: string;
  name: string;
  value: string;
  valid: string;
  cached: unknown[];
}

interface DomainDetailDomain {
  name: string;
  state: string;
  type: string;
  spam_action: string;
  wildcard: boolean;
  web_scheme: string;
  web_prefix: string;
  created_at: string;
  smtp_login: string;
}

interface DomainDetailResponse {
  domain: DomainDetailDomain;
  sending_dns_records: DnsRecord[];
  receiving_dns_records: DnsRecord[];
}

interface TrackingSetting {
  active: string;
  html_footer?: string;
  text_footer?: string;
}

interface TrackingResponse {
  tracking: {
    open: TrackingSetting;
    click: TrackingSetting;
    unsubscribe: TrackingSetting;
  };
}

interface ConnectionResponse {
  connection: {
    require_tls: boolean;
    skip_verification: boolean;
  };
}

interface CreateDomainResponse {
  domain: DomainDetailDomain;
  sending_dns_records: DnsRecord[];
  receiving_dns_records: DnsRecord[];
  message: string;
}

interface DeleteDomainResponse {
  message: string;
}

// Domain list state
const domains = ref<DomainListItem[]>([]);
const totalCount = ref(0);
const loading = ref(true);
const error = ref<string | null>(null);

// Add domain form
const newDomainName = ref("");
const creating = ref(false);

// Detail view state
const selectedDomain = ref<DomainDetailResponse | null>(null);
const detailLoading = ref(false);
const trackingSettings = ref<TrackingResponse["tracking"] | null>(null);
const connectionSettings = ref<ConnectionResponse["connection"] | null>(null);

const columns: Column[] = [
  { key: "name", label: "Name" },
  { key: "state", label: "State" },
  { key: "type", label: "Type" },
  { key: "spam_action", label: "Spam Action" },
  { key: "created_at", label: "Created At" },
  { key: "actions", label: "" },
];

function formatDate(dateStr: string): string {
  if (!dateStr) return "-";
  try {
    const date = new Date(dateStr);
    return date.toLocaleString();
  } catch {
    return dateStr;
  }
}

const tableRows = computed(() =>
  domains.value.map((d) => ({
    ...d,
    created_at: formatDate(d.created_at),
    _name: d.name,
    _raw: d,
  }))
);

async function fetchDomains() {
  loading.value = true;
  error.value = null;
  try {
    const resp = await api.get<DomainsListResponse>("/v4/domains");
    domains.value = resp.items || [];
    totalCount.value = resp.total_count;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    loading.value = false;
  }
}

async function createDomain() {
  const name = newDomainName.value.trim();
  if (!name) return;
  creating.value = true;
  error.value = null;
  try {
    await api.post<CreateDomainResponse>("/v4/domains", { name });
    newDomainName.value = "";
    await fetchDomains();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to create domain";
  } finally {
    creating.value = false;
  }
}

async function deleteDomain(name: string) {
  if (!window.confirm(`Delete domain "${name}"?`)) return;
  try {
    await api.del<DeleteDomainResponse>(`/v3/domains/${encodeURIComponent(name)}`);
    if (selectedDomain.value && selectedDomain.value.domain.name === name) {
      selectedDomain.value = null;
      trackingSettings.value = null;
      connectionSettings.value = null;
    }
    await fetchDomains();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete domain";
  }
}

async function viewDetail(row: Record<string, unknown>) {
  const name = row._name as string;
  if (selectedDomain.value && selectedDomain.value.domain.name === name) {
    selectedDomain.value = null;
    trackingSettings.value = null;
    connectionSettings.value = null;
    return;
  }
  detailLoading.value = true;
  trackingSettings.value = null;
  connectionSettings.value = null;
  try {
    const [detail, tracking, connection] = await Promise.all([
      api.get<DomainDetailResponse>(`/v4/domains/${encodeURIComponent(name)}`),
      api.get<TrackingResponse>(`/v3/domains/${encodeURIComponent(name)}/tracking`).catch(() => null),
      api.get<ConnectionResponse>(`/v3/domains/${encodeURIComponent(name)}/connection`).catch(() => null),
    ]);
    selectedDomain.value = detail;
    if (tracking) trackingSettings.value = tracking.tracking;
    if (connection) connectionSettings.value = connection.connection;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domain detail";
  } finally {
    detailLoading.value = false;
  }
}

async function verifyDomain() {
  if (!selectedDomain.value) return;
  const name = selectedDomain.value.domain.name;
  error.value = null;
  try {
    const resp = await api.put<DomainDetailResponse>(`/v4/domains/${encodeURIComponent(name)}/verify`);
    selectedDomain.value = resp;
    await fetchDomains();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to verify domain";
  }
}

onMounted(() => fetchDomains());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Domains</h1>
      <div class="header-actions">
        <span class="total-count">{{ totalCount }} total</span>
      </div>
    </div>

    <!-- Add Domain Form -->
    <div class="add-domain-form">
      <input
        v-model="newDomainName"
        type="text"
        placeholder="Enter domain name (e.g. example.com)"
        class="filter-input"
        @keyup.enter="createDomain"
      >
      <button
        class="btn btn-primary"
        :disabled="!newDomainName.trim() || creating"
        @click="createDomain"
      >
        {{ creating ? "Creating..." : "Add Domain" }}
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
      <template #cell-name="{ row }">
        <a
          href="#"
          class="domain-link"
          @click.prevent="viewDetail(row)"
        >
          {{ row.name }}
        </a>
      </template>
      <template #cell-state="{ value }">
        <StatusBadge :status="String(value)" />
      </template>
      <template #cell-actions="{ row }">
        <button
          class="btn-icon btn-delete"
          title="Delete domain"
          @click.stop="deleteDomain(String(row._name))"
        >
          x
        </button>
      </template>
    </DataTable>

    <!-- Detail Panel -->
    <div
      v-if="detailLoading"
      class="detail-panel loading"
    >
      Loading domain detail...
    </div>
    <div
      v-else-if="selectedDomain"
      class="detail-panel"
    >
      <div class="detail-header">
        <h2>Domain Detail</h2>
        <div class="detail-header-actions">
          <button
            class="btn btn-primary btn-sm"
            @click="verifyDomain"
          >
            Verify Domain
          </button>
          <button
            class="btn btn-sm"
            @click="selectedDomain = null; trackingSettings = null; connectionSettings = null"
          >
            Close
          </button>
        </div>
      </div>

      <!-- Domain Info -->
      <div class="detail-grid">
        <div class="detail-field">
          <label>Name</label>
          <span class="mono">{{ selectedDomain.domain.name }}</span>
        </div>
        <div class="detail-field">
          <label>State</label>
          <span><StatusBadge :status="selectedDomain.domain.state" /></span>
        </div>
        <div class="detail-field">
          <label>Type</label>
          <span>{{ selectedDomain.domain.type }}</span>
        </div>
        <div class="detail-field">
          <label>SMTP Login</label>
          <span class="mono">{{ selectedDomain.domain.smtp_login }}</span>
        </div>
        <div class="detail-field">
          <label>Web Scheme</label>
          <span>{{ selectedDomain.domain.web_scheme }}</span>
        </div>
        <div class="detail-field">
          <label>Spam Action</label>
          <span>{{ selectedDomain.domain.spam_action }}</span>
        </div>
        <div class="detail-field">
          <label>Wildcard</label>
          <span>{{ selectedDomain.domain.wildcard ? "Yes" : "No" }}</span>
        </div>
        <div class="detail-field">
          <label>Created At</label>
          <span>{{ formatDate(selectedDomain.domain.created_at) }}</span>
        </div>
      </div>

      <!-- Sending DNS Records -->
      <div
        v-if="selectedDomain.sending_dns_records && selectedDomain.sending_dns_records.length > 0"
        class="detail-section"
      >
        <h3>Sending DNS Records</h3>
        <table class="dns-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Name</th>
              <th>Value</th>
              <th>Valid</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(rec, idx) in selectedDomain.sending_dns_records"
              :key="'send-' + idx"
            >
              <td>
                {{ rec.record_type }}
              </td>
              <td class="mono">
                {{ rec.name }}
              </td>
              <td class="mono dns-value">
                {{ rec.value }}
              </td>
              <td>
                <StatusBadge :status="rec.valid === 'valid' ? 'active' : 'unverified'" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Receiving DNS Records -->
      <div
        v-if="selectedDomain.receiving_dns_records && selectedDomain.receiving_dns_records.length > 0"
        class="detail-section"
      >
        <h3>Receiving DNS Records</h3>
        <table class="dns-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Name</th>
              <th>Value</th>
              <th>Valid</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(rec, idx) in selectedDomain.receiving_dns_records"
              :key="'recv-' + idx"
            >
              <td>
                {{ rec.record_type }}
              </td>
              <td class="mono">
                {{ rec.name }}
              </td>
              <td class="mono dns-value">
                {{ rec.value }}
              </td>
              <td>
                <StatusBadge :status="rec.valid === 'valid' ? 'active' : 'unverified'" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Tracking Settings -->
      <div
        v-if="trackingSettings"
        class="detail-section"
      >
        <h3>Tracking Settings</h3>
        <div class="detail-grid">
          <div class="detail-field">
            <label>Open Tracking</label>
            <span>
              <StatusBadge :status="trackingSettings.open.active === 'yes' ? 'active' : 'disabled'" />
            </span>
          </div>
          <div class="detail-field">
            <label>Click Tracking</label>
            <span>
              <StatusBadge :status="trackingSettings.click.active === 'yes' ? 'active' : 'disabled'" />
            </span>
          </div>
          <div class="detail-field">
            <label>Unsubscribe Tracking</label>
            <span>
              <StatusBadge :status="trackingSettings.unsubscribe.active === 'yes' ? 'active' : 'disabled'" />
            </span>
          </div>
        </div>
        <div
          v-if="trackingSettings.unsubscribe.html_footer"
          class="tracking-footer"
        >
          <label>Unsubscribe HTML Footer</label>
          <pre class="body-content">{{ trackingSettings.unsubscribe.html_footer }}</pre>
        </div>
        <div
          v-if="trackingSettings.unsubscribe.text_footer"
          class="tracking-footer"
        >
          <label>Unsubscribe Text Footer</label>
          <pre class="body-content">{{ trackingSettings.unsubscribe.text_footer }}</pre>
        </div>
      </div>

      <!-- Connection Settings -->
      <div
        v-if="connectionSettings"
        class="detail-section"
      >
        <h3>Connection Settings</h3>
        <div class="detail-grid">
          <div class="detail-field">
            <label>Require TLS</label>
            <span>
              <StatusBadge :status="connectionSettings.require_tls ? 'active' : 'disabled'" />
            </span>
          </div>
          <div class="detail-field">
            <label>Skip Verification</label>
            <span>
              <StatusBadge :status="connectionSettings.skip_verification ? 'active' : 'disabled'" />
            </span>
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

.add-domain-form {
  display: flex;
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
  min-width: 16rem;
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

.btn-sm.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #ffffff;
}

.btn-sm.btn-primary:hover {
  opacity: 0.9;
  background: var(--color-primary, #3b82f6);
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

.domain-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.domain-link:hover {
  text-decoration: underline;
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

.detail-header-actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
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

.dns-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
  margin-top: 0.5rem;
}

.dns-table th,
.dns-table td {
  text-align: left;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.dns-table th {
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
  background: var(--color-bg-subtle, #f8fafc);
  text-transform: uppercase;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
}

.dns-table tbody tr:hover {
  background: var(--color-bg-hover, #f1f5f9);
}

.dns-value {
  max-width: 24rem;
  word-break: break-all;
}

.tracking-footer {
  margin-top: 0.75rem;
}

.tracking-footer label {
  display: block;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  margin-bottom: 0.25rem;
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
</style>
