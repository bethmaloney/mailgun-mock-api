<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import type { Column } from "@/components/DataTable.vue";

interface RouteItem {
  id: string;
  priority: number;
  description: string;
  expression: string;
  actions: string[];
  created_at: string;
}

interface RoutesResponse {
  items: RouteItem[];
  total_count: number;
}

interface RouteDetailResponse {
  route: RouteItem;
}

interface CreateRouteResponse {
  message: string;
  route: RouteItem;
}

interface DeleteRouteResponse {
  message: string;
  id: string;
}

const routes = ref<RouteItem[]>([]);
const totalCount = ref(0);
const loading = ref(true);
const error = ref<string | null>(null);

// Pagination
const skip = ref(0);
const limit = ref(30);

const hasNext = computed(() => skip.value + limit.value < totalCount.value);
const hasPrevious = computed(() => skip.value > 0);

// Detail view
const selectedRoute = ref<RouteItem | null>(null);
const detailLoading = ref(false);

// Create form
const showCreateForm = ref(false);
const newPriority = ref(0);
const newExpression = ref("");
const newActions = ref("");
const newDescription = ref("");
const creating = ref(false);

const columns: Column[] = [
  { key: "priority", label: "Priority" },
  { key: "expression", label: "Expression" },
  { key: "actions", label: "Actions" },
  { key: "description", label: "Description" },
  { key: "created_at", label: "Created At" },
];

const tableRows = computed(() =>
  routes.value.map((route) => ({
    ...route,
    actions: route.actions ? route.actions.join(", ") : "",
    _raw: route,
  }))
);

async function fetchRoutes() {
  loading.value = true;
  error.value = null;
  try {
    const resp = await api.get<RoutesResponse>(
      `/v3/routes?skip=${skip.value}&limit=${limit.value}`
    );
    routes.value = resp.items || [];
    totalCount.value = resp.total_count || 0;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load routes";
  } finally {
    loading.value = false;
  }
}

function loadNext() {
  skip.value += limit.value;
  fetchRoutes();
}

function loadPrev() {
  skip.value = Math.max(0, skip.value - limit.value);
  fetchRoutes();
}

async function viewDetail(row: Record<string, unknown>) {
  const raw = row._raw as RouteItem;
  if (selectedRoute.value && selectedRoute.value.id === raw.id) {
    selectedRoute.value = null;
    return;
  }
  detailLoading.value = true;
  try {
    const resp = await api.get<RouteDetailResponse>(
      `/v3/routes/${encodeURIComponent(raw.id)}`
    );
    selectedRoute.value = resp.route;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load route detail";
  } finally {
    detailLoading.value = false;
  }
}

async function deleteRoute(id: string) {
  if (!window.confirm("Delete this route?")) return;
  try {
    await api.del<DeleteRouteResponse>(
      `/v3/routes/${encodeURIComponent(id)}`
    );
    if (selectedRoute.value && selectedRoute.value.id === id) {
      selectedRoute.value = null;
    }
    await fetchRoutes();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete route";
  }
}

async function createRoute() {
  if (!newExpression.value.trim()) {
    error.value = "Expression is required";
    return;
  }
  creating.value = true;
  error.value = null;
  try {
    const actionList = newActions.value
      .split(",")
      .map((a) => a.trim())
      .filter((a) => a.length > 0);
    await api.post<CreateRouteResponse>("/v3/routes", {
      priority: newPriority.value,
      description: newDescription.value,
      expression: newExpression.value,
      action: actionList,
    });
    // Reset form
    newPriority.value = 0;
    newExpression.value = "";
    newActions.value = "";
    newDescription.value = "";
    showCreateForm.value = false;
    // Refresh list
    skip.value = 0;
    await fetchRoutes();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to create route";
  } finally {
    creating.value = false;
  }
}

onMounted(() => fetchRoutes());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Routes</h1>
      <div class="header-actions">
        <span class="total-count">{{ totalCount }} total</span>
        <button
          class="btn btn-primary"
          @click="showCreateForm = !showCreateForm"
        >
          {{ showCreateForm ? "Cancel" : "Add Route" }}
        </button>
      </div>
    </div>

    <!-- Create Route Form -->
    <div
      v-if="showCreateForm"
      class="create-form"
    >
      <h3>Create Route</h3>
      <div class="form-grid">
        <div class="form-field">
          <label for="route-priority">Priority</label>
          <input
            id="route-priority"
            v-model.number="newPriority"
            type="number"
            min="0"
            class="filter-input"
            placeholder="0"
          >
        </div>
        <div class="form-field">
          <label for="route-expression">Expression</label>
          <input
            id="route-expression"
            v-model="newExpression"
            type="text"
            class="filter-input"
            placeholder="match_recipient('user@example.com')"
          >
        </div>
        <div class="form-field">
          <label for="route-actions">Actions (comma-separated)</label>
          <input
            id="route-actions"
            v-model="newActions"
            type="text"
            class="filter-input"
            placeholder="forward('http://...'), stop()"
          >
        </div>
        <div class="form-field">
          <label for="route-description">Description</label>
          <input
            id="route-description"
            v-model="newDescription"
            type="text"
            class="filter-input"
            placeholder="Route description"
          >
        </div>
      </div>
      <button
        class="btn btn-primary"
        :disabled="creating"
        @click="createRoute"
      >
        {{ creating ? "Creating..." : "Create" }}
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
      <template #cell-expression="{ row }">
        <a
          href="#"
          class="route-link"
          @click.prevent="viewDetail(row)"
        >
          {{ row.expression }}
        </a>
      </template>
      <template #cell-created_at="{ row }">
        <span class="date-cell">
          {{ row.created_at }}
          <button
            class="btn-icon btn-delete"
            title="Delete route"
            @click.stop="deleteRoute(String((row._raw as RouteItem).id))"
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
      Loading route detail...
    </div>
    <div
      v-else-if="selectedRoute"
      class="detail-panel"
    >
      <div class="detail-header">
        <h2>Route Detail</h2>
        <button
          class="btn btn-sm"
          @click="selectedRoute = null"
        >
          Close
        </button>
      </div>

      <div class="detail-grid">
        <div class="detail-field">
          <label>ID</label>
          <span class="mono">{{ selectedRoute.id }}</span>
        </div>
        <div class="detail-field">
          <label>Priority</label>
          <span>{{ selectedRoute.priority }}</span>
        </div>
        <div class="detail-field">
          <label>Description</label>
          <span>{{ selectedRoute.description || "-" }}</span>
        </div>
        <div class="detail-field">
          <label>Created At</label>
          <span>{{ selectedRoute.created_at }}</span>
        </div>
      </div>

      <!-- Expression -->
      <div class="detail-section">
        <h3>Expression</h3>
        <pre class="body-content">{{ selectedRoute.expression }}</pre>
      </div>

      <!-- Actions -->
      <div
        v-if="selectedRoute.actions && selectedRoute.actions.length > 0"
        class="detail-section"
      >
        <h3>Actions</h3>
        <ul class="actions-list">
          <li
            v-for="(action, idx) in selectedRoute.actions"
            :key="idx"
          >
            <code class="action-item">{{ action }}</code>
          </li>
        </ul>
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

.create-form {
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
  margin-bottom: 1rem;
}

.create-form h3 {
  margin: 0 0 1rem;
  font-size: 1rem;
  color: var(--color-text-primary, #1e293b);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.form-field label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
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

.route-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.route-link:hover {
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

.actions-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.actions-list li {
  padding: 0;
}

.action-item {
  display: inline-block;
  padding: 0.25rem 0.625rem;
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  font-size: 0.8125rem;
  font-family: monospace;
  color: var(--color-text-primary, #1e293b);
}
</style>
