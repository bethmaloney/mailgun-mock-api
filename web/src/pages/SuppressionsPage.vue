<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
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

interface SuppressionPaging {
  next?: string;
  previous?: string;
}

interface BounceItem {
  address: string;
  code: string;
  error: string;
  created_at: string;
}

interface BouncesResponse {
  items: BounceItem[];
  paging: SuppressionPaging;
}

interface ComplaintItem {
  address: string;
  count: number;
  created_at: string;
}

interface ComplaintsResponse {
  items: ComplaintItem[];
  paging: SuppressionPaging;
}

interface UnsubscribeItem {
  address: string;
  tags: string[];
  created_at: string;
}

interface UnsubscribesResponse {
  items: UnsubscribeItem[];
  paging: SuppressionPaging;
}

interface AllowlistItem {
  value: string;
  type: string;
  reason: string;
  createdAt: string;
}

interface AllowlistResponse {
  items: AllowlistItem[];
  paging: SuppressionPaging;
}

type TabName = "bounces" | "complaints" | "unsubscribes" | "allowlist";

// --- State ---

const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const domainsLoading = ref(true);
const error = ref<string | null>(null);
const loading = ref(false);
const activeTab = ref<TabName>("bounces");
const searchQuery = ref("");

// Data per tab
const bounces = ref<BounceItem[]>([]);
const complaints = ref<ComplaintItem[]>([]);
const unsubscribes = ref<UnsubscribeItem[]>([]);
const allowlist = ref<AllowlistItem[]>([]);

// Paging per tab
const bouncesPaging = ref<SuppressionPaging>({});
const complaintsPaging = ref<SuppressionPaging>({});
const unsubscribesPaging = ref<SuppressionPaging>({});
const allowlistPaging = ref<SuppressionPaging>({});

// Add form state
const addBounceAddress = ref("");
const addBounceCode = ref("550");
const addBounceError = ref("");
const addComplaintAddress = ref("");
const addUnsubAddress = ref("");
const addUnsubTag = ref("*");
const addAllowlistValue = ref("");
const addAllowlistType = ref<"address" | "domain">("address");
const showAddForm = ref(false);

// --- Column Definitions ---

const bounceColumns: Column[] = [
  { key: "address", label: "Address" },
  { key: "code", label: "Code" },
  { key: "error", label: "Error" },
  { key: "created_at", label: "Created At" },
  { key: "actions", label: "" },
];

const complaintColumns: Column[] = [
  { key: "address", label: "Address" },
  { key: "count", label: "Count" },
  { key: "created_at", label: "Created At" },
  { key: "actions", label: "" },
];

const unsubscribeColumns: Column[] = [
  { key: "address", label: "Address" },
  { key: "tags", label: "Tags" },
  { key: "created_at", label: "Created At" },
  { key: "actions", label: "" },
];

const allowlistColumns: Column[] = [
  { key: "value", label: "Value" },
  { key: "type", label: "Type" },
  { key: "reason", label: "Reason" },
  { key: "createdAt", label: "Created At" },
  { key: "actions", label: "" },
];

// --- Computed ---

const currentColumns = computed(() => {
  switch (activeTab.value) {
    case "bounces":
      return bounceColumns;
    case "complaints":
      return complaintColumns;
    case "unsubscribes":
      return unsubscribeColumns;
    case "allowlist":
      return allowlistColumns;
    default:
      return bounceColumns;
  }
});

const currentPaging = computed(() => {
  switch (activeTab.value) {
    case "bounces":
      return bouncesPaging.value;
    case "complaints":
      return complaintsPaging.value;
    case "unsubscribes":
      return unsubscribesPaging.value;
    case "allowlist":
      return allowlistPaging.value;
    default:
      return bouncesPaging.value;
  }
});

const hasNext = computed(() => !!currentPaging.value.next);
const hasPrevious = computed(() => !!currentPaging.value.previous);

function filterBySearch<T extends Record<string, unknown>>(items: T[]): T[] {
  if (!searchQuery.value) return items;
  const q = searchQuery.value.toLowerCase();
  return items.filter((item) =>
    Object.values(item).some((val) =>
      String(val).toLowerCase().includes(q)
    )
  );
}

const bounceRows = computed(() =>
  filterBySearch(
    bounces.value.map((b) => ({
      address: b.address,
      code: b.code,
      error: b.error,
      created_at: b.created_at,
      _address: b.address,
    }))
  )
);

const complaintRows = computed(() =>
  filterBySearch(
    complaints.value.map((c) => ({
      address: c.address,
      count: c.count,
      created_at: c.created_at,
      _address: c.address,
    }))
  )
);

const unsubscribeRows = computed(() =>
  filterBySearch(
    unsubscribes.value.map((u) => ({
      address: u.address,
      tags: u.tags ? u.tags.join(", ") : "-",
      created_at: u.created_at,
      _address: u.address,
    }))
  )
);

const allowlistRows = computed(() =>
  filterBySearch(
    allowlist.value.map((a) => ({
      value: a.value,
      type: a.type,
      reason: a.reason,
      createdAt: a.createdAt,
      _value: a.value,
    }))
  )
);

const currentRows = computed(() => {
  switch (activeTab.value) {
    case "bounces":
      return bounceRows.value;
    case "complaints":
      return complaintRows.value;
    case "unsubscribes":
      return unsubscribeRows.value;
    case "allowlist":
      return allowlistRows.value;
    default:
      return bounceRows.value;
  }
});

// --- Domain Fetch ---

async function fetchDomains() {
  domainsLoading.value = true;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
      await fetchData();
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

// --- Paging URL Helper ---

function buildPagingUrl(pagingUrl: string): string {
  try {
    const u = new URL(pagingUrl);
    return u.pathname + u.search;
  } catch {
    return pagingUrl;
  }
}

// --- Data Fetch Functions ---

async function fetchBounces(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  try {
    const url = pagingUrl
      ? buildPagingUrl(pagingUrl)
      : `/v3/${encodeURIComponent(selectedDomain.value)}/bounces`;
    const resp = await api.get<BouncesResponse>(url);
    bounces.value = resp.items || [];
    bouncesPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load bounces";
  } finally {
    loading.value = false;
  }
}

async function fetchComplaints(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  try {
    const url = pagingUrl
      ? buildPagingUrl(pagingUrl)
      : `/v3/${encodeURIComponent(selectedDomain.value)}/complaints`;
    const resp = await api.get<ComplaintsResponse>(url);
    complaints.value = resp.items || [];
    complaintsPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load complaints";
  } finally {
    loading.value = false;
  }
}

async function fetchUnsubscribes(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  try {
    const url = pagingUrl
      ? buildPagingUrl(pagingUrl)
      : `/v3/${encodeURIComponent(selectedDomain.value)}/unsubscribes`;
    const resp = await api.get<UnsubscribesResponse>(url);
    unsubscribes.value = resp.items || [];
    unsubscribesPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load unsubscribes";
  } finally {
    loading.value = false;
  }
}

async function fetchAllowlist(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  try {
    const url = pagingUrl
      ? buildPagingUrl(pagingUrl)
      : `/v3/${encodeURIComponent(selectedDomain.value)}/whitelists`;
    const resp = await api.get<AllowlistResponse>(url);
    allowlist.value = resp.items || [];
    allowlistPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load allowlist";
  } finally {
    loading.value = false;
  }
}

async function fetchData(pagingUrl?: string) {
  switch (activeTab.value) {
    case "bounces":
      return fetchBounces(pagingUrl);
    case "complaints":
      return fetchComplaints(pagingUrl);
    case "unsubscribes":
      return fetchUnsubscribes(pagingUrl);
    case "allowlist":
      return fetchAllowlist(pagingUrl);
  }
}

// --- Add Functions ---

async function addBounce() {
  if (!addBounceAddress.value || !selectedDomain.value) return;
  error.value = null;
  try {
    await api.postForm(
      `/v3/${encodeURIComponent(selectedDomain.value)}/bounces`,
      {
        address: addBounceAddress.value,
        code: addBounceCode.value,
        error: addBounceError.value,
      }
    );
    addBounceAddress.value = "";
    addBounceCode.value = "550";
    addBounceError.value = "";
    showAddForm.value = false;
    await fetchBounces();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to add bounce";
  }
}

async function addComplaint() {
  if (!addComplaintAddress.value || !selectedDomain.value) return;
  error.value = null;
  try {
    await api.postForm(
      `/v3/${encodeURIComponent(selectedDomain.value)}/complaints`,
      { address: addComplaintAddress.value }
    );
    addComplaintAddress.value = "";
    showAddForm.value = false;
    await fetchComplaints();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to add complaint";
  }
}

async function addUnsubscribe() {
  if (!addUnsubAddress.value || !selectedDomain.value) return;
  error.value = null;
  try {
    await api.postForm(
      `/v3/${encodeURIComponent(selectedDomain.value)}/unsubscribes`,
      {
        address: addUnsubAddress.value,
        tag: addUnsubTag.value || "*",
      }
    );
    addUnsubAddress.value = "";
    addUnsubTag.value = "*";
    showAddForm.value = false;
    await fetchUnsubscribes();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to add unsubscribe";
  }
}

async function addAllowlistEntry() {
  if (!addAllowlistValue.value || !selectedDomain.value) return;
  error.value = null;
  try {
    const payload: Record<string, string> =
      addAllowlistType.value === "domain"
        ? { domain: addAllowlistValue.value }
        : { address: addAllowlistValue.value };
    await api.postForm(
      `/v3/${encodeURIComponent(selectedDomain.value)}/whitelists`,
      payload
    );
    addAllowlistValue.value = "";
    addAllowlistType.value = "address";
    showAddForm.value = false;
    await fetchAllowlist();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to add allowlist entry";
  }
}

function submitAdd() {
  switch (activeTab.value) {
    case "bounces":
      return addBounce();
    case "complaints":
      return addComplaint();
    case "unsubscribes":
      return addUnsubscribe();
    case "allowlist":
      return addAllowlistEntry();
  }
}

// --- Delete Functions ---

async function deleteBounce(address: string) {
  if (!window.confirm(`Delete bounce for ${address}?`)) return;
  error.value = null;
  try {
    await api.del(
      `/v3/${encodeURIComponent(selectedDomain.value)}/bounces/${encodeURIComponent(address)}`
    );
    await fetchBounces();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete bounce";
  }
}

async function deleteComplaint(address: string) {
  if (!window.confirm(`Delete complaint for ${address}?`)) return;
  error.value = null;
  try {
    await api.del(
      `/v3/${encodeURIComponent(selectedDomain.value)}/complaints/${encodeURIComponent(address)}`
    );
    await fetchComplaints();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete complaint";
  }
}

async function deleteUnsubscribe(address: string) {
  if (!window.confirm(`Delete unsubscribe for ${address}?`)) return;
  error.value = null;
  try {
    await api.del(
      `/v3/${encodeURIComponent(selectedDomain.value)}/unsubscribes/${encodeURIComponent(address)}`
    );
    await fetchUnsubscribes();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete unsubscribe";
  }
}

async function deleteAllowlistEntry(value: string) {
  if (!window.confirm(`Delete allowlist entry for ${value}?`)) return;
  error.value = null;
  try {
    await api.del(
      `/v3/${encodeURIComponent(selectedDomain.value)}/whitelists/${encodeURIComponent(value)}`
    );
    await fetchAllowlist();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete allowlist entry";
  }
}

function deleteEntry(row: Record<string, unknown>) {
  switch (activeTab.value) {
    case "bounces":
      return deleteBounce(String(row._address));
    case "complaints":
      return deleteComplaint(String(row._address));
    case "unsubscribes":
      return deleteUnsubscribe(String(row._address));
    case "allowlist":
      return deleteAllowlistEntry(String(row._value));
  }
}

// --- Clear All ---

async function clearAll() {
  const label = activeTab.value === "allowlist" ? "allowlist entries" : activeTab.value;
  if (!window.confirm(`Delete ALL ${label} for ${selectedDomain.value}?`))
    return;
  error.value = null;
  try {
    const endpoint =
      activeTab.value === "allowlist" ? "whitelists" : activeTab.value;
    await api.del(
      `/v3/${encodeURIComponent(selectedDomain.value)}/${endpoint}`
    );
    await fetchData();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || `Failed to clear ${label}`;
  }
}

// --- Navigation ---

function onDomainChange() {
  showAddForm.value = false;
  fetchData();
}

function switchTab(tab: TabName) {
  activeTab.value = tab;
  showAddForm.value = false;
  searchQuery.value = "";
  fetchData();
}

function loadNext() {
  const p = currentPaging.value;
  if (p.next) fetchData(p.next);
}

function loadPrev() {
  const p = currentPaging.value;
  if (p.previous) fetchData(p.previous);
}

function toggleAddForm() {
  showAddForm.value = !showAddForm.value;
}

// --- Lifecycle ---

onMounted(() => fetchDomains());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Suppressions</h1>
      <div class="header-actions">
        <button
          v-if="selectedDomain"
          class="btn btn-primary"
          @click="toggleAddForm"
        >
          {{ showAddForm ? "Cancel" : "Add Entry" }}
        </button>
        <button
          v-if="selectedDomain"
          class="btn btn-danger"
          @click="clearAll"
        >
          Clear All
        </button>
      </div>
    </div>

    <!-- Domain Selector -->
    <div class="controls">
      <div class="domain-selector">
        <label for="suppression-domain-select">Domain:</label>
        <select
          id="suppression-domain-select"
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

      <div
        v-if="selectedDomain"
        class="filters"
      >
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search..."
          class="filter-input"
        >
      </div>
    </div>

    <!-- Tabs -->
    <div
      v-if="selectedDomain"
      class="tab-bar"
    >
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'bounces' }"
        @click="switchTab('bounces')"
      >
        Bounces
      </button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'complaints' }"
        @click="switchTab('complaints')"
      >
        Complaints
      </button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'unsubscribes' }"
        @click="switchTab('unsubscribes')"
      >
        Unsubscribes
      </button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'allowlist' }"
        @click="switchTab('allowlist')"
      >
        Allowlist
      </button>
    </div>

    <!-- Add Form -->
    <div
      v-if="showAddForm && selectedDomain"
      class="add-form"
    >
      <h3>Add {{ activeTab === 'allowlist' ? 'Allowlist Entry' : activeTab.slice(0, -1).charAt(0).toUpperCase() + activeTab.slice(1, -1) }}</h3>

      <!-- Bounce add form -->
      <div
        v-if="activeTab === 'bounces'"
        class="form-fields"
      >
        <input
          v-model="addBounceAddress"
          type="email"
          placeholder="Email address"
          class="filter-input"
          @keyup.enter="submitAdd"
        >
        <input
          v-model="addBounceCode"
          type="text"
          placeholder="Code (e.g. 550)"
          class="filter-input filter-input-sm"
          @keyup.enter="submitAdd"
        >
        <input
          v-model="addBounceError"
          type="text"
          placeholder="Error message"
          class="filter-input"
          @keyup.enter="submitAdd"
        >
        <button
          class="btn btn-primary"
          :disabled="!addBounceAddress"
          @click="submitAdd"
        >
          Add Bounce
        </button>
      </div>

      <!-- Complaint add form -->
      <div
        v-if="activeTab === 'complaints'"
        class="form-fields"
      >
        <input
          v-model="addComplaintAddress"
          type="email"
          placeholder="Email address"
          class="filter-input"
          @keyup.enter="submitAdd"
        >
        <button
          class="btn btn-primary"
          :disabled="!addComplaintAddress"
          @click="submitAdd"
        >
          Add Complaint
        </button>
      </div>

      <!-- Unsubscribe add form -->
      <div
        v-if="activeTab === 'unsubscribes'"
        class="form-fields"
      >
        <input
          v-model="addUnsubAddress"
          type="email"
          placeholder="Email address"
          class="filter-input"
          @keyup.enter="submitAdd"
        >
        <input
          v-model="addUnsubTag"
          type="text"
          placeholder="Tag (default: *)"
          class="filter-input filter-input-sm"
          @keyup.enter="submitAdd"
        >
        <button
          class="btn btn-primary"
          :disabled="!addUnsubAddress"
          @click="submitAdd"
        >
          Add Unsubscribe
        </button>
      </div>

      <!-- Allowlist add form -->
      <div
        v-if="activeTab === 'allowlist'"
        class="form-fields"
      >
        <select
          v-model="addAllowlistType"
          class="select-input"
        >
          <option value="address">
            Address
          </option>
          <option value="domain">
            Domain
          </option>
        </select>
        <input
          v-model="addAllowlistValue"
          type="text"
          :placeholder="addAllowlistType === 'domain' ? 'example.com' : 'user@example.com'"
          class="filter-input"
          @keyup.enter="submitAdd"
        >
        <button
          class="btn btn-primary"
          :disabled="!addAllowlistValue"
          @click="submitAdd"
        >
          Add to Allowlist
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
      No domains configured. Create a domain first to manage suppressions.
    </div>

    <template v-if="selectedDomain">
      <DataTable
        :columns="currentColumns"
        :rows="currentRows"
        :loading="loading"
      >
        <template #cell-actions="{ row }">
          <button
            class="btn-icon btn-delete"
            title="Delete entry"
            @click.stop="deleteEntry(row)"
          >
            x
          </button>
        </template>
      </DataTable>

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
  gap: 0.5rem;
}

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

.filter-input-sm {
  min-width: 6rem;
  max-width: 10rem;
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

.info {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border-radius: 0.375rem;
}

/* Tab Bar */
.tab-bar {
  display: flex;
  gap: 0;
  border-bottom: 2px solid var(--color-border, #e2e8f0);
  margin-bottom: 1rem;
}

.tab-btn {
  padding: 0.5rem 1rem;
  font-size: 0.875rem;
  font-weight: 500;
  border: none;
  background: none;
  cursor: pointer;
  color: var(--color-text-secondary, #64748b);
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: color 0.15s, border-color 0.15s;
}

.tab-btn:hover {
  color: var(--color-text-primary, #1e293b);
}

.tab-btn.active {
  color: var(--color-primary, #3b82f6);
  border-bottom-color: var(--color-primary, #3b82f6);
}

/* Add Form */
.add-form {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1rem;
  margin-bottom: 1rem;
}

.add-form h3 {
  margin: 0 0 0.75rem;
  font-size: 0.875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.form-fields {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
</style>
