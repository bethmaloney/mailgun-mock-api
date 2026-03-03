<script setup lang="ts">
export interface Column {
  key: string;
  label: string;
  sortable?: boolean;
}

defineProps<{
  columns: Column[];
  rows: Record<string, unknown>[];
  loading?: boolean;
}>();
</script>

<template>
  <div class="data-table-wrapper">
    <div
      v-if="loading"
      class="data-table-loading"
    >
      Loading...
    </div>
    <table
      v-else
      class="data-table"
    >
      <thead>
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-if="rows.length === 0"
        >
          <td
            :colspan="columns.length"
            class="data-table-empty"
          >
            No data available.
          </td>
        </tr>
        <tr
          v-for="(row, idx) in rows"
          :key="idx"
        >
          <td
            v-for="col in columns"
            :key="col.key"
          >
            <slot
              :name="`cell-${col.key}`"
              :row="row"
              :value="row[col.key]"
            >
              {{ row[col.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.data-table-wrapper {
  width: 100%;
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.data-table th,
.data-table td {
  text-align: left;
  padding: 0.625rem 0.75rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.data-table th {
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
  background: var(--color-bg-subtle, #f8fafc);
  text-transform: uppercase;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
}

.data-table tbody tr:hover {
  background: var(--color-bg-hover, #f1f5f9);
}

.data-table-empty {
  text-align: center;
  color: var(--color-text-secondary, #64748b);
  padding: 2rem 0.75rem;
}

.data-table-loading {
  text-align: center;
  padding: 2rem;
  color: var(--color-text-secondary, #64748b);
}
</style>
