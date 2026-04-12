<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "@/api/client";

interface ManagedKey {
  id: string;
  name: string;
  key_value: string;
  prefix: string;
  created_at: string;
}

const keys = ref<ManagedKey[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const newKeyName = ref("");
const justCreatedKey = ref<ManagedKey | null>(null);

function formatDate(dateStr: string): string {
  if (!dateStr) return "-";
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return dateStr;
  }
}

async function fetchKeys() {
  loading.value = true;
  error.value = null;
  try {
    const resp = await api.get<ManagedKey[]>("/mock/api-keys");
    keys.value = resp || [];
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load API keys";
  } finally {
    loading.value = false;
  }
}

async function createKey() {
  const trimmedName = newKeyName.value.trim();
  if (!trimmedName) return;
  error.value = null;
  try {
    const resp = await api.post<ManagedKey>("/mock/api-keys", { name: trimmedName });
    justCreatedKey.value = resp;
    newKeyName.value = "";
    await fetchKeys();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to create API key";
  }
}

async function deleteKey(id: string) {
  if (!window.confirm("Delete this API key? Applications using it will no longer be able to authenticate.")) return;
  error.value = null;
  try {
    await api.del(`/mock/api-keys/${id}`);
    await fetchKeys();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete API key";
  }
}

function copyKey(value: string) {
  navigator.clipboard.writeText(value).catch(() => {
    // Clipboard API may not be available in all contexts
  });
}

onMounted(() => fetchKeys());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>API Keys</h1>
        <p class="description">
          Managed API keys for test applications calling the Mailgun API surface (/v3/*, /v4/*).
        </p>
      </div>
    </div>

    <div class="create-form">
      <input
        v-model="newKeyName"
        type="text"
        placeholder="Key name (e.g. my-test-app)"
        class="filter-input"
        @keyup.enter="createKey"
      >
      <button
        class="btn btn-primary"
        :disabled="!newKeyName"
        @click="createKey"
      >
        Create Key
      </button>
    </div>

    <div
      v-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <div
      v-if="justCreatedKey"
      class="created-panel"
    >
      <div class="created-header">
        <strong>Key created successfully</strong>
        <button
          class="btn btn-sm"
          @click="justCreatedKey = null"
        >
          Dismiss
        </button>
      </div>
      <p class="created-hint">
        Copy this key now. You will not be able to see the full value again.
      </p>
      <div class="created-key-row">
        <span class="mono">{{ justCreatedKey.key_value }}</span>
        <button
          class="btn btn-sm"
          @click="copyKey(justCreatedKey!.key_value)"
        >
          Copy
        </button>
      </div>
    </div>

    <div
      v-if="loading"
      class="info"
    >
      Loading...
    </div>
    <div
      v-else-if="keys.length === 0"
      class="info"
    >
      No API keys yet. Test apps won't be able to call the Mailgun API surface until you create one.
    </div>
    <table
      v-else
      class="keys-table"
    >
      <thead>
        <tr>
          <th>Name</th>
          <th>Prefix</th>
          <th>Created</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="k in keys"
          :key="k.id"
        >
          <td>
            {{ k.name }}
          </td>
          <td class="mono">
            {{ k.prefix }}
          </td>
          <td>
            {{ formatDate(k.created_at) }}
          </td>
          <td>
            <button
              class="btn-icon btn-delete"
              title="Delete key"
              @click="deleteKey(k.id)"
            >
              x
            </button>
          </td>
        </tr>
      </tbody>
    </table>
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

.description {
  margin: 0.25rem 0 0;
  font-size: 0.875rem;
  color: var(--color-text-secondary, #64748b);
}

.create-form {
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

.created-panel {
  padding: 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-success-bg, #dcfce7);
  border: 1px solid var(--color-badge-success-text, #166534);
  border-radius: 0.375rem;
  color: var(--color-badge-success-text, #166534);
}

.created-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.created-hint {
  margin: 0 0 0.5rem;
  font-size: 0.8125rem;
}

.created-key-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.mono {
  font-family: monospace;
  font-size: 0.8125rem;
  word-break: break-all;
}

.keys-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.keys-table th,
.keys-table td {
  text-align: left;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.keys-table th {
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
  font-size: 0.75rem;
  text-transform: uppercase;
}

.keys-table tr:hover {
  background: var(--color-bg-hover, #f1f5f9);
}
</style>
