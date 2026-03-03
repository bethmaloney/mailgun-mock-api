<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import StatusBadge from "@/components/StatusBadge.vue";
import type { Column } from "@/components/DataTable.vue";

interface Domain {
  name: string;
  state: string;
}

interface DomainsResponse {
  items: Domain[];
  total_count: number;
}

interface TemplateItem {
  name: string;
  description: string;
  createdAt: string;
}

interface TemplatePaging {
  next?: string;
  previous?: string;
}

interface TemplatesResponse {
  items: TemplateItem[];
  paging: TemplatePaging;
}

interface TemplateVersion {
  tag: string;
  template: string;
  engine: string;
  active: boolean;
  comment: string;
  mjml: string;
  createdAt: string;
}

interface TemplateDetailResponse {
  template: {
    name: string;
    description: string;
    createdAt: string;
    version: TemplateVersion;
  };
}

interface TemplateVersionsResponse {
  template: {
    name: string;
  };
  items: TemplateVersion[];
  paging: TemplatePaging;
}

interface TemplateVersionDetailResponse {
  template: {
    name: string;
    version: TemplateVersion;
  };
}

interface DeleteTemplateResponse {
  message: string;
  template: {
    name: string;
  };
}

// Domain state
const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const domainsLoading = ref(true);

// Template list state
const templates = ref<TemplateItem[]>([]);
const templatesPaging = ref<TemplatePaging>({});
const loading = ref(false);
const error = ref<string | null>(null);

// Template detail state
const selectedTemplateName = ref<string | null>(null);
const selectedTemplateDetail = ref<TemplateDetailResponse["template"] | null>(null);
const detailLoading = ref(false);

// Version list state
const versions = ref<TemplateVersion[]>([]);
const versionsPaging = ref<TemplatePaging>({});
const versionsLoading = ref(false);

// Version detail state
const selectedVersionTag = ref<string | null>(null);
const selectedVersionDetail = ref<TemplateVersion | null>(null);
const versionDetailLoading = ref(false);

const templateColumns: Column[] = [
  { key: "name", label: "Name" },
  { key: "description", label: "Description" },
  { key: "createdAt", label: "Created At" },
  { key: "actions", label: "" },
];

const versionColumns: Column[] = [
  { key: "tag", label: "Tag" },
  { key: "engine", label: "Engine" },
  { key: "active", label: "Active" },
  { key: "comment", label: "Comment" },
  { key: "createdAt", label: "Created At" },
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

const templateRows = computed(() =>
  templates.value.map((t) => ({
    name: t.name,
    description: t.description || "-",
    createdAt: formatDate(t.createdAt),
    _raw: t,
  }))
);

const versionRows = computed(() =>
  versions.value.map((v) => ({
    tag: v.tag,
    engine: v.engine || "-",
    active: v.active ? "active" : "inactive",
    comment: v.comment || "-",
    createdAt: formatDate(v.createdAt),
    _raw: v,
  }))
);

const hasTemplatesNext = computed(() => !!templatesPaging.value.next);
const hasTemplatesPrevious = computed(() => !!templatesPaging.value.previous);
const hasVersionsNext = computed(() => !!versionsPaging.value.next);
const hasVersionsPrevious = computed(() => !!versionsPaging.value.previous);

async function fetchDomains() {
  domainsLoading.value = true;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
      await fetchTemplates();
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

function buildTemplatesUrl(pagingUrl?: string): string {
  if (pagingUrl) {
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  return `/v3/${encodeURIComponent(selectedDomain.value)}/templates`;
}

async function fetchTemplates(pagingUrl?: string) {
  if (!selectedDomain.value) return;
  loading.value = true;
  error.value = null;
  selectedTemplateName.value = null;
  selectedTemplateDetail.value = null;
  versions.value = [];
  selectedVersionTag.value = null;
  selectedVersionDetail.value = null;
  try {
    const url = buildTemplatesUrl(pagingUrl);
    const resp = await api.get<TemplatesResponse>(url);
    templates.value = resp.items || [];
    templatesPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load templates";
  } finally {
    loading.value = false;
  }
}

async function selectTemplate(row: Record<string, unknown>) {
  const t = row._raw as TemplateItem;
  if (selectedTemplateName.value === t.name) {
    selectedTemplateName.value = null;
    selectedTemplateDetail.value = null;
    versions.value = [];
    selectedVersionTag.value = null;
    selectedVersionDetail.value = null;
    return;
  }

  selectedTemplateName.value = t.name;
  selectedVersionTag.value = null;
  selectedVersionDetail.value = null;
  detailLoading.value = true;

  try {
    const resp = await api.get<TemplateDetailResponse>(
      `/v3/${encodeURIComponent(selectedDomain.value)}/templates/${encodeURIComponent(t.name)}?active=yes`
    );
    selectedTemplateDetail.value = resp.template;
    await fetchVersions();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load template detail";
  } finally {
    detailLoading.value = false;
  }
}

function buildVersionsUrl(pagingUrl?: string): string {
  if (pagingUrl) {
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  return `/v3/${encodeURIComponent(selectedDomain.value)}/templates/${encodeURIComponent(selectedTemplateName.value!)}/versions`;
}

async function fetchVersions(pagingUrl?: string) {
  if (!selectedTemplateName.value) return;
  versionsLoading.value = true;
  selectedVersionTag.value = null;
  selectedVersionDetail.value = null;
  try {
    const url = buildVersionsUrl(pagingUrl);
    const resp = await api.get<TemplateVersionsResponse>(url);
    versions.value = resp.items || [];
    versionsPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load versions";
  } finally {
    versionsLoading.value = false;
  }
}

async function selectVersion(row: Record<string, unknown>) {
  const v = row._raw as TemplateVersion;
  if (selectedVersionTag.value === v.tag) {
    selectedVersionTag.value = null;
    selectedVersionDetail.value = null;
    return;
  }

  selectedVersionTag.value = v.tag;
  versionDetailLoading.value = true;
  try {
    const resp = await api.get<TemplateVersionDetailResponse>(
      `/v3/${encodeURIComponent(selectedDomain.value)}/templates/${encodeURIComponent(selectedTemplateName.value!)}/versions/${encodeURIComponent(v.tag)}`
    );
    selectedVersionDetail.value = resp.template.version;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load version detail";
  } finally {
    versionDetailLoading.value = false;
  }
}

async function deleteTemplate(name: string) {
  if (!window.confirm(`Delete template "${name}"?`)) return;
  try {
    await api.del<DeleteTemplateResponse>(
      `/v3/${encodeURIComponent(selectedDomain.value)}/templates/${encodeURIComponent(name)}`
    );
    if (selectedTemplateName.value === name) {
      selectedTemplateName.value = null;
      selectedTemplateDetail.value = null;
      versions.value = [];
      selectedVersionTag.value = null;
      selectedVersionDetail.value = null;
    }
    await fetchTemplates();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete template";
  }
}

function onDomainChange() {
  fetchTemplates();
}

function loadTemplatesNext() {
  if (templatesPaging.value.next) fetchTemplates(templatesPaging.value.next);
}

function loadTemplatesPrev() {
  if (templatesPaging.value.previous) fetchTemplates(templatesPaging.value.previous);
}

function loadVersionsNext() {
  if (versionsPaging.value.next) fetchVersions(versionsPaging.value.next);
}

function loadVersionsPrev() {
  if (versionsPaging.value.previous) fetchVersions(versionsPaging.value.previous);
}

onMounted(() => fetchDomains());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Templates</h1>
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
      No domains configured. Create a domain first to view templates.
    </div>

    <template v-if="selectedDomain">
      <!-- Template List -->
      <DataTable
        :columns="templateColumns"
        :rows="templateRows"
        :loading="loading"
      >
        <template #cell-name="{ value, row }">
          <a
            href="#"
            class="template-link"
            @click.prevent="selectTemplate(row)"
          >
            {{ value }}
          </a>
        </template>
        <template #cell-actions="{ row }">
          <button
            class="btn btn-danger btn-sm"
            @click.stop="deleteTemplate(String(row.name))"
          >
            Delete
          </button>
        </template>
      </DataTable>

      <Pagination
        :has-next="hasTemplatesNext"
        :has-previous="hasTemplatesPrevious"
        @next="loadTemplatesNext"
        @previous="loadTemplatesPrev"
      />

      <!-- Template Detail Panel -->
      <div
        v-if="detailLoading"
        class="detail-panel loading"
      >
        Loading template detail...
      </div>
      <div
        v-else-if="selectedTemplateDetail"
        class="detail-panel"
      >
        <div class="detail-header">
          <h2>Template Detail</h2>
          <button
            class="btn btn-sm"
            @click="selectedTemplateName = null; selectedTemplateDetail = null; versions = []; selectedVersionTag = null; selectedVersionDetail = null"
          >
            Close
          </button>
        </div>

        <div class="detail-grid">
          <div class="detail-field">
            <label>Name</label>
            <span class="mono">{{ selectedTemplateDetail.name }}</span>
          </div>
          <div class="detail-field">
            <label>Description</label>
            <span>{{ selectedTemplateDetail.description || "-" }}</span>
          </div>
          <div class="detail-field">
            <label>Created At</label>
            <span>{{ formatDate(selectedTemplateDetail.createdAt) }}</span>
          </div>
        </div>

        <!-- Active Version Summary -->
        <div
          v-if="selectedTemplateDetail.version"
          class="detail-section"
        >
          <h3>Active Version</h3>
          <div class="detail-grid">
            <div class="detail-field">
              <label>Tag</label>
              <span class="mono">{{ selectedTemplateDetail.version.tag }}</span>
            </div>
            <div class="detail-field">
              <label>Engine</label>
              <span>{{ selectedTemplateDetail.version.engine || "-" }}</span>
            </div>
            <div class="detail-field">
              <label>Comment</label>
              <span>{{ selectedTemplateDetail.version.comment || "-" }}</span>
            </div>
          </div>
        </div>

        <!-- Versions List -->
        <div class="detail-section">
          <h3>Versions</h3>
          <DataTable
            :columns="versionColumns"
            :rows="versionRows"
            :loading="versionsLoading"
          >
            <template #cell-tag="{ value, row }">
              <a
                href="#"
                class="template-link"
                @click.prevent="selectVersion(row)"
              >
                {{ value }}
              </a>
            </template>
            <template #cell-active="{ value }">
              <StatusBadge
                v-if="value === 'active'"
                status="active"
              />
              <span v-else>-</span>
            </template>
          </DataTable>

          <Pagination
            :has-next="hasVersionsNext"
            :has-previous="hasVersionsPrevious"
            @next="loadVersionsNext"
            @previous="loadVersionsPrev"
          />
        </div>

        <!-- Version Content Preview -->
        <div
          v-if="versionDetailLoading"
          class="detail-section"
        >
          <h3>Version Content</h3>
          <div class="loading-text">
            Loading version content...
          </div>
        </div>
        <div
          v-else-if="selectedVersionDetail"
          class="detail-section"
        >
          <div class="version-detail-header">
            <h3>Version Content: {{ selectedVersionDetail.tag }}</h3>
            <button
              class="btn btn-sm"
              @click="selectedVersionTag = null; selectedVersionDetail = null"
            >
              Close
            </button>
          </div>

          <div class="detail-grid">
            <div class="detail-field">
              <label>Tag</label>
              <span class="mono">{{ selectedVersionDetail.tag }}</span>
            </div>
            <div class="detail-field">
              <label>Engine</label>
              <span>{{ selectedVersionDetail.engine || "-" }}</span>
            </div>
            <div class="detail-field">
              <label>Active</label>
              <StatusBadge
                v-if="selectedVersionDetail.active"
                status="active"
              />
              <span v-else>No</span>
            </div>
            <div class="detail-field">
              <label>Comment</label>
              <span>{{ selectedVersionDetail.comment || "-" }}</span>
            </div>
            <div
              v-if="selectedVersionDetail.mjml"
              class="detail-field"
            >
              <label>MJML</label>
              <span class="mono">{{ selectedVersionDetail.mjml }}</span>
            </div>
            <div class="detail-field">
              <label>Created At</label>
              <span>{{ formatDate(selectedVersionDetail.createdAt) }}</span>
            </div>
          </div>

          <div
            v-if="selectedVersionDetail.template"
            class="version-body"
          >
            <h4>Template Body</h4>
            <pre class="body-content">{{ selectedVersionDetail.template }}</pre>
          </div>
        </div>
      </div>
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

.template-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.template-link:hover {
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

.version-detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.version-detail-header h3 {
  margin: 0;
}

.version-body {
  margin-top: 1rem;
}

.version-body h4 {
  font-size: 0.8125rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  margin: 0 0 0.5rem;
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
  max-height: 30rem;
  overflow-y: auto;
}

.loading-text {
  text-align: center;
  padding: 1rem;
  color: var(--color-text-secondary, #64748b);
}
</style>
